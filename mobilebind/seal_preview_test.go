// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

func TestSealPreviewJSONUsesSessionAndRealComposer(t *testing.T) {
	f := newFacade(nil, nil, nil)
	f.session = newSessionIdentityStore()
	f.session.identity = &sessionIdentity{reference: domain.CertificateRef{ID: "qa", Subject: "Firmante QA", Issuer: "Emisor QA"}}
	payload, _ := json.Marshal(sealPreviewRequest{CertificateID: "qa", Options: map[string]string{
		"visibleSeal": "true", "visibleSealRectW": "220", "visibleSealRectH": "70",
		"visibleSealLogoOpacityPercent": "50", "rotation": "30",
	}})
	response, err := f.SealPreviewJSON(string(payload))
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		ImageBase64 string `json:"image_base64"`
	}
	if err := json.Unmarshal([]byte(response), &decoded); err != nil {
		t.Fatal(err)
	}
	image, err := base64.StdEncoding.DecodeString(decoded.ImageBase64)
	if err != nil || len(image) < 8 || string(image[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatal("la vista previa no devolvió un PNG")
	}
	if _, err := f.SealPreviewJSON(`{"certificate_id":"otra","options":{"visibleSeal":"true"}}`); err == nil {
		t.Fatal("se aceptó un certificado ajeno a la sesión")
	}
	if _, err := f.SealPreviewJSON(`{"certificate_id":"qa","options":{"visibleSeal":"true","qrContent":"http://example.org"}}`); err == nil {
		t.Fatal("se aceptó QR HTTP")
	}
}

func TestMobileSealOptionsBoundaries(t *testing.T) {
	if err := validateOptions(map[string]string{"visibleSealPlacements": strings.Repeat("a", 4096)}); err != nil {
		t.Fatalf("el contrato rechazó una lista dentro de su límite: %v", err)
	}
	if err := validateOptions(map[string]string{"visibleSealImagePath": "/tmp/logo.png"}); err == nil {
		t.Fatal("se aceptó una ruta local de imagen")
	}
}
