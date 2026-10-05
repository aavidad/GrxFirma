// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
	"time"

	"software.sslmate.com/src/go-pkcs12"
)

func TestMobileHashMatchesDesktopFormats(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	content := []byte("contenido para huella")
	sum := sha256.Sum256(content)
	cases := []struct {
		format, extension string
		output            []byte
	}{
		{"hex", "hexhash", []byte(hex.EncodeToString(sum[:]) + "h")},
		{"base64", "hashb64", []byte(base64.StdEncoding.EncodeToString(sum[:]))},
		{"bin", "hash", sum[:]},
	}
	for _, tc := range cases {
		t.Run(tc.format, func(t *testing.T) {
			raw, err := facade.CreateHashJSON(mustJSON(t, hashCreateRequest{
				ContentBase64: base64.StdEncoding.EncodeToString(content), Algorithm: "SHA-256", Format: tc.format,
			}))
			if err != nil {
				t.Fatal(err)
			}
			var created hashCreateResponse
			decodeResponse(t, raw, &created)
			output, _ := base64.StdEncoding.DecodeString(created.OutputBase64)
			if created.Extension != tc.extension || !bytes.Equal(output, tc.output) || created.Algorithm != "SHA-256" {
				t.Fatalf("huella inesperada: %+v", created)
			}
			check := func(data []byte) hashCheckResponse {
				raw, err := facade.CheckHashJSON(mustJSON(t, hashCheckRequest{
					ContentBase64:  base64.StdEncoding.EncodeToString(data),
					HashFileBase64: created.OutputBase64,
					HashFileName:   "doc.txt." + created.Extension,
				}))
				if err != nil {
					t.Fatal(err)
				}
				var checked hashCheckResponse
				decodeResponse(t, raw, &checked)
				return checked
			}
			if got := check(content); !got.Valid || got.Algorithm != "SHA-256" {
				t.Fatalf("la huella propia no coincide: %+v", got)
			}
			if got := check([]byte("otro")); got.Valid {
				t.Fatal("una huella distinta se dio por buena")
			}
		})
	}
}

func TestMobileHashAlgorithmsAndRejections(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	content := base64.StdEncoding.EncodeToString([]byte("x"))
	for algorithm, size := range map[string]int{"SHA-1": 20, "SHA-384": 48, "SHA-512": 64} {
		raw, err := facade.CreateHashJSON(mustJSON(t, hashCreateRequest{ContentBase64: content, Algorithm: algorithm, Format: "bin"}))
		if err != nil {
			t.Fatal(err)
		}
		var created hashCreateResponse
		decodeResponse(t, raw, &created)
		output, _ := base64.StdEncoding.DecodeString(created.OutputBase64)
		if len(output) != size || created.Algorithm != algorithm {
			t.Fatalf("%s: %+v", algorithm, created)
		}
	}
	if _, err := facade.CreateHashJSON(mustJSON(t, hashCreateRequest{ContentBase64: content, Algorithm: "MD5"})); err == nil {
		t.Fatal("MD5 no debe admitirse")
	}
	if _, err := facade.CreateHashJSON(`{"content_base64":"eA==","extra":1}`); err == nil {
		t.Fatal("campos desconocidos admitidos")
	}
	_, err := facade.CheckHashJSON(mustJSON(t, hashCheckRequest{ContentBase64: content,
		HashFileBase64: base64.StdEncoding.EncodeToString([]byte("abc")), HashFileName: "x.hexhash"}))
	if err == nil || err.Error() != mobileHashFileInvalidMessage {
		t.Fatalf("huella inválida aceptada: %v", err)
	}
	big := base64.StdEncoding.EncodeToString(make([]byte, maxStoredHashBytes+1))
	if _, err := facade.CheckHashJSON(mustJSON(t, hashCheckRequest{ContentBase64: content, HashFileBase64: big})); err == nil {
		t.Fatal("fichero de huella demasiado grande aceptado")
	}
}

