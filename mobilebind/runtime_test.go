// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"grxfirma/internal/testsupport/pdffixture"

	"software.sslmate.com/src/go-pkcs12"
)

func TestAndroidFacadeRealCryptoRoundTrip(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	certificateID := importEphemeralIdentity(t, facade, "clave-prueba")

	selectedRaw, err := facade.SelectCertificateJSON(
		`{"subject_filter":"Mobile Test","issuer_filter":"","solo_no_caducados":true}`,
	)
	if err != nil {
		t.Fatalf("SelectCertificateJSON() error = %v", err)
	}
	var selected selectCertificateResponse
	decodeResponse(t, selectedRaw, &selected)
	if selected.CertificateID != certificateID || !selected.Confirmed || selected.Fingerprint == "" {
		t.Fatalf("seleccion inesperada: %+v", selected)
	}

	t.Run("cades detached", func(t *testing.T) {
		original := []byte("expediente administrativo 2026")
		signed := signWithFacade(t, facade, certificateID, "expediente.txt", "text/plain", "cades", original)
		defer zeroBytes(signed)

		verification := verifyWithFacade(
			t,
			facade,
			"expediente.p7s",
			"application/pkcs7-signature",
			signed,
			original,
		)
		assertCryptographicVerification(t, verification, "CAdES")
	})

	t.Run("pades embedded and automatic format", func(t *testing.T) {
		original := pdffixture.Minimal()
		signed := signWithFacade(t, facade, certificateID, "expediente.pdf", "application/pdf", "auto", original)
		defer zeroBytes(signed)
		if !strings.HasPrefix(string(signed), "%PDF-") {
			t.Fatal("la firma automatica de un PDF no produjo PAdES")
		}

		verification := verifyWithFacade(
			t,
			facade,
			"expediente-firmado.pdf",
			"application/pdf",
			signed,
			nil,
		)
		assertCryptographicVerification(t, verification, "PAdES")
	})

	t.Run("xades embedded with RSA identity", func(t *testing.T) {
		p12 := ephemeralRSAPKCS12(t, "xades-rsa")
		defer zeroBytes(p12)
		rsaCertificateID := importPKCS12(t, facade, p12, "xades-rsa")
		original := []byte(`<expediente id="E-2026-1"><asunto>Prueba mobile</asunto></expediente>`)
		signed := signWithFacade(t, facade, rsaCertificateID, "expediente.xml", "application/xml", "auto", original)
		defer zeroBytes(signed)
		if !strings.Contains(string(signed), "xades:QualifyingProperties") {
			t.Fatal("la firma automatica XML no produjo XAdES")
		}

		verification := verifyWithFacade(
			t,
			facade,
			"expediente-firmado.xml",
			"application/xml",
			signed,
			nil,
		)
		assertCryptographicVerification(t, verification, "XAdES")
	})

	facade.ClearSession()
	if _, err := facade.SelectCertificateJSON(`{"solo_no_caducados":true}`); err == nil {
		t.Fatal("ClearSession() debe retirar la identidad del catalogo")
	}
}

func TestAndroidFacadePAdESUsesPrivateTemporaryDirectory(t *testing.T) {
	filesDirectory := t.TempDir()
	noBackupDirectory := t.TempDir()
	facade, err := NewAndroidFacade(filesDirectory, noBackupDirectory)
	if err != nil {
		t.Fatalf("NewAndroidFacade() error = %v", err)
	}
	certificateID := importEphemeralIdentity(t, facade, "temporal-privado")

	invalidSystemTemporaryDirectory := filepath.Join(t.TempDir(), "no-existe")
	t.Setenv("TMPDIR", invalidSystemTemporaryDirectory)
	signed := signWithFacade(
		t,
		facade,
		certificateID,
		"expediente.pdf",
		"application/pdf",
		"pades",
		pdffixture.Minimal(),
	)
	defer zeroBytes(signed)
	if os.Getenv("TMPDIR") != invalidSystemTemporaryDirectory {
		t.Fatal("la firma PAdES no restauro TMPDIR")
	}

	privateTemporaryDirectory := filepath.Join(noBackupDirectory, ".grxfirma-tmp")
	info, err := os.Stat(privateTemporaryDirectory)
	if err != nil {
		t.Fatalf("Stat() directorio temporal error = %v", err)
	}
	assertPrivateDirectoryMode(t, info)
	entries, err := os.ReadDir(privateTemporaryDirectory)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("el motor dejo %d temporales tras firmar", len(entries))
	}
}

