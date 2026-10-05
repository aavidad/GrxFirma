// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"fmt"
	"image/png"
	"math/big"
	"strings"
	"testing"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
)

func TestSessionKeepsSeveralIdentitiesAndSignsWithTheChosenOne(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	first := importEphemeralIdentity(t, facade, "uno")
	second := importEphemeralIdentity(t, facade, "dos")
	if first == second {
		t.Fatal("dos PKCS#12 distintos no pueden compartir identificador")
	}
	var details certificateDetailsResponse
	raw, err := facade.CertificateDetailsJSON()
	if err != nil {
		t.Fatal(err)
	}
	decodeResponse(t, raw, &details)
	if len(details.Certificates) != 2 || details.Certificates[0].CertificateID != first || details.Certificates[1].CertificateID != second {
		t.Fatalf("la sesión debe listar ambos certificados en orden de apertura: %+v", details.Certificates)
	}
	for _, id := range []string{first, second} {
		payload := mustJSON(t, signRequest{
			Name: "nota.txt", ContentBase64: base64.StdEncoding.EncodeToString([]byte("contenido")),
			MIMEType: "text/plain", Format: "cades", Action: "sign", CertificateID: id, Options: map[string]string{},
		})
		out, err := facade.SignJSON(payload)
		if err != nil {
			t.Fatalf("firma con %s: %v", id[:8], err)
		}
		var response signResponse
		decodeResponse(t, out, &response)
		if response.CertificateID != id {
			t.Fatalf("se firmó con otro certificado: %s", response.CertificateID)
		}
	}

	var contract mobileContract
	decodeResponse(t, facade.MobileContractJSON(), &contract)
	if contract.IdentityStore.MaxIdentities != maxSessionIdentities || !contract.Services["session_identities"] {
		t.Fatalf("el contrato debe declarar varias identidades: %+v", contract.IdentityStore)
	}
}

func TestRemoveSessionIdentityClosesOnlyThatOne(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	first := importEphemeralIdentity(t, facade, "uno")
	second := importEphemeralIdentity(t, facade, "dos")
	raw, err := facade.RemoveSessionIdentityJSON(mustJSON(t, certificateRequest{CertificateID: first}))
	if err != nil {
		t.Fatal(err)
	}
	var response removeIdentityResponse
	decodeResponse(t, raw, &response)
	if !response.Removed || response.Remaining != 1 {
		t.Fatalf("respuesta: %+v", response)
	}
	if _, err := facade.session.KeyFor(context.Background(), domain.CertificateRef{ID: first}); err == nil {
		t.Fatal("el certificado cerrado no puede seguir firmando")
	}
	if _, err := facade.session.KeyFor(context.Background(), domain.CertificateRef{ID: second}); err != nil {
		t.Fatalf("el otro certificado debe seguir abierto: %v", err)
	}
	if _, err := facade.RemoveSessionIdentityJSON(mustJSON(t, certificateRequest{CertificateID: first})); err == nil {
		t.Fatal("cerrar dos veces el mismo certificado debe fallar")
	}
	for _, bad := range []string{``, `{}`, `{"certificate_id":""}`, `{"certificate_id":"x","otro":1}`} {
		if _, err := facade.RemoveSessionIdentityJSON(bad); err == nil {
			t.Fatalf("petición no válida aceptada: %q", bad)
		}
	}
	facade.ClearSession()
	if facade.session.count() != 0 {
		t.Fatal("ClearSession debe cerrar todos los certificados")
	}
}

func TestReimportingTheSameIdentityDoesNotDuplicateIt(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	p12, _ := ephemeralPKCS12(t, "clave")
	copyOfP12 := append([]byte(nil), p12...)
	first := importPKCS12(t, facade, p12, "clave")
	second := importPKCS12(t, facade, copyOfP12, "clave")
	if first != second || facade.session.count() != 1 {
		t.Fatalf("reimportar debe sustituir, no duplicar: %d identidades", facade.session.count())
	}
}

func TestSessionRejectsMoreThanTheMaximumIdentities(t *testing.T) {
	store := newSessionIdentityStore()
	for index := 0; index < maxSessionIdentities; index++ {
		if err := store.add(&sessionIdentity{reference: domain.CertificateRef{ID: fmt.Sprintf("id-%d", index)}}, false); err != nil {
			t.Fatalf("identidad %d rechazada: %v", index, err)
		}
	}
	extra := &sessionIdentity{reference: domain.CertificateRef{ID: "sobrante"}}
	if err := store.add(extra, false); !errors.Is(err, errMobileSessionFull) {
		t.Fatalf("se esperaba sesión llena, got %v", err)
	}
	// Sustituir una ya abierta no cuenta como nueva.
	if err := store.add(&sessionIdentity{reference: domain.CertificateRef{ID: "id-3"}}, false); err != nil {
		t.Fatalf("sustituir una identidad abierta: %v", err)
	}
	if store.count() != maxSessionIdentities {
		t.Fatalf("recuento: %d", store.count())
	}
	if got := mobileCertificateImportErrorMessage(errMobileSessionFull); got != mobileSessionFullKey {
		t.Fatalf("la sesión llena debe devolver la clave cerrada, got %q", got)
	}
}