func TestMobileProtectRoundTripForSessionCertificate(t *testing.T) {
	for _, container := range []string{"cms", "authenvelopeddata"} {
		t.Run(container, func(t *testing.T) {
			facade := newAndroidFacadeForTest(t)
			p12, _ := encipheringPKCS12(t, "proteger")
			importPKCS12(t, facade, p12, "proteger")
			original := []byte("documento confidencial")
			raw, err := facade.ProtectJSON(mustJSON(t, protectRequest{Name: "nota.txt", MIMEType: "text/plain",
				ContentBase64: base64.StdEncoding.EncodeToString(original), Container: container,
				IncludeSessionCertificate: true}), nil)
			if err != nil {
				t.Fatal(err)
			}
			var protected protectResponse
			decodeResponse(t, raw, &protected)
			if protected.RecipientCount != 1 || protected.Container != container || protected.ContentBase64 == "" {
				t.Fatalf("protección inesperada: %+v", protected)
			}
			opened := unprotectForTest(t, facade, protected, nil)
			if !bytes.Equal(opened, original) {
				t.Fatal("el contenido desprotegido no coincide")
			}
			facade.ClearSession()
			if _, err := facade.UnprotectJSON(mustJSON(t, unprotectRequest{Name: protected.Name,
				MIMEType: protected.MIMEType, ContentBase64: protected.ContentBase64}), nil); err == nil ||
				err.Error() != mobileUnprotectFailedMessage {
				t.Fatalf("se desprotegió sin clave: %v", err)
			}
		})
	}
}

func TestMobileProtectForExternalRecipientOnly(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	_, recipient := encipheringPKCS12(t, "otro")
	raw, err := facade.ProtectJSON(mustJSON(t, protectRequest{Name: "a.bin", MIMEType: "application/octet-stream",
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("datos")), Container: "cms",
		Recipients: []protectionRecipientRequest{{CertificateBase64: base64.StdEncoding.EncodeToString(recipient.Raw)}}}), nil)
	if err != nil {
		t.Fatal(err)
	}
	var protected protectResponse
	decodeResponse(t, raw, &protected)
	if protected.RecipientCount != 1 {
		t.Fatalf("destinatarios = %d", protected.RecipientCount)
	}
	// Un destinatario que solo firma no puede recibir el sobre.
	signingOnly := ephemeralRSAPKCS12(t, "firma")
	_, certificate, err := pkcs12.Decode(signingOnly, "firma")
	if err != nil {
		t.Fatal(err)
	}
	_, err = facade.ProtectJSON(mustJSON(t, protectRequest{Name: "a.bin", MIMEType: "application/octet-stream",
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("datos")),
		Recipients:    []protectionRecipientRequest{{CertificateBase64: base64.StdEncoding.EncodeToString(certificate.Raw)}}}), nil)
	if err == nil || err.Error() != mobileProtectionRecipientMessage {
		t.Fatalf("destinatario sin cifrado aceptado: %v", err)
	}
}