func TestImportedMobileIdentityDeclaresItsDecodedSigningKey(t *testing.T) {
	p12, _ := ephemeralPKCS12(t, "clave-presente")
	defer zeroBytes(p12)
	identity, err := decodePKCS12Identity(p12, "clave-presente")
	if err != nil {
		t.Fatalf("decodePKCS12Identity() error = %v", err)
	}
	defer destroySessionIdentity(identity)
	if !identity.reference.HasSigningKey {
		t.Fatal("la identidad PKCS#12 decodificada debe declarar su clave de firma")
	}
}

func TestPlatformContractsAreHonest(t *testing.T) {
	android := newAndroidFacadeForTest(t)
	var androidContract mobileContract
	decodeResponse(t, android.MobileContractJSON(), &androidContract)
	if androidContract.Platform != "android" || androidContract.ContractVersion != mobileContractVersion {
		t.Fatalf("contrato Android inesperado: %+v", androidContract)
	}
	for _, service := range []string{"sign", "verify", "select_certificate", "import_certificate", "external_signer"} {
		if !androidContract.Services[service] {
			t.Fatalf("servicio Android no declarado: %s", service)
		}
	}
	if androidContract.IdentityStore.Persistent || androidContract.IdentityStore.Mode != "memory_session" {
		t.Fatalf("persistencia Android declarada incorrectamente: %+v", androidContract.IdentityStore)
	}
	if androidContract.Verification.SystemTrustAnchors {
		t.Fatal("el contrato no debe afirmar anclas de confianza del sistema")
	}
	if got := androidContract.Signing.KeyTypesByFormat["XAdES"]; len(got) != 1 || got[0] != "RSA" {
		t.Fatalf("compatibilidad XAdES declarada incorrectamente: %+v", androidContract.Signing)
	}
	profileRaw, err := android.ResolvePlatformProfileJSON()
	if err != nil {
		t.Fatalf("ResolvePlatformProfileJSON() error = %v", err)
	}
	var profile platformProfileResponse
	decodeResponse(t, profileRaw, &profile)
	if !profile.HasTemporaryStorage || profile.HasSecureStorage {
		t.Fatalf("perfil Android inesperado: %+v", profile)
	}

	iosDir := t.TempDir()
	ios, err := NewIOSFacade(iosDir, "", "")
	if err != nil {
		t.Fatalf("NewIOSFacade() error = %v", err)
	}
	var iosContract mobileContract
	decodeResponse(t, ios.MobileContractJSON(), &iosContract)
	if iosContract.Platform != "ios" || iosContract.Services["android_intent"] {
		t.Fatalf("contrato iOS inesperado: %+v", iosContract)
	}
}

func TestFactoriesRejectUnsafePaths(t *testing.T) {
	if _, err := NewAndroidFacade("relative", t.TempDir()); err == nil {
		t.Fatal("NewAndroidFacade() debe rechazar filesDir relativo")
	}
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := NewAndroidFacade(missing, t.TempDir()); err == nil {
		t.Fatal("NewAndroidFacade() debe rechazar directorios inexistentes")
	}
	t.Run("enlaces_simbolicos", func(t *testing.T) {
		realDirectory := t.TempDir()
		symlink := filepath.Join(t.TempDir(), "enlace")
		if err := os.Symlink(realDirectory, symlink); err != nil {
			// ERROR_PRIVILEGE_NOT_HELD: este caso requiere un contexto de
			// laboratorio que pueda crear enlaces. No elevar la aplicación ni
			// omitir los otros contratos de rutas del proceso limitado.
			if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(1314)) {
				t.Skipf("sin privilegio Windows para crear el enlace sintético: %v", err)
			}
			t.Fatalf("Symlink() error = %v", err)
		}
		if _, err := NewAndroidFacade(symlink, t.TempDir()); err == nil {
			t.Fatal("NewAndroidFacade() debe rechazar directorios simbolicos")
		}
	})
	dir := t.TempDir()
	if _, err := NewAndroidFacade(dir, dir); err == nil {
		t.Fatal("NewAndroidFacade() debe exigir directorios distintos")
	}
	if _, err := NewIOSFacade("relative", "", ""); err == nil {
		t.Fatal("NewIOSFacade() debe rechazar applicationSupportDir relativo")
	}
	supportDirectory := filepath.Join(t.TempDir(), "GrxFirma")
	ios, err := NewIOSFacade(supportDirectory, "", "")
	if err != nil {
		t.Fatalf("NewIOSFacade() no creo Application Support: %v", err)
	}
	defer ios.ClearSession()
	info, err := os.Stat(supportDirectory)
	if err != nil {
		t.Fatalf("Stat(Application Support) error = %v", err)
	}
	assertPrivateDirectoryMode(t, info)
}