func TestDnieReplacesPreviousDnieButKeepsPKCS12(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	pkcs12ID := importEphemeralIdentity(t, facade, "clave")
	install := func(serial int64) string {
		private, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{
			SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "DNIe simulado"},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
			KeyUsage: x509.KeyUsageContentCommitment,
		}
		der, err := x509.CreateCertificate(rand.Reader, template, template, &private.PublicKey, private)
		if err != nil {
			t.Fatal(err)
		}
		ref, err := facade.session.installExternalIdentity(der, nil, &simulatedCard{private: private})
		if err != nil {
			t.Fatal(err)
		}
		return ref.ID
	}
	firstCard := install(1)
	secondCard := install(2)
	if facade.session.has(firstCard) || !facade.session.has(secondCard) || !facade.session.has(pkcs12ID) {
		t.Fatal("el DNIe nuevo debe sustituir al anterior y conservar el PKCS#12")
	}
	snapshots := facade.session.snapshots()
	if len(snapshots) != 2 || snapshots[0].external || !snapshots[1].external {
		t.Fatalf("instantáneas: %+v", snapshots)
	}
}

func TestProtectAndSignNeedsTheCertificateWhenSeveralAreOpen(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	importPKCS12(t, facade, ephemeralRSAPKCS12(t, "uno"), "uno")
	importPKCS12(t, facade, ephemeralRSAPKCS12(t, "dos"), "dos")
	if facade.session.hasSigningIdentity("") {
		t.Fatal("con varias identidades no se puede elegir una sin identificador")
	}
	if _, ok := facade.session.certificateDER(""); ok {
		t.Fatal("con varias identidades no se puede elegir el destinatario propio sin identificador")
	}
	keys, err := mobileSessionDecryptionKeys{facade.session}.DecryptionKeys(context.Background())
	if err != nil || len(keys) != 2 {
		t.Fatalf("desproteger debe probar con todas las claves RSA abiertas: %d, %v", len(keys), err)
	}
	for _, key := range keys {
		zeroBytes(key.RSAOAEP256PrivateKeyPKCS8)
	}
}

func veriFactuQRPNG(t *testing.T, content string) []byte {
	t.Helper()
	code, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, code.Image(400)); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestReadVeriFactuQRImageJSONDecodesAndWipesTheImage(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	image := veriFactuQRPNG(t, testVeriFactuQR)
	raw, err := facade.ReadVeriFactuQRImageJSON(image)
	if err != nil {
		t.Fatal(err)
	}
	var qr commonsigner.VeriFactuQR
	decodeResponse(t, raw, &qr)
	if qr.NIF != "89890001K" || qr.Number != "ABC&G33" || qr.Amount != "241.4" {
		t.Fatalf("datos del QR: %+v", qr)
	}
	if !bytes.Equal(image, make([]byte, len(image))) {
		t.Fatal("la imagen debe borrarse tras leerla")
	}
}

func TestReadVeriFactuQRImageJSONReturnsClosedKeys(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	cases := []struct {
		name  string
		input []byte
		key   string
	}{
		{"vacía", nil, "verifactu.qr_image"},
		{"no es imagen", []byte("%PDF-1.7 no es una imagen"), "verifactu.qr_image"},
		{"demasiado grande", make([]byte, commonsigner.VeriFactuQRMaxImageBytes+1), "verifactu.qr_image"},
		{"QR ajeno", veriFactuQRPNG(t, "https://example.org/otra-cosa"), "verifactu.qr_url"},
	}
	for _, tc := range cases {
		_, err := facade.ReadVeriFactuQRImageJSON(tc.input)
		if err == nil || err.Error() != tc.key {
			t.Fatalf("%s: se esperaba %s, got %v", tc.name, tc.key, err)
		}
	}
	facade.veriFactuImageRead = func(context.Context, []byte) (commonsigner.VeriFactuQR, error) {
		return commonsigner.VeriFactuQR{}, errors.New("detalle interno /ruta/privada")
	}
	_, err := facade.ReadVeriFactuQRImageJSON([]byte{0x89, 'P', 'N', 'G'})
	if err == nil || err.Error() != "verifactu.qr_image" || strings.Contains(err.Error(), "ruta") {
		t.Fatalf("un error interno debe quedar en la clave genérica: %v", err)
	}
}