func TestMobileEncryptedDataWithTransientKey(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	keyBytes := bytes.Repeat([]byte{7}, 32)
	key := func() []byte { return []byte(base64.StdEncoding.EncodeToString(keyBytes)) }
	original := []byte("compartido con clave")
	secret := key()
	raw, err := facade.ProtectJSON(mustJSON(t, protectRequest{Name: "c.txt", MIMEType: "text/plain",
		ContentBase64: base64.StdEncoding.EncodeToString(original), Container: "cms-encrypted"}), secret)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(secret, make([]byte, len(secret))) {
		t.Fatal("la clave no se borró tras usarse")
	}
	var protected protectResponse
	decodeResponse(t, raw, &protected)
	if !strings.HasSuffix(protected.Name, ".encrypted.p7m") {
		t.Fatalf("nombre inesperado: %s", protected.Name)
	}
	if got := unprotectForTest(t, facade, protected, key()); !bytes.Equal(got, original) {
		t.Fatal("EncryptedData no se abrió con su clave")
	}
	wrong := []byte(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32)))
	if _, err := facade.UnprotectJSON(mustJSON(t, unprotectRequest{Name: protected.Name, MIMEType: protected.MIMEType,
		ContentBase64: protected.ContentBase64}), wrong); err == nil || err.Error() != mobileUnprotectFailedMessage {
		t.Fatalf("clave incorrecta aceptada: %v", err)
	}
	for _, bad := range [][]byte{[]byte("corta"), []byte(strings.Repeat("A", 43) + "B")} {
		_, err := facade.ProtectJSON(mustJSON(t, protectRequest{Name: "c.txt", MIMEType: "text/plain",
			ContentBase64: base64.StdEncoding.EncodeToString(original), Container: "cms-encrypted"}), bad)
		if err == nil || err.Error() != mobileProtectionKeyInvalidMessage {
			t.Fatalf("clave no canónica aceptada: %v", err)
		}
	}
	if _, err := facade.ProtectJSON(mustJSON(t, protectRequest{Name: "c.txt", MIMEType: "text/plain",
		ContentBase64: base64.StdEncoding.EncodeToString(original), Container: "cms-encrypted"}), nil); err == nil {
		t.Fatal("EncryptedData sin clave aceptado")
	}
	if _, err := facade.ProtectJSON(mustJSON(t, protectRequest{Name: "c.txt", MIMEType: "text/plain",
		ContentBase64: base64.StdEncoding.EncodeToString(original), Container: "compressed"}), nil); err == nil {
		t.Fatal("contenedor no declarado aceptado")
	}
}

func TestMobileProtectAndSignWithSessionIdentity(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	p12, _ := encipheringPKCS12(t, "remitente")
	id := importPKCS12(t, facade, p12, "remitente")
	raw, err := facade.ProtectJSON(mustJSON(t, protectRequest{Name: "f.txt", MIMEType: "text/plain",
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("firmado y cifrado")), Sign: true,
		CertificateID: id, IncludeSessionCertificate: true}), nil)
	if err != nil {
		t.Fatal(err)
	}
	var protected protectResponse
	decodeResponse(t, raw, &protected)
	if protected.Container != "signedandenvelopeddata" || protected.CertificateID != id {
		t.Fatalf("proteger y firmar inesperado: %+v", protected)
	}
	if _, err := facade.ProtectJSON(mustJSON(t, protectRequest{Name: "f.txt", MIMEType: "text/plain",
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("x")), Sign: true, Container: "cms",
		CertificateID: id, IncludeSessionCertificate: true}), nil); err == nil {
		t.Fatal("proteger y firmar con otro contenedor aceptado")
	}
}

func TestMobileBatchSignsEachItemAndReportsByPosition(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	p12 := ephemeralRSAPKCS12(t, "lote")
	id := importPKCS12(t, facade, p12, "lote")
	items := []batchItemRequest{
		{Name: "uno.txt", MIMEType: "text/plain", ContentBase64: base64.StdEncoding.EncodeToString([]byte("uno")), Format: "cades"},
		{Name: "dos.xml", MIMEType: "application/xml", ContentBase64: base64.StdEncoding.EncodeToString([]byte("<a>dos</a>")), Format: "xades"},
	}
	raw, err := facade.ProcessBatchJSON(mustJSON(t, mobileBatchRequest{Items: items, CertificateID: id}))
	if err != nil {
		t.Fatal(err)
	}
	var response mobileBatchResponse
	decodeResponse(t, raw, &response)
	if !response.OK || len(response.Items) != 2 || response.CertificateID != id {
		t.Fatalf("lote inesperado: %+v", response)
	}
	for index, item := range response.Items {
		if !item.OK || item.Index != index || item.SignedContentBase64 == "" {
			t.Fatalf("documento %d: %+v", index, item)
		}
	}
	// Con ECDSA, XAdES falla solo en su posición y CAdES sigue firmándose.
	ecdsaID := importEphemeralIdentity(t, facade, "lote-ec")
	raw, err = facade.ProcessBatchJSON(mustJSON(t, mobileBatchRequest{Items: items, CertificateID: ecdsaID}))
	if err != nil {
		t.Fatal(err)
	}
	response = mobileBatchResponse{}
	decodeResponse(t, raw, &response)
	if response.OK || !response.Items[0].OK || response.Items[1].OK || response.Items[1].Error == "" {
		t.Fatalf("resultado parcial inesperado: %+v", response)
	}
	var contract mobileContract
	decodeResponse(t, facade.MobileContractJSON(), &contract)
	if !contract.Services["process_batch"] || contract.Services["remote_exchange"] || contract.Limits.BatchItems != maxBatchItems {
		t.Fatalf("contrato de lote inesperado: %+v", contract)
	}
}

