// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"

	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
)

// certificadoConEmisorOrganizacion crea un certificado cuyo emisor tiene una
// organización distinta de su CN, como los de las autoridades reales.
func certificadoConEmisorOrganizacion(t *testing.T) *x509.Certificate {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true,
		Subject:   pkix.Name{CommonName: "CA QA", Organization: []string{"Organizacion Emisora"}},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageCertSign,
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "Firmante QA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func previewImage(t *testing.T, f *Facade, options map[string]string) []byte {
	t.Helper()
	payload, _ := json.Marshal(sealPreviewRequest{CertificateID: "qa", Options: options})
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
	if err != nil {
		t.Fatal(err)
	}
	return image
}

// La vista previa debe escribir el mismo emisor que la firma PAdES (la
// organización del emisor), no el CN de la autoridad.
func TestSealPreviewJSONUsesSameIssuerAsSignature(t *testing.T) {
	cert := certificadoConEmisorOrganizacion(t)
	fixed := time.Date(2026, 10, 5, 10, 3, 0, 0, time.UTC)
	f := newFacade(nil, nil, nil)
	f.clock = func() time.Time { return fixed }
	f.session = newSessionIdentityStore()
	f.session.identities = []*sessionIdentity{{reference: domain.CertificateRef{ID: "qa", Subject: "Firmante QA", Issuer: "CA QA"}, certificate: cert}}
	options := func() map[string]string {
		return map[string]string{"visibleSeal": "true", "visibleSealRectW": "220", "visibleSealRectH": "70"}
	}
	got := previewImage(t, f, options())
	if desktopsigner.EmisorSelloCertificado(cert) != "Organizacion Emisora" {
		t.Fatalf("emisor inesperado: %q", desktopsigner.EmisorSelloCertificado(cert))
	}
	want, err := desktopsigner.PrevisualizarSello(f.withRegionOptions(options()), "Firmante QA", "Organizacion Emisora", fixed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("la vista previa no usa el emisor que escribe la firma")
	}
	withCN, err := desktopsigner.PrevisualizarSello(f.withRegionOptions(options()), "Firmante QA", "CA QA", fixed)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(got, withCN) {
		t.Fatal("la vista previa sigue usando el CN del emisor")
	}
}

func TestSealPreviewJSONUsesSessionAndRealComposer(t *testing.T) {
	f := newFacade(nil, nil, nil)
	f.session = newSessionIdentityStore()
	f.session.identities = []*sessionIdentity{{reference: domain.CertificateRef{ID: "qa", Subject: "Firmante QA", Issuer: "Emisor QA"}, certificate: certificadoConEmisorOrganizacion(t)}}
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
