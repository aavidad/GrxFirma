// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/eni"
	"grxfirma/internal/domain"
)

// Regla del perfil ENI (lote 2b): en un expediente ENI de verdad, la firma del
// índice es válida aunque metadatosExp, el atributo ref de enids:firma y
// enids:TipoFirma queden fuera, con un aviso que lo dice. Fuera de esa
// estructura exacta, lo no firmado sigue dando cobertura parcial (E1-02).

func expedienteENIFirmado(t *testing.T) (string, *LocalSigningKey) {
	t.Helper()
	priv, cert := certForTest(t, "Expediente ENI")
	clave := &LocalSigningKey{ID: "k", Signer: priv, Certificate: cert}
	ahora := time.Now().UTC().Truncate(time.Second)
	x, err := eni.Generar(eni.Documento{
		Contenido: []byte("%PDF-1.7 uno"), NombreFormato: "PDF",
		Metadatos: eni.Metadatos{Organos: []string{"L01180877"}, EstadoElaboracion: "EE01", TipoDocumental: "TD99"},
		Firmas:    []eni.Firma{{Tipo: eni.FirmaPAdES}},
	}, ahora)
	if err != nil {
		t.Fatal(err)
	}
	firmar := func(nodo []byte, id string) ([]byte, error) { return FirmarNodoXAdES(nodo, id, clave, nil) }
	exp, err := eni.GenerarExpediente(eni.MetadatosExpediente{
		Organos: []string{"L01180877"}, Clasificacion: "L01180877_PRO_SUBVENCIONES", Estado: "E02",
	}, []eni.DocumentoExpediente{{XML: x}}, firmar, CanonicalizarExclusivo, ahora)
	if err != nil {
		t.Fatal(err)
	}
	return string(exp), clave
}

func verificarXAdESTexto(t *testing.T, data string) (domain.VerificationResult, error) {
	t.Helper()
	doc, _ := domain.NewDocument("expediente.xml", []byte(data), "application/xml")
	vr, _, err := NewXAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	return vr, err
}

func TestXAdESVerifier_ExpedienteENIValidoConAviso(t *testing.T) {
	exp, _ := expedienteENIFirmado(t)
	vr, err := verificarXAdESTexto(t, exp)
	if err != nil || !vr.Valid || vr.Coverage != domain.CoberturaCompleta || vr.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("expediente ENI: err=%v valid=%v coverage=%q %v", err, vr.Valid, vr.Coverage, vr.Details)
	}
	detalles := strings.Join(vr.Details, ";")
	if !strings.Contains(detalles, "fuera_de_firma_por_formato=atributos:firma,TipoFirma,metadatosExp") {
		t.Fatalf("el detalle debe decir qué queda fuera: %v", vr.Details)
	}
	if !strings.Contains(strings.Join(vr.Warnings, ";"), motivoFueraDeFirmaPorFormato) {
		t.Fatalf("falta el aviso: %v", vr.Warnings)
	}
}

