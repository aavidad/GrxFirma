// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"strings"
	"testing"
	"time"

	cryptobinpkcs7 "github.com/deatil/go-cryptobin/pkcs7"
	cryptobinx509 "github.com/deatil/go-cryptobin/x509"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/machinepolicy"
)

const contenidoCMSPrueba = "contenido confidencial de prueba"

func destinatarioCMSPrueba(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(77),
		Subject:      pkix.Name{CommonName: "Destinatario CMS"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDataEncipherment,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	return priv, der
}

func envelopedConCifrado(t *testing.T, cipher cryptobinpkcs7.Cipher, certDER []byte) domain.Document {
	t.Helper()
	cert, err := cryptobinx509.ParseCertificate(certDER)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := cryptobinpkcs7.Encrypt(rand.Reader, []byte(contenidoCMSPrueba), []*cryptobinx509.Certificate{cert}, cryptobinpkcs7.Opts{
		Cipher:     cipher,
		KeyEncrypt: cryptobinpkcs7.KeyEncryptRSA,
		Mode:       cryptobinpkcs7.DefaultMode,
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := domain.NewDocument("recibido.enveloped", blob, domain.MIMETypeProtectedCMS)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func encryptedConCifrado(t *testing.T, cipher cryptobinpkcs7.Cipher, secret []byte) domain.Document {
	t.Helper()
	blob, err := cryptobinpkcs7.EncryptUsingPSK(rand.Reader, []byte(contenidoCMSPrueba), secret, cipher)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := domain.NewDocument("recibido.encrypted.p7m", blob, domain.MIMETypeProtectedCMS)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// F-06 y lote 13b, punto 6: EnvelopedData y EncryptedData se descifran con
// lo que GrxFirma abría antes de F-06. AES-GCM y AES-CBC, sin más; los
// cifrados heredados que sabe descifrar la biblioteca (DES, 3DES, AES-ECB,
// OFB, CFB, RC2, SM4…), con aviso visible y con el OID para la auditoría.
// La política de máquina prohibir_cifrado_cms_heredado los vuelve a cerrar.
func TestCMSUnprotect_ListaDeAlgoritmosPermitidos(t *testing.T) {
	priv, certDER := destinatarioCMSPrueba(t)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	claveRSA := []domain.ProtectionKeyMaterial{{RecipientID: "dest", CertificateDER: certDER, RSAOAEP256PrivateKeyPKCS8: pkcs8}}
	secreto := make([]byte, protectionAES256KeyBytes)
	if _, err := rand.Read(secreto); err != nil {
		t.Fatal(err)
	}
	claveSimetrica := []domain.ProtectionKeyMaterial{{RecipientID: "psk", SymmetricKey: secreto}}
	engine := NuevoCMSProtector()
	abrir := func(doc domain.Document, claves []domain.ProtectionKeyMaterial) (domain.UnprotectedPayload, error) {
		out, err := engine.Unprotect(context.Background(), doc, claves)
		if err == nil && string(out.Document.Content) != contenidoCMSPrueba {
			t.Fatalf("contenido descifrado inesperado: %q", out.Document.Content)
		}
		return out, err
	}

	heredados := map[string]struct {
		doc    domain.Document
		claves []domain.ProtectionKeyMaterial
		oid    string
	}{
		"EnvelopedData DES":     {envelopedConCifrado(t, cryptobinpkcs7.DESCBC, certDER), claveRSA, "1.3.14.3.2.7"},
		"EnvelopedData 3DES":    {envelopedConCifrado(t, cryptobinpkcs7.DESEDE3CBC, certDER), claveRSA, "1.2.840.113549.3.7"},
		"EnvelopedData AES-ECB": {envelopedConCifrado(t, cryptobinpkcs7.AES256ECB, certDER), claveRSA, "2.16.840.1.101.3.4.1.41"},
		"EnvelopedData AES-CFB": {envelopedConCifrado(t, cryptobinpkcs7.AES256CFB, certDER), claveRSA, "2.16.840.1.101.3.4.1.44"},
		"EnvelopedData RC2":     {envelopedConCifrado(t, cryptobinpkcs7.RC2CBC, certDER), claveRSA, "1.2.840.113549.3.2"},
		"EnvelopedData SM4-CBC": {envelopedConCifrado(t, cryptobinpkcs7.SM4CBC, certDER), claveRSA, ""},
		"EncryptedData AES-ECB": {encryptedConCifrado(t, cryptobinpkcs7.AES256ECB, secreto), claveSimetrica, "2.16.840.1.101.3.4.1.41"},
		"EncryptedData AES-OFB": {encryptedConCifrado(t, cryptobinpkcs7.AES256OFB, secreto), claveSimetrica, "2.16.840.1.101.3.4.1.43"},
	}
	machinepolicy.SetForTest(t, machinepolicy.ProhibirCifradoCMSHeredado, false)
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, false)
	for nombre, caso := range heredados {
		out, err := abrir(caso.doc, caso.claves)
		if err != nil {
			t.Errorf("%s: lo que se abría antes debe seguir abriéndose: %v", nombre, err)
			continue
		}
		if len(out.Avisos) != 1 || out.Avisos[0] != avisoCMSCifradoHeredado {
			t.Errorf("%s: falta el aviso visible: %v", nombre, out.Avisos)
		}
		if len(out.Detalles) != 1 || !strings.HasPrefix(out.Detalles[0], detalleCMSCifradoHeredado) ||
			(caso.oid != "" && out.Detalles[0] != detalleCMSCifradoHeredado+caso.oid) {
			t.Errorf("%s: falta el detalle de auditoría: %v", nombre, out.Detalles)
		}
	}

	// Lo que escribe GrxFirma y AES-CBC (lo que emite `openssl cms -encrypt`
	// por defecto) se abre sin aviso, con o sin la política.
	actuales := map[string]struct {
		doc    domain.Document
		claves []domain.ProtectionKeyMaterial
	}{
		"EnvelopedData AES-256-GCM": {envelopedConCifrado(t, cryptobinpkcs7.AES256GCM, certDER), claveRSA},
		"EnvelopedData AES-128-GCM": {envelopedConCifrado(t, cryptobinpkcs7.AES128GCM, certDER), claveRSA},
		"EnvelopedData AES-256-CBC": {envelopedConCifrado(t, cryptobinpkcs7.AES256CBC, certDER), claveRSA},
		"EncryptedData AES-256-GCM": {encryptedConCifrado(t, cryptobinpkcs7.AES256GCM, secreto), claveSimetrica},
		"EncryptedData AES-256-CBC": {encryptedConCifrado(t, cryptobinpkcs7.AES256CBC, secreto), claveSimetrica},
	}
	for _, prohibir := range []bool{false, true} {
		machinepolicy.SetForTest(t, machinepolicy.ProhibirCifradoCMSHeredado, prohibir)
		for nombre, caso := range actuales {
			if out, err := abrir(caso.doc, caso.claves); err != nil || len(out.Avisos) != 0 || len(out.Detalles) != 0 {
				t.Errorf("%s (prohibir=%v): %v %v", nombre, prohibir, err, out.Avisos)
			}
		}
	}

	// Con la política, ningún cifrado heredado se descifra, ni siquiera con
	// permitir_des_legacy, que ya no hace falta para CMS.
	machinepolicy.SetForTest(t, machinepolicy.ProhibirCifradoCMSHeredado, true)
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, true)
	for nombre, caso := range heredados {
		_, err := abrir(caso.doc, caso.claves)
		if err == nil || !strings.Contains(err.Error(), machinepolicy.ProhibirCifradoCMSHeredado) {
			t.Errorf("%s: la política prohibir_cifrado_cms_heredado debe impedirlo: %v", nombre, err)
		}
	}
}

// Lo que F-06 cerró y la biblioteca no sabe descifrar sigue cerrado: un
// cifrado desconocido o un transporte de clave que no es RSA se rechazan
// antes de llegar a las bibliotecas.
func TestCMSUnprotect_AlgoritmoDesconocidoSigueRechazado(t *testing.T) {
	priv, certDER := destinatarioCMSPrueba(t)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	claveRSA := []domain.ProtectionKeyMaterial{{RecipientID: "dest", CertificateDER: certDER, RSAOAEP256PrivateKeyPKCS8: pkcs8}}
	machinepolicy.SetForTest(t, machinepolicy.ProhibirCifradoCMSHeredado, false)
	base := envelopedConCifrado(t, cryptobinpkcs7.AES256GCM, certDER)
	cambiarOID := func(desde, hasta []byte) domain.Document {
		if bytes.Count(base.Content, desde) != 1 {
			t.Fatalf("el OID %x debe aparecer una vez", desde)
		}
		doc := base
		doc.Content = bytes.Replace(base.Content, desde, hasta, 1)
		return doc
	}
	for nombre, doc := range map[string]domain.Document{
		// 2.16.840.1.101.3.4.1.46 (AES-256-GCM) pasa a .127, que no existe.
		"cifrado desconocido": cambiarOID([]byte{0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x01, 0x2e}, []byte{0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x01, 0x7f}),
		// rsaEncryption pasa a sha256WithRSAEncryption.
		"transporte no RSA": cambiarOID([]byte{0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x01, 0x01}, []byte{0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x01, 0x0b}),
	} {
		if _, err := NuevoCMSProtector().Unprotect(context.Background(), doc, claveRSA); err == nil || !strings.Contains(err.Error(), "no permitido") {
			t.Errorf("%s: se esperaba rechazo antes de descifrar: %v", nombre, err)
		}
	}
}