func TestMobileBatchKeepsSecurityLimits(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	id := importEphemeralIdentity(t, facade, "limites")
	item := batchItemRequest{Name: "a.txt", MIMEType: "text/plain", ContentBase64: base64.StdEncoding.EncodeToString([]byte("a"))}
	tooMany := make([]batchItemRequest, maxBatchItems+1)
	for i := range tooMany {
		tooMany[i] = item
	}
	cases := map[string]string{
		"vacío":          mustJSON(t, mobileBatchRequest{CertificateID: id}),
		"demasiados":     mustJSON(t, mobileBatchRequest{Items: tooMany, CertificateID: id}),
		"sesión":         `{"certificate_id":"` + id + `","items":[{"name":"a.txt","content_base64":"YQ==","mime_type":"text/plain","format":"","action":"","options":null}],"session":{"request_id":"r"}}`,
		"ruta":           mustJSON(t, mobileBatchRequest{CertificateID: id, Items: []batchItemRequest{{Name: "../a.txt", MIMEType: "text/plain", ContentBase64: "YQ=="}}}),
		"perfil sin tsa": mustJSON(t, mobileBatchRequest{CertificateID: id, Items: []batchItemRequest{item}, Options: map[string]string{"profile": "t"}}),
		"pades contrafirma": mustJSON(t, mobileBatchRequest{CertificateID: id, Items: []batchItemRequest{{Name: "a.pdf",
			MIMEType: "application/pdf", ContentBase64: "YQ==", Action: "countersign"}}}),
	}
	for name, payload := range cases {
		if _, err := facade.ProcessBatchJSON(payload); err == nil {
			t.Fatalf("%s: lote inseguro aceptado", name)
		}
	}
	half := base64.StdEncoding.EncodeToString(make([]byte, maxBatchInputBytes/2+1))
	big := mobileBatchRequest{CertificateID: id, Items: []batchItemRequest{
		{Name: "a.bin", MIMEType: "application/octet-stream", ContentBase64: half},
		{Name: "b.bin", MIMEType: "application/octet-stream", ContentBase64: half},
	}}
	if _, err := facade.ProcessBatchJSON(mustJSON(t, big)); err == nil {
		t.Fatal("lote por encima del tamaño total aceptado")
	}
}

func unprotectForTest(t *testing.T, facade *Facade, protected protectResponse, secret []byte) []byte {
	t.Helper()
	raw, err := facade.UnprotectJSON(mustJSON(t, unprotectRequest{Name: protected.Name, MIMEType: protected.MIMEType,
		ContentBase64: protected.ContentBase64}), secret)
	if err != nil {
		t.Fatal(err)
	}
	var opened unprotectResponse
	decodeResponse(t, raw, &opened)
	content, err := base64.StdEncoding.DecodeString(opened.ContentBase64)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

// encipheringPKCS12 genera una identidad efímera apta para firmar y para
// recibir sobres CMS (digitalSignature + keyEncipherment).
func encipheringPKCS12(t *testing.T, password string) ([]byte, *x509.Certificate) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               pkix.Name{CommonName: "Destinatario de prueba"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	p12, err := pkcs12.Modern.Encode(privateKey, certificate, nil, password)
	if err != nil {
		t.Fatal(err)
	}
	return p12, certificate
}