func TestFacadeRejectsOversizedAndUnknownInput(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	if _, err := facade.SelectCertificateJSON(strings.Repeat("x", maxSelectJSONBytes+1)); err == nil {
		t.Fatal("se esperaba rechazo por JSON sobredimensionado")
	}
	if _, err := facade.SelectCertificateJSON(`{"solo_no_caducados":true,"unexpected":1}`); err == nil {
		t.Fatal("se esperaba rechazo de campos desconocidos")
	}
	tooLongPassword := strings.Repeat("x", maxPasswordBytes+1)
	payload := mustJSON(t, importCertificateRequest{
		DataBase64: base64.StdEncoding.EncodeToString([]byte("not-a-p12")),
		Password:   tooLongPassword,
	})
	if _, err := facade.ImportCertificateJSON(payload); err == nil {
		t.Fatal("se esperaba rechazo de password sobredimensionada")
	}
}

func TestFacadeRejectsUndeclaredSigningModes(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	certificateID := importEphemeralIdentity(t, facade, "modos")
	baseRequest := signRequest{
		Name:          "documento.txt",
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("contenido")),
		MIMEType:      "text/plain",
		Format:        "cades",
		Action:        "sign",
		CertificateID: certificateID,
	}

	undeclaredAction := baseRequest
	undeclaredAction.Action = "unsupported"
	if _, err := facade.SignJSON(mustJSON(t, undeclaredAction)); err == nil {
		t.Fatal("SignJSON() debe rechazar acciones no declaradas en el contrato")
	}

	undeclaredFormat := baseRequest
	undeclaredFormat.Format = "pkcs1"
	if _, err := facade.SignJSON(mustJSON(t, undeclaredFormat)); err == nil {
		t.Fatal("SignJSON() debe rechazar formatos no declarados en el contrato")
	}
}

func TestImportErrorDoesNotExposePassword(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	p12, _ := ephemeralPKCS12(t, "correcta")
	defer zeroBytes(p12)
	secret := "no-filtrar-esta-contrasena"
	payload := mustJSON(t, importCertificateRequest{
		DataBase64: base64.StdEncoding.EncodeToString(p12),
		Password:   secret,
	})
	_, err := facade.ImportCertificateJSON(payload)
	if err == nil {
		t.Fatal("se esperaba error con contrasena incorrecta")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("el error filtro la contrasena: %v", err)
	}
	if err.Error() != mobilePKCS12DecodeMessage {
		t.Fatalf("mensaje de decodificación inesperado: %q", err)
	}
}

func TestImportRejectsPEMPrivateKeyBundle(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	payload := mustJSON(t, importCertificateRequest{
		DataBase64: base64.StdEncoding.EncodeToString([]byte("-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----")),
		Password:   "no-aplicable",
	})
	if _, err := facade.ImportCertificateJSON(payload); err == nil {
		t.Fatal("la API mobile debe aceptar exclusivamente PKCS#12")
	}
}

func TestImportRejectsWeakRSAIdentity(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	p12 := ephemeralRSAPKCS12WithBits(t, "rsa-debil", 1024)
	defer zeroBytes(p12)
	payload := mustJSON(t, importCertificateRequest{
		DataBase64: base64.StdEncoding.EncodeToString(p12),
		Password:   "rsa-debil",
	})
	if _, err := facade.ImportCertificateJSON(payload); err == nil {
		t.Fatal("la API mobile debe rechazar claves RSA inferiores a 2048 bits")
	} else if err.Error() != mobileSigningIdentityUnsupportedMessage {
		t.Fatalf("mensaje de clave débil inesperado: %q", err)
	}
}

func TestImportRejectsCertificateOutsideValidity(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	now := time.Now().UTC()
	p12 := ephemeralRSAPKCS12WithProperties(
		t,
		"caducado",
		2048,
		now.Add(-48*time.Hour),
		now.Add(-24*time.Hour),
		x509.KeyUsageDigitalSignature,
	)
	defer zeroBytes(p12)

	_, err := facade.ImportCertificateJSON(mustJSON(t, importCertificateRequest{
		DataBase64: base64.StdEncoding.EncodeToString(p12),
		Password:   "caducado",
	}))

	if err == nil {
		t.Fatal("la API mobile debe rechazar certificados fuera de vigencia")
	}
	if err.Error() != mobileCertificateNotCurrentMessage {
		t.Fatalf("mensaje de vigencia inesperado: %q", err)
	}
}