// Variantes del expediente que se salen de la estructura de la norma: la
// regla deja de aplicarse y lo no firmado vuelve a ser cobertura parcial.
func TestXAdESVerifier_ExpedienteENIFueraDeNormaEsParcial(t *testing.T) {
	exp, _ := expedienteENIFirmado(t)
	casos := map[string]func(string) string{
		"elemento añadido en el índice": func(s string) string {
			return strings.Replace(s, "<enids:firmas>", `<eniexpind:Nota>Pagar 9000 euros</eniexpind:Nota><enids:firmas>`, 1)
		},
		"atributo añadido a enids:firma": func(s string) string {
			return strings.Replace(s, `<enids:firma Id="FIRMA_INDICE"`, `<enids:firma importe="9000" Id="FIRMA_INDICE"`, 1)
		},
		"TipoFirma con texto libre": func(s string) string {
			return strings.Replace(s, "<enids:TipoFirma>TF02</enids:TipoFirma>", "<enids:TipoFirma>Pagar 9000 euros</enids:TipoFirma>", 1)
		},
		"ref que no apunta al índice": func(s string) string {
			return strings.Replace(s, `ref="INDICE_CONTENIDO_`, `ref="OTRO_`, 1)
		},
		"elemento de otro espacio de nombres en metadatosExp": func(s string) string {
			return strings.Replace(s, "<eniexpmeta:metadatosExp>", `<eniexpmeta:metadatosExp><CONTENT xmlns="urn:x">Pagar 9000 euros</CONTENT>`, 1)
		},
		"elemento añadido tras metadatosExp": func(s string) string {
			return strings.Replace(s, "</eniexpmeta:metadatosExp>", `</eniexpmeta:metadatosExp><eniexpmeta:metadatosExp/>`, 1)
		},
		"texto en el expediente": func(s string) string {
			return strings.Replace(s, "<eniexpind:indice ", "Pagar 9000 euros<eniexpind:indice ", 1)
		},
	}
	for nombre, cambio := range casos {
		t.Run(nombre, func(t *testing.T) {
			alterado := cambio(exp)
			if alterado == exp {
				t.Fatal("la variante no cambia el documento")
			}
			vr, err := verificarXAdESTexto(t, alterado)
			noValido(t, vr, err, nombre)
		})
	}
}

// E1-02 con la regla ENI: una firma ajena (de un nodo que no es un índice
// ENI) metida en un falso expediente ENI, con los datos del atacante en
// metadatosExp, no puede salir válida.
func TestXAdESVerifier_FirmaAjenaEnFalsoExpedienteENI(t *testing.T) {
	priv, cert := certForTest(t, "Victima")
	clave := &LocalSigningKey{ID: "k", Signer: priv, Certificate: cert}
	nodo := `<v:Recibi xmlns:v="urn:victima" Id="R1">Recibí 10 euros</v:Recibi>`
	firma, err := FirmarNodoXAdES([]byte(nodo), "R1", clave, nil)
	if err != nil {
		t.Fatal(err)
	}
	falso := func(contenido, ref string) string {
		return `<?xml version="1.0" encoding="UTF-8"?>` +
			`<eniexp:expediente xmlns:eniexp="` + nsENIExpediente + `" xmlns:eniexpind="` + nsENIIndice + `" xmlns:eniexpmeta="` + nsENIMetadatos + `" xmlns:enids="` + nsENIFirma + `" Id="ES_X">` +
			`<eniexpind:indice Id="INDICE_X">` + contenido +
			`<enids:firmas><enids:firma Id="FIRMA_INDICE" ref="` + ref + `"><enids:TipoFirma>TF02</enids:TipoFirma><enids:ContenidoFirma><enids:FirmaConCertificado>` +
			string(firma) +
			`</enids:FirmaConCertificado></enids:ContenidoFirma></enids:firma></enids:firmas></eniexpind:indice>` +
			`<eniexpmeta:metadatosExp><eniexpmeta:Clasificacion>La víctima autoriza un pago de 9000 euros</eniexpmeta:Clasificacion></eniexpmeta:metadatosExp>` +
			`</eniexp:expediente>`
	}
	// El nodo firmado en el sitio de IndiceContenido: la firma cuadra, pero no
	// es un índice ENI.
	vr, err := verificarXAdESTexto(t, falso(nodo, "R1"))
	if err != nil {
		t.Fatalf("la firma ajena debería verificarse criptográficamente para que la prueba tenga sentido: %v", err)
	}
	noValido(t, vr, err, "firma ajena en falso expediente ENI")
	if !strings.Contains(strings.Join(vr.Details, ";"), "metadatosExp") {
		t.Fatalf("metadatosExp debe salir como contenido sin firmar: %v", vr.Details)
	}

	// El nodo firmado dentro de un IndiceContenido de verdad: la firma no
	// cubre el IndiceContenido entero, solo un nodo de dentro.
	vr, err = verificarXAdESTexto(t, falso(`<eniexpind:IndiceContenido Id="IC">`+nodo+`</eniexpind:IndiceContenido>`, "IC"))
	noValido(t, vr, err, "firma ajena dentro de IndiceContenido")
}
