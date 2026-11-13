// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
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

// F-06: EnvelopedData y EncryptedData solo se descifran con algoritmos de
// contenido permitidos. 3DES y DES exigen la política de máquina
// permitir_des_legacy; ECB, OFB, CFB, RC2, RC5 o SM4 no se aceptan nunca.
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
	abrir := func(doc domain.Document, claves []domain.ProtectionKeyMaterial) error {
		out, err := engine.Unprotect(context.Background(), doc, claves)
		if err == nil && string(out.Document.Content) != contenidoCMSPrueba {
			t.Fatalf("contenido descifrado inesperado: %q", out.Document.Content)
		}
		return err
	}

	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, false)
	for nombre, doc := range map[string]domain.Document{
		"EnvelopedData 3DES":    envelopedConCifrado(t, cryptobinpkcs7.DESEDE3CBC, certDER),
		"EnvelopedData AES-ECB": envelopedConCifrado(t, cryptobinpkcs7.AES256ECB, certDER),
		"EnvelopedData AES-CFB": envelopedConCifrado(t, cryptobinpkcs7.AES256CFB, certDER),
		"EnvelopedData RC2":     envelopedConCifrado(t, cryptobinpkcs7.RC2CBC, certDER),
		"EnvelopedData SM4-CBC": envelopedConCifrado(t, cryptobinpkcs7.SM4CBC, certDER),
		"EncryptedData AES-ECB": encryptedConCifrado(t, cryptobinpkcs7.AES256ECB, secreto),
		"EncryptedData AES-OFB": encryptedConCifrado(t, cryptobinpkcs7.AES256OFB, secreto),
		"EncryptedData DES-CBC": encryptedConCifrado(t, cryptobinpkcs7.DESCBC, secreto[:8]),
	} {
		claves := claveRSA
		if strings.HasPrefix(nombre, "EncryptedData") {
			claves = claveSimetrica
		}
		if err := abrir(doc, claves); err == nil {
			t.Errorf("%s: se descifró sin política", nombre)
		}
	}

	// Formatos habituales que siguen abriéndose: los que escribe GrxFirma y
	// AES-CBC, que es lo que emite `openssl cms -encrypt` por defecto.
	for nombre, caso := range map[string]struct {
		doc    domain.Document
		claves []domain.ProtectionKeyMaterial
	}{
		"EnvelopedData AES-256-GCM": {envelopedConCifrado(t, cryptobinpkcs7.AES256GCM, certDER), claveRSA},
		"EnvelopedData AES-128-GCM": {envelopedConCifrado(t, cryptobinpkcs7.AES128GCM, certDER), claveRSA},
		"EnvelopedData AES-256-CBC": {envelopedConCifrado(t, cryptobinpkcs7.AES256CBC, certDER), claveRSA},
		"EncryptedData AES-256-GCM": {encryptedConCifrado(t, cryptobinpkcs7.AES256GCM, secreto), claveSimetrica},
		"EncryptedData AES-256-CBC": {encryptedConCifrado(t, cryptobinpkcs7.AES256CBC, secreto), claveSimetrica},
	} {
		if err := abrir(caso.doc, caso.claves); err != nil {
			t.Errorf("%s: %v", nombre, err)
		}
	}

	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, true)
	if err := abrir(envelopedConCifrado(t, cryptobinpkcs7.DESEDE3CBC, certDER), claveRSA); err != nil {
		t.Fatalf("3DES con la política permitir_des_legacy: %v", err)
	}
	if err := abrir(envelopedConCifrado(t, cryptobinpkcs7.AES256ECB, certDER), claveRSA); err == nil {
		t.Fatal("AES-ECB se descifró aunque la política solo permite DES heredado")
	}
}