func TestImportRejectsCertificateWithoutDigitalSignatureUsage(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	now := time.Now().UTC()
	p12 := ephemeralRSAPKCS12WithProperties(
		t,
		"sin-firma",
		2048,
		now.Add(-time.Hour),
		now.Add(time.Hour),
		x509.KeyUsageKeyEncipherment,
	)
	defer zeroBytes(p12)

	_, err := facade.ImportCertificateJSON(mustJSON(t, importCertificateRequest{
		DataBase64: base64.StdEncoding.EncodeToString(p12),
		Password:   "sin-firma",
	}))

	if err == nil {
		t.Fatal("la API mobile debe rechazar certificados sin uso de firma digital")
	}
	if err.Error() != mobileSigningIdentityUnsupportedMessage {
		t.Fatalf("mensaje de uso de clave inesperado: %q", err)
	}
}

// El certificado de FIRMA del DNIe solo lleva no repudio (contentCommitment);
// escritorio lo admite y el móvil debe hacer lo mismo.
func TestImportAcceptsNonRepudiationOnlyCertificateLikeDNIe(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	now := time.Now().UTC()
	p12 := ephemeralRSAPKCS12WithProperties(
		t,
		"no-repudio",
		2048,
		now.Add(-time.Hour),
		now.Add(time.Hour),
		x509.KeyUsageContentCommitment,
	)
	defer zeroBytes(p12)

	if _, err := facade.ImportCertificateJSON(mustJSON(t, importCertificateRequest{
		DataBase64: base64.StdEncoding.EncodeToString(p12),
		Password:   "no-repudio",
	})); err != nil {
		t.Fatalf("un certificado solo con no repudio debe admitirse para firmar: %v", err)
	}
}

func TestFacadeConcurrentSignAndVerify(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	certificateID := importEphemeralIdentity(t, facade, "concurrente")
	original := []byte("contenido concurrente")

	const workers = 8
	var wg sync.WaitGroup
	errorsChannel := make(chan error, workers)
	for index := 0; index < workers; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			name := fmt.Sprintf("documento-%d.txt", index)
			signed, err := signForConcurrentTest(facade, certificateID, name, original)
			if err != nil {
				errorsChannel <- err
				return
			}
			defer zeroBytes(signed)
			payload := mustJSONNoTest(verifyRequest{
				Name:           "documento.p7s",
				ContentBase64:  base64.StdEncoding.EncodeToString(signed),
				MIMEType:       "application/pkcs7-signature",
				OriginalBase64: base64.StdEncoding.EncodeToString(original),
			})
			out, err := facade.VerifyJSON(payload)
			if err != nil {
				errorsChannel <- err
				return
			}
			var verification verifyResponse
			if err := json.Unmarshal([]byte(out), &verification); err != nil {
				errorsChannel <- err
				return
			}
			if !verification.Valid {
				errorsChannel <- fmt.Errorf("verificacion concurrente invalida: %+v", verification)
			}
		}(index)
	}
	wg.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		t.Error(err)
	}
}

func newAndroidFacadeForTest(t *testing.T) *Facade {
	t.Helper()
	filesDir := t.TempDir()
	noBackupDir := t.TempDir()
	facade, err := NewAndroidFacade(filesDir, noBackupDir)
	if err != nil {
		t.Fatalf("NewAndroidFacade() error = %v", err)
	}
	return facade
}

func importEphemeralIdentity(t *testing.T, facade *Facade, password string) string {
	t.Helper()
	p12, _ := ephemeralPKCS12(t, password)
	defer zeroBytes(p12)
	return importPKCS12(t, facade, p12, password)
}

func importPKCS12(t *testing.T, facade *Facade, p12 []byte, password string) string {
	t.Helper()
	payload := mustJSON(t, importCertificateRequest{
		DataBase64: base64.StdEncoding.EncodeToString(p12),
		Password:   password,
	})
	out, err := facade.ImportCertificateJSON(payload)
	if err != nil {
		t.Fatalf("ImportCertificateJSON() error = %v", err)
	}
	var response importCertificateResponse
	decodeResponse(t, out, &response)
	if response.CertificateID == "" || response.Fingerprint == "" {
		t.Fatalf("identidad importada incompleta: %+v", response)
	}
	return response.CertificateID
}

func ephemeralRSAPKCS12(t *testing.T, password string) []byte {
	return ephemeralRSAPKCS12WithBits(t, password, 2048)
}

func ephemeralRSAPKCS12WithBits(t *testing.T, password string, bits int) []byte {
	now := time.Now().UTC()
	return ephemeralRSAPKCS12WithProperties(
		t,
		password,
		bits,
		now.Add(-time.Hour),
		now.Add(time.Hour),
		x509.KeyUsageDigitalSignature,
	)
}

