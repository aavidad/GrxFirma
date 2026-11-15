// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

// Las firmas de testdata/grxfirma_anterior las generó el código de 80e7dd5
// (anterior a E1-07 y E1-08) con un certificado sintético autofirmado, en una
// prueba auxiliar fuera del repositorio. Con aquel verificador salían íntegras.
func leerFirmaGrxFirmaAnterior(t *testing.T, nombre string) []byte {
	t.Helper()
	datos, err := os.ReadFile(filepath.Join("testdata", "grxfirma_anterior", nombre))
	if err != nil {
		t.Fatal(err)
	}
	return datos
}

func verificarXAdESBytes(datos []byte) (domain.VerificationResult, []domain.CertificateRef, error) {
	return NewXAdESVerifier().Verify(context.Background(), domain.Document{Name: "firma.xsig", MIMEType: "application/xml", Content: datos}, domain.CertificateChain{})
}

func exigirCompatibilidadHistorica(t *testing.T, nombre string, result domain.VerificationResult, avisos ...string) {
	t.Helper()
	if result.Valid || result.Integrity.Status != domain.VerificationStatusWarning {
		t.Fatalf("%s: se esperaba compatibilidad histórica (no válida, integridad en advertencia): valid=%v integridad=%+v", nombre, result.Valid, result.Integrity)
	}
	if len(xmlCompatibilityDetails(result)) == 0 {
		t.Fatalf("%s: falta la evidencia de compatibilidad", nombre)
	}
	for _, aviso := range avisos {
		if !contieneString(result.Warnings, aviso) {
			t.Fatalf("%s: falta el aviso %q en %v", nombre, aviso, result.Warnings)
		}
	}
}

// Lote 13b, punto 3: una firma de una versión anterior de GrxFirma sobre un
// documento con tabuladores y saltos de línea en atributos, o con una hoja de
// estilo fuera de la raíz, salía con error («digest de referencia no
// coincide»). Ahora es compatibilidad histórica con su aviso.
func TestXAdES_FirmaDeGrxFirmaAnteriorEsCompatibilidadHistorica(t *testing.T) {
	activarExcepcionPrueba(t, "xml_c14n_antigua")
	for _, caso := range []struct {
		fichero string
		avisos  []string
	}{
		{"atributos_tab_lf.xsig", []string{motivoC14NAntiguaGrxFirma}},
		{"pi_fuera_raiz.xsig", []string{motivoC14NAntiguaGrxFirma, motivoPIFueraDeRaizSinFirmar}},
		{"doctype_externo.xsig", []string{motivoDOCTYPEExterno}},
	} {
		result, signers, err := verificarXAdESBytes(leerFirmaGrxFirmaAnterior(t, caso.fichero))
		if err != nil {
			t.Fatalf("%s: %v", caso.fichero, err)
		}
		if len(signers) != 1 {
			t.Fatalf("%s: firmantes=%d", caso.fichero, len(signers))
		}
		exigirCompatibilidadHistorica(t, caso.fichero, result, caso.avisos...)
	}
}

func TestXAdESC14NAntiguaNoSeEvaluaSinAjuste(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_xml_c14n_antigua")
	excepciones.SetForTest(t, "xml_c14n_antigua", false)
	result, _, err := verificarXAdESBytes(leerFirmaGrxFirmaAnterior(t, "atributos_tab_lf.xsig"))
	if err != nil {
		t.Fatal(err)
	}
	block := bloqueoTipadoV38(result.PolicyError())
	if block == nil || block.ID != "xml_c14n_antigua" || result.Valid || result.Integrity.Status != domain.VerificationStatusUnknown {
		t.Fatalf("C14N antigua debe quedar no evaluada con su ajuste visible: valid=%v integridad=%s motivo=%q", result.Valid, result.Integrity.Status, result.Reason)
	}
}

func TestXAdESManipuladaNoSugiereActivarC14NAntigua(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_xml_c14n_antigua")
	excepciones.SetForTest(t, "xml_c14n_antigua", false)
	firmado := operarXAdES(t, []byte(`<pedido><importe>10</importe></pedido>`), "application/xml", domain.ActionSign,
		map[string]string{"format": "XAdES Enveloped"}, "Original")
	alterado := bytes.Replace(firmado, []byte("<importe>10"), []byte("<importe>99"), 1)
	if bytes.Equal(alterado, firmado) {
		t.Fatal("el fixture no cambió")
	}
	_, _, err := verificarXAdESBytes(alterado)
	if err == nil {
		t.Fatal("el XML alterado no puede verificarse")
	}
	var block *excepciones.Block
	if errors.As(err, &block) && block.ID == "xml_c14n_antigua" {
		t.Fatalf("un XML manipulado no debe ofrecer activar C14N antigua: %v", err)
	}
}

func TestXAdESHistoricaConContenidoAlteradoNoSugiereActivarC14N(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_xml_c14n_antigua")
	excepciones.SetForTest(t, "xml_c14n_antigua", false)
	firmado := leerFirmaGrxFirmaAnterior(t, "atributos_tab_lf.xsig")
	alterado := bytes.Replace(firmado, []byte("Pago de 120 euros"), []byte("Pago de 920 euros"), 1)
	if bytes.Equal(alterado, firmado) {
		t.Fatal("el fixture no cambió")
	}
	_, _, err := verificarXAdESBytes(alterado)
	if err == nil {
		t.Fatal("el contenido alterado no puede verificarse")
	}
	var block *excepciones.Block
	if errors.As(err, &block) && block.ID == "xml_c14n_antigua" {
		t.Fatalf("el contenido alterado no debe invitar a activar C14N: %v", err)
	}
}

