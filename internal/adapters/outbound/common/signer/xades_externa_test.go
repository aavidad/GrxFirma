// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

// Caso de Navarra: XAdES Externally Detached con Manifest y huella SHA-256
// precalculada de un documento identificado por URN.
func TestXAdES_ExternallyDetachedManifestComoNavarra(t *testing.T) {
	documento := []byte("<?xml version='1.0'?><solicitud>3086</solicitud>")
	huella := sha256.Sum256(documento)
	firmado := operarXAdES(t, huella[:], "application/octet-stream", domain.ActionSign, map[string]string{
		"format": "XAdES Externally Detached", "useManifest": "true",
		"precalculatedHashAlgorithm": "SHA-256", "uri": "urn:id:3086", "algorithm": "SHA256withRSA",
	}, "Navarra")
	texto := string(firmado)
	for _, esperado := range []string{
		`URI="urn:id:3086"`, base64.StdEncoding.EncodeToString(huella[:]),
		`Type="` + typeXMLDSigManifest + `"`, "<ds:Manifest",
	} {
		if !strings.Contains(texto, esperado) {
			t.Fatalf("falta %q en la firma:\n%s", esperado, texto)
		}
	}
	exigirFirmantesXAdES(t, firmado, 1)

	uno, dos := sha256.Sum256([]byte("a")), sha256.Sum256([]byte("b"))
	multiple := operarXAdES(t, []byte("ignorado"), "application/octet-stream", domain.ActionSign, map[string]string{
		"format": "XAdES Externally Detached", "useManifest": "true", "algorithm": "SHA256withRSA",
		"uri1": "urn:a", "md1": base64.StdEncoding.EncodeToString(uno[:]),
		"uri2": "urn:b", "md2": base64.StdEncoding.EncodeToString(dos[:]),
	}, "Manifest")
	if strings.Count(string(multiple), `URI="urn:`) != 2 {
		t.Fatalf("el manifest debe contener las dos referencias:\n%s", multiple)
	}
	exigirFirmantesXAdES(t, multiple, 1)
}

func TestXAdES_ExternallyDetachedSinManifest(t *testing.T) {
	datos := []byte("contenido externo")
	firmado := operarXAdES(t, datos, "text/plain", domain.ActionSign, map[string]string{
		"format": "XAdES Externally Detached", "uri": "https://sede.example.es/doc/1", "algorithm": "SHA256withRSA",
	}, "Externa")
	huella := sha256.Sum256(datos)
	if !strings.Contains(string(firmado), `URI="https://sede.example.es/doc/1"`) ||
		!strings.Contains(string(firmado), base64.StdEncoding.EncodeToString(huella[:])) {
		t.Fatalf("referencia externa incorrecta:\n%s", firmado)
	}
	priv, cert := certForTest(t, "mal")
	doc, _ := domain.NewDocument("h", []byte("corta"), "application/octet-stream")
	if _, err := NewXAdESBESDetached().Sign(t.Context(), domain.SignatureJob{
		Document: doc, Format: domain.FormatXAdES, Action: domain.ActionSign,
		Options: map[string]string{"format": "XAdES Externally Detached", "uri": "urn:x", "precalculatedHashAlgorithm": "SHA-256"},
	}, &LocalSigningKey{ID: "k", Signer: priv, Certificate: cert}); err == nil {
		t.Fatal("una huella precalculada de longitud incorrecta debe rechazarse")
	}
}