func ephemeralRSAPKCS12WithProperties(
	t *testing.T,
	password string,
	bits int,
	notBefore, notAfter time.Time,
	keyUsage x509.KeyUsage,
) []byte {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(20260723),
		Subject: pkix.Name{
			CommonName:   "Mobile Test RSA",
			Organization: []string{"Diputacion de Granada"},
		},
		Issuer:                pkix.Name{CommonName: "Mobile Test RSA"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              keyUsage,
		BasicConstraintsValid: true,
		IsCA:                  false,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("CreateCertificate(RSA) error = %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate(RSA) error = %v", err)
	}
	p12, err := pkcs12.Modern.Encode(privateKey, certificate, nil, password)
	if err != nil {
		t.Fatalf("PKCS12 Encode(RSA) error = %v", err)
	}
	return p12
}

func signWithFacade(
	t *testing.T,
	facade *Facade,
	certificateID, name, mimeType, format string,
	content []byte,
) []byte {
	t.Helper()
	payload := mustJSON(t, signRequest{
		Name:          name,
		ContentBase64: base64.StdEncoding.EncodeToString(content),
		MIMEType:      mimeType,
		Format:        format,
		Action:        "sign",
		CertificateID: certificateID,
		Options:       map[string]string{},
	})
	out, err := facade.SignJSON(payload)
	if err != nil {
		t.Fatalf("SignJSON() error = %v", err)
	}
	var response signResponse
	decodeResponse(t, out, &response)
	signed, err := base64.StdEncoding.Strict().DecodeString(response.SignedContentBase64)
	if err != nil {
		t.Fatalf("firma base64 invalida: %v", err)
	}
	if len(signed) == 0 {
		t.Fatal("SignJSON() devolvio firma vacia")
	}
	return signed
}

func verifyWithFacade(
	t *testing.T,
	facade *Facade,
	name, mimeType string,
	signed, original []byte,
) verifyResponse {
	t.Helper()
	request := verifyRequest{
		Name:          name,
		ContentBase64: base64.StdEncoding.EncodeToString(signed),
		MIMEType:      mimeType,
	}
	if len(original) > 0 {
		request.OriginalBase64 = base64.StdEncoding.EncodeToString(original)
	}
	out, err := facade.VerifyJSON(mustJSON(t, request))
	if err != nil {
		t.Fatalf("VerifyJSON() error = %v", err)
	}
	var response verifyResponse
	decodeResponse(t, out, &response)
	return response
}

func assertCryptographicVerification(t *testing.T, response verifyResponse, format string) {
	t.Helper()
	if !response.Valid || response.Format != format {
		t.Fatalf("verificacion %s inesperada: %+v", format, response)
	}
	if response.IntegrityStatus != "valid" {
		t.Fatalf("integridad no valida: %+v", response)
	}
	if response.TrustStatus != "unknown" {
		t.Fatalf("la confianza sin anclas debe ser unknown: %+v", response)
	}
	if response.RevocationMode != "embedded_evidence_only" {
		t.Fatalf("modo de revocacion mobile inesperado: %+v", response)
	}
	if len(response.Warnings) == 0 {
		t.Fatalf("falta advertencia sobre confianza no evaluada: %+v", response)
	}
}

func signForConcurrentTest(facade *Facade, certificateID, name string, content []byte) ([]byte, error) {
	payload := mustJSONNoTest(signRequest{
		Name:          name,
		ContentBase64: base64.StdEncoding.EncodeToString(content),
		MIMEType:      "text/plain",
		Format:        "cades",
		Action:        "sign",
		CertificateID: certificateID,
		Options:       map[string]string{},
	})
	out, err := facade.SignJSON(payload)
	if err != nil {
		return nil, err
	}
	var response signResponse
	if err := json.Unmarshal([]byte(out), &response); err != nil {
		return nil, err
	}
	return base64.StdEncoding.Strict().DecodeString(response.SignedContentBase64)
}

func ephemeralPKCS12(t *testing.T, password string) ([]byte, *x509.Certificate) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(20260722),
		Subject: pkix.Name{
			CommonName:   "Mobile Test",
			Organization: []string{"Diputacion de Granada"},
		},
		Issuer:                pkix.Name{CommonName: "Mobile Test"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate() error = %v", err)
	}
	p12, err := pkcs12.Modern.Encode(privateKey, certificate, nil, password)
	if err != nil {
		t.Fatalf("PKCS12 Encode() error = %v", err)
	}
	return p12, certificate
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return string(raw)
}

func mustJSONNoTest(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func decodeResponse(t *testing.T, raw string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
}