func TestXAdESReferenciaPosteriorAlteradaNoSugiereActivarC14N(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_xml_c14n_antigua")
	excepciones.SetForTest(t, "xml_c14n_antigua", false)
	ruta := filepath.Join("..", "..", "..", "..", "..", "test", "regression", "fixtures", "v1", "samples", "2_xml_signed.xsig")
	firmado, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatal(err)
	}
	alterado := bytes.Replace(firmado, []byte("2026-07-29T02:15:50Z"), []byte("2026-07-29T02:15:51Z"), 1)
	if bytes.Equal(alterado, firmado) {
		t.Fatal("la referencia posterior no cambió")
	}
	ctx := context.WithValue(context.Background(), offlineVerificationContextKey{}, true)
	doc := domain.Document{Name: "muestra.xsig", MIMEType: "application/xml", Content: alterado}
	_, _, err = NewXAdESVerifier().Verify(ctx, doc, domain.CertificateChain{})
	if err == nil {
		t.Fatal("la referencia posterior alterada no puede verificarse")
	}
	var block *excepciones.Block
	if errors.As(err, &block) && block.ID == "xml_c14n_antigua" {
		t.Fatalf("otra referencia alterada no debe invitar a activar C14N: %v", err)
	}
}

func TestXAdESDTDExternaNoEsExcepcionC14N(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_xml_c14n_antigua")
	excepciones.SetForTest(t, "xml_c14n_antigua", false)
	result, _, err := verificarXAdESBytes(leerFirmaGrxFirmaAnterior(t, "doctype_externo.xsig"))
	var block *excepciones.Block
	if errors.As(err, &block) && block.ID == "xml_c14n_antigua" {
		t.Fatalf("una DTD externa no requiere ajuste C14N: %v", err)
	}
	if err != nil || result.Valid || !contieneString(result.Warnings, motivoDOCTYPEExterno) {
		t.Fatalf("la DTD externa debe quedar advertida, sin validez: err=%v avisos=%v", err, result.Warnings)
	}
}

func TestSoloCompatibilidadDTDNoBloqueaC14N(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_xml_c14n_antigua")
	excepciones.SetForTest(t, "xml_c14n_antigua", false)
	result := markXMLCompatibility(domain.NewVerificationSuccess("XAdES", "", nil), []string{detalleDOCTYPEExterno}, context.Background())
	var block *excepciones.Block
	if block = bloqueoTipadoV38(result.PolicyError()); block != nil && block.ID == "xml_c14n_antigua" {
		t.Fatalf("solo DTD externa no debe pedir ajuste C14N: %v", result.Reason)
	}
}

// El DOCTYPE con subconjunto interno sigue rechazado (E1-07), aunque la firma
// sea de una versión anterior.
func TestXAdES_FirmaAnteriorConDTDInternaSigueRechazada(t *testing.T) {
	if _, _, err := verificarXAdESBytes(leerFirmaGrxFirmaAnterior(t, "doctype_interno.xsig")); err == nil {
		t.Fatal("un DOCTYPE con subconjunto interno debe rechazarse")
	}
}

// Lo que cierra la C14N antigua sigue cerrado: cambiar el contenido firmado
// de la firma antigua la invalida, y una firma actual con una hoja de estilo
// añadida después nunca sale válida (como mucho, compatibilidad con el aviso
// de que esa instrucción no estaba firmada).
func TestXAdES_C14NAntiguaNoAbreAtaques(t *testing.T) {
	activarExcepcionPrueba(t, "xml_c14n_antigua")
	antigua := string(leerFirmaGrxFirmaAnterior(t, "atributos_tab_lf.xsig"))
	for nombre, alterado := range map[string]string{
		"contenido": strings.Replace(antigua, "Pago de 120 euros", "Pago de 920 euros", 1),
		"atributo":  strings.Replace(antigua, "A\tB", "A\tC", 1),
	} {
		if result, _, err := verificarXAdESBytes([]byte(alterado)); err == nil && result.Integrity.Status != domain.VerificationStatusInvalid {
			t.Fatalf("%s alterado: integridad=%+v", nombre, result.Integrity)
		}
	}

	firmado := string(operarXAdES(t, []byte(`<pedido><importe>10</importe></pedido>`), "application/xml", domain.ActionSign, map[string]string{"format": "XAdES Enveloped"}, "Victima"))
	raiz := strings.Index(firmado, "<pedido")
	atacante := firmado[:raiz] + `<?xml-stylesheet type="text/xsl" href="https://atacante.example/x.xsl"?>` + firmado[raiz:]
	result, _, err := verificarXAdESBytes([]byte(atacante))
	if err == nil {
		if result.Valid || result.Integrity.Status == domain.VerificationStatusValid {
			t.Fatalf("una hoja de estilo añadida no puede dejar la firma válida: %+v", result.Integrity)
		}
		if !contieneString(result.Warnings, motivoPIFueraDeRaizSinFirmar) {
			t.Fatalf("falta el aviso de instrucción de proceso sin firmar: %v", result.Warnings)
		}
	}
}

// La variante antigua solo cambia lo que cambió E1-07/E1-08.
func TestCanonicalizeXML_VarianteAntiguaGrxFirma(t *testing.T) {
	entrada := "<?xml-stylesheet href=\"a.xsl\"?>\n<a b=\"x\ty\nz\">t&#13;</a>"
	got, err := canonicalizeXMLVariante([]byte(entrada), algExcC14N, c14nAntiguaGrxFirma)
	if err != nil {
		t.Fatal(err)
	}
	if want := `<a b="x&#9;y&#10;z">t&#13;</a>`; string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if _, err := canonicalizeXMLVariante([]byte(`<!DOCTYPE a [<!ATTLIST a b CDATA "1">]><a/>`), algC14N, c14nAntiguaGrxFirma); err == nil {
		t.Fatal("la variante antigua tampoco admite DTD interna")
	}
}
