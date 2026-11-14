// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"testing"

	"grxfirma/internal/domain"
)

// Lote 17a (muestra externally_detached.xsig): una XAdES que referencia un
// documento por http:// no es un error. GrxFirma no descarga nada, así que esa
// referencia queda sin comprobar y la firma no sale válida.
func firmaXAdESConReferenciaHTTP(t *testing.T) []byte {
	t.Helper()
	return operarXAdES(t, []byte("contenido externo"), "text/plain", domain.ActionSign, map[string]string{
		"format": "XAdES Externally Detached", "uri": "http://sede.example.es/doc/1.pdf", "algorithm": "SHA256withRSA",
	}, "Externa")
}

func TestXAdES_ReferenciaExternaQuedaSinComprobar(t *testing.T) {
	firmado := firmaXAdESConReferenciaHTTP(t)
	vr := exigirCoberturaParcialXAdES(t, firmado, 1, domain.CoberturaNoAcreditada)
	if vr.Integrity.Status != domain.VerificationStatusUnknown || vr.Reason != motivoReferenciaExternaNoComprobable {
		t.Fatalf("integridad=%s motivo=%q", vr.Integrity.Status, vr.Reason)
	}
	if contieneString(vr.Details, "referencias=ok") {
		t.Fatalf("una referencia sin comprobar no puede dejar «referencias=ok»: %v", vr.Details)
	}

	// Lo que sí está en el fichero se sigue comprobando: unas propiedades
	// firmadas alteradas son un error.
	alterado := bytes.Replace(firmado, []byte("<xades:SigningTime>"), []byte("<xades:SigningTime>1"), 1)
	if bytes.Equal(alterado, firmado) {
		t.Fatal("la prueba no altera nada")
	}
	doc, _ := domain.NewDocument("f.xsig", alterado, "application/xml")
	if _, _, err := NewXAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{}); err == nil {
		t.Fatal("una firma alterada con una referencia externa no puede pasar")
	}
	// Y la URI externa está firmada: cambiarla rompe la firma.
	otraURI := bytes.Replace(firmado, []byte("http://sede.example.es/doc/1.pdf"), []byte("http://sede.example.es/doc/2.pdf"), 1)
	doc, _ = domain.NewDocument("f.xsig", otraURI, "application/xml")
	if _, _, err := NewXAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{}); err == nil {
		t.Fatal("cambiar la URI externa firmada no puede pasar")
	}
}

// Fuera de XAdES y XMLdSig sueltas (FacturaE, Veri*Factu, contenedores, cotejo
// ENI) una referencia externa sin resolver sigue siendo error.
func TestXAdES_ReferenciaExternaSigueSiendoErrorFueraDeXAdES(t *testing.T) {
	firmado := firmaXAdESConReferenciaHTTP(t)
	if _, err := verifyXMLSignatureDocument(context.Background(), firmado, true, nil, domain.CertificateChain{}, "FacturaE", "", nil, coberturaDelDocumento); err == nil {
		t.Fatal("sin activarlo, una referencia externa sin resolver es un error")
	}
}

// Los Manifest de AutoFirma con referencias urn: (xades_con_manifest.xsig,
// firma_xades_manifest.xsig) dicen ahora por qué no se comprueban.
func TestXAdES_ManifestConReferenciasExternasDiceQueSonExternas(t *testing.T) {
	firmado := operarXAdES(t, []byte("ignorado"), "application/octet-stream", domain.ActionSign, map[string]string{
		"format": "XAdES Externally Detached", "useManifest": "true", "algorithm": "SHA256withRSA",
		"uri1": "urn:a", "md1": "ypeBEsobvcr6wjGzmiPcTaeG7/gUfE5yuYB3ha/uSLs=",
	}, "Manifest")
	vr := exigirCoberturaParcialXAdES(t, firmado, 1, domain.CoberturaNoAcreditada)
	if vr.Reason != motivoReferenciaExternaNoComprobable {
		t.Fatalf("motivo=%q", vr.Reason)
	}
}

// Si además de una referencia externa hay algo no comprobado de otro tipo,
// el motivo es el general.
func TestSoloReferenciasExternas(t *testing.T) {
	casos := []struct {
		motivos []string
		want    bool
	}{
		{nil, false},
		{[]string{detalleReferenciaExterna}, true},
		{[]string{detalleReferenciaExterna, "manifest_referencia_externa"}, true},
		{[]string{detalleReferenciaExterna, "manifest_relationship_transform"}, false},
	}
	for _, c := range casos {
		if got := soloReferenciasExternas(c.motivos); got != c.want {
			t.Fatalf("%v: %v, want %v", c.motivos, got, c.want)
		}
	}
}

// Con SHA-1 y la integridad sin comprobar no se dice que el documento no ha
// cambiado.
func TestCoberturaNoAcreditadaQuitaElAvisoSHA1(t *testing.T) {
	res := domain.NewVerificationSuccess("XAdES", "x", nil)
	res.Warnings = []string{motivoAlgoritmoAntiguo}
	res = aplicarCoberturaXML(res, resultadoCobertura{noComprobado: []string{detalleReferenciaExterna}})
	if contieneString(res.Warnings, motivoAlgoritmoAntiguo) {
		t.Fatalf("avisos=%v", res.Warnings)
	}
}
