// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"bytes"
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"slices"
	"testing"
	"time"

	cryptobinpkcs7 "github.com/deatil/go-cryptobin/pkcs7"
	cryptobinx509 "github.com/deatil/go-cryptobin/x509"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

func comprobarBloqueoCMS(t *testing.T, err error, id string) {
	t.Helper()
	var bloqueo *excepciones.Block
	if !errors.As(err, &bloqueo) || bloqueo.ID != id || bloqueo.BlockKey == "" {
		t.Fatalf("bloqueo %s: %v", id, err)
	}
}

func TestCMSCrearPKCS1v15ExigeAjusteYDevuelveAviso(t *testing.T) {
	_, cert := newCMSRSAIdentity(t, x509.KeyUsageKeyEncipherment)
	receptor := newCMSRecipient(t, "dest", cert)
	doc, err := domain.NewDocument("prueba.bin", []byte("contenido"), "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	job := domain.ProtectionJob{Document: doc, Profile: domain.ProtectionProfileCompat, Options: map[string]string{
		"container": "cms", "cms_key_transport": "pkcs1v15",
	}}
	jobSinContenedor := job
	jobSinContenedor.Options = map[string]string{"cms_key_transport": "pkcs1v15"}
	for _, caso := range []struct {
		nombre  string
		usuario bool
		maquina string
	}{
		{"sin ajuste", false, "ausente"},
		{"ajuste de usuario", true, "ausente"},
		{"prohibido por máquina", true, "prohibido"},
		{"directiva ilegible", true, "ilegible"},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			excepciones.SetForTest(t, "cms_rsa_v15_crear", caso.usuario)
			switch caso.maquina {
			case "ausente":
				machinepolicy.UnsetForTest(t, "permitir_cms_rsa_v15_crear")
			case "prohibido":
				machinepolicy.SetForTest(t, "permitir_cms_rsa_v15_crear", false)
			case "ilegible":
				machinepolicy.ErrorForTest(t, "permitir_cms_rsa_v15_crear", errors.New("fixture ilegible"))
			}
			ctx, avisos := domain.ConAvisosOperacion(context.Background())
			out, err := NuevoCMSProtector().Protect(ctx, job, []domain.ProtectionRecipient{receptor})
			if caso.nombre != "ajuste de usuario" {
				comprobarBloqueoCMS(t, err, "cms_rsa_v15_crear")
				_, err = NuevoCMSProtector().Protect(ctx, jobSinContenedor, []domain.ProtectionRecipient{receptor})
				comprobarBloqueoCMS(t, err, "cms_rsa_v15_crear")
				return
			}
			if err != nil || !slices.Contains(avisos.Claves(), excepciones.Warning("cms_rsa_v15_crear")) {
				t.Fatalf("CMS PKCS1v15 autorizado: %v, avisos=%v", err, avisos.Claves())
			}
			if got := inspectCMSAlgorithms(t, out.Document.Content).keyEncryption[0]; !got.Equal(oidCMSRSAPKCS1v15) {
				t.Fatalf("transporte esperado PKCS1v15: %v", got)
			}
		})
	}
}

func TestCMSLeerCBCExigeAjusteYDevuelveAviso(t *testing.T) {
	secret := bytes.Repeat([]byte{0x42}, protectionAES256KeyBytes)
	doc := encryptedConCifrado(t, cryptobinpkcs7.AES256CBC, secret)
	keys := []domain.ProtectionKeyMaterial{{RecipientID: "psk", SymmetricKey: secret}}
	for _, caso := range []struct {
		nombre  string
		usuario bool
		maquina string
	}{
		{"sin ajuste", false, "ausente"},
		{"ajuste de usuario", true, "ausente"},
		{"prohibido por máquina", true, "prohibido"},
		{"directiva ilegible", true, "ilegible"},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			excepciones.SetForTest(t, "cms_cbc", caso.usuario)
			machinepolicy.UnsetForTest(t, machinepolicy.ProhibirCifradoCMSHeredado)
			switch caso.maquina {
			case "ausente":
				machinepolicy.UnsetForTest(t, "permitir_cms_cbc")
			case "prohibido":
				machinepolicy.SetForTest(t, "permitir_cms_cbc", false)
			case "ilegible":
				machinepolicy.ErrorForTest(t, "permitir_cms_cbc", errors.New("fixture ilegible"))
			}
			out, err := NuevoCMSProtector().Unprotect(context.Background(), doc, keys)
			if caso.nombre != "ajuste de usuario" {
				comprobarBloqueoCMS(t, err, "cms_cbc")
				return
			}
			if err != nil || !slices.Contains(out.Avisos, excepciones.Warning("cms_cbc")) {
				t.Fatalf("CMS CBC autorizado: %v, avisos=%v", err, out.Avisos)
			}
		})
	}
}

func TestCMSLeerPKCS1v15ExigeAjusteYDevuelveAviso(t *testing.T) {
	private, cert := destinatarioCMSPrueba(t)
	privateDER, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	doc := envelopedConCifrado(t, cryptobinpkcs7.AES256GCM, cert)
	keys := []domain.ProtectionKeyMaterial{{RecipientID: "dest", CertificateDER: cert, RSAOAEP256PrivateKeyPKCS8: privateDER}}
	for _, caso := range []struct {
		nombre  string
		usuario bool
		maquina string
	}{
		{"sin ajuste", false, "ausente"},
		{"ajuste de usuario", true, "ausente"},
		{"prohibido por máquina", true, "prohibido"},
		{"directiva ilegible", true, "ilegible"},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			excepciones.SetForTest(t, "cms_rsa_v15_leer", caso.usuario)
			switch caso.maquina {
			case "ausente":
				machinepolicy.UnsetForTest(t, "permitir_cms_rsa_v15_leer")
			case "prohibido":
				machinepolicy.SetForTest(t, "permitir_cms_rsa_v15_leer", false)
			case "ilegible":
				machinepolicy.ErrorForTest(t, "permitir_cms_rsa_v15_leer", errors.New("fixture ilegible"))
			}
			out, err := NuevoCMSProtector().Unprotect(context.Background(), doc, keys)
			if caso.nombre != "ajuste de usuario" {
				comprobarBloqueoCMS(t, err, "cms_rsa_v15_leer")
				return
			}
			if err != nil || !slices.Contains(out.Avisos, excepciones.Warning("cms_rsa_v15_leer")) {
				t.Fatalf("lectura PKCS1v15 autorizada: %v, avisos=%v", err, out.Avisos)
			}
		})
	}
}

func TestCMSOAEP256ConDestinatarioOAEPAntiguoConservaAmbos(t *testing.T) {
	_, certA := newCMSRSAIdentity(t, x509.KeyUsageKeyEncipherment)
	_, certB := newCMSRSAIdentity(t, x509.KeyUsageKeyEncipherment)
	doc, err := domain.NewDocument("mixto.bin", []byte("contenido"), "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	out, err := NuevoCMSProtector().Protect(context.Background(), domain.ProtectionJob{
		Document: doc, Profile: domain.ProtectionProfileCompat, Options: map[string]string{"container": "cms"},
	}, []domain.ProtectionRecipient{newCMSRecipient(t, "a", certA), newCMSRecipient(t, "b", certB)})
	if err != nil {
		t.Fatal(err)
	}
	var wrapper cmsContentInfo
	if _, err := asn1.Unmarshal(out.Document.Content, &wrapper); err != nil {
		t.Fatal(err)
	}
	var sobre struct {
		Version          int
		Recipients       []cmsKeyTransRecipientInfo `asn1:"set"`
		EncryptedContent cmsAuthEncryptedContentInfo
	}
	if _, err := asn1.Unmarshal(wrapper.Content.Bytes, &sobre); err != nil {
		t.Fatal(err)
	}
	sobre.Recipients[0].KeyEncryptionAlgorithm.Parameters = asn1.RawValue{}
	inner, err := asn1.Marshal(sobre)
	if err != nil {
		t.Fatal(err)
	}
	wrapper.Content.Bytes, wrapper.Content.FullBytes = inner, nil
	mixed, err := asn1.Marshal(wrapper)
	if err != nil {
		t.Fatal(err)
	}
	adapted, changed, err := transformarOAEP256(mixed, false)
	if err != nil || !changed {
		t.Fatalf("adaptación OAEP mixta: %v, changed=%v", err, changed)
	}
	if _, err := asn1.Unmarshal(adapted, &wrapper); err != nil {
		t.Fatal(err)
	}
	if _, err := asn1.Unmarshal(wrapper.Content.Bytes, &sobre); err != nil {
		t.Fatal(err)
	}
	if len(sobre.Recipients) != 2 {
		t.Fatalf("se perdió un destinatario: %d", len(sobre.Recipients))
	}
	var antiguos, nuevos int
	for _, r := range sobre.Recipients {
		if r.KeyEncryptionAlgorithm.Algorithm.Equal(oidCMSRSAESOAEP) {
			antiguos++
		}
		if r.KeyEncryptionAlgorithm.Algorithm.Equal(oidCMSSHA256WithRSA) {
			nuevos++
		}
	}
	if antiguos != 1 || nuevos != 1 {
		t.Fatalf("algoritmos tras adaptación: antiguos=%d nuevos=%d", antiguos, nuevos)
	}
}

func TestCMSDestinatarioOAEPPropioNoExigeAjustePorOtroV15(t *testing.T) {
	private, cert := newCMSRSAIdentity(t, x509.KeyUsageKeyEncipherment)
	otherPrivate, otherCert := newCMSRSAIdentity(t, x509.KeyUsageKeyEncipherment)
	doc, err := domain.NewDocument("mixto.bin", []byte("solo para OAEP"), "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	protected, err := NuevoCMSProtector().Protect(context.Background(), domain.ProtectionJob{
		Document: doc, Profile: domain.ProtectionProfileCompat, Options: map[string]string{"container": "cms"},
	}, []domain.ProtectionRecipient{newCMSRecipient(t, "oaep", cert)})
	if err != nil {
		t.Fatal(err)
	}
	var wrapper cmsContentInfo
	if _, err := asn1.Unmarshal(protected.Document.Content, &wrapper); err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Version    int
		Recipients []cmsKeyTransRecipientInfo `asn1:"set"`
		Content    cmsAuthEncryptedContentInfo
	}
	if _, err := asn1.Unmarshal(wrapper.Content.Bytes, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Recipients = append(envelope.Recipients, cmsKeyTransRecipientInfo{
		Version: 0,
		IssuerAndSerialNumber: cmsIssuerAndSerialNumber{
			IssuerName: asn1.RawValue{FullBytes: otherCert.RawIssuer}, SerialNumber: otherCert.SerialNumber,
		},
		KeyEncryptionAlgorithm: pkix.AlgorithmIdentifier{Algorithm: oidCMSRSAPKCS1v15},
		EncryptedKey:           bytes.Repeat([]byte{0x42}, otherPrivate.Size()),
	})
	inner, err := asn1.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	wrapper.Content.Bytes, wrapper.Content.FullBytes = inner, nil
	protected.Document.Content, err = asn1.Marshal(wrapper)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	machinepolicy.SetForTest(t, "permitir_cms_rsa_v15_leer", false)
	out, err := NuevoCMSProtector().Unprotect(context.Background(), protected.Document, []domain.ProtectionKeyMaterial{{
		RecipientID: "oaep", CertificateDER: cert.Raw, RSAOAEP256PrivateKeyPKCS8: privateDER,
	}})
	if err != nil || !bytes.Equal(out.Document.Content, doc.Content) || slices.Contains(out.Avisos, excepciones.Warning("cms_rsa_v15_leer")) {
		t.Fatalf("destinatario OAEP propio quedó bloqueado o avisó v15 ajeno: %v, avisos=%v", err, out.Avisos)
	}
}

func TestCMSSignedEnvelopedLeerPKCS1v15ExigeAjuste(t *testing.T) {
	protected, key, root, _ := legacySAEDFixture(t, cryptobinpkcs7.AES256GCM)
	for _, caso := range []struct {
		nombre  string
		usuario bool
		maquina string
	}{
		{"sin ajuste", false, "ausente"},
		{"ajuste de usuario", true, "ausente"},
		{"prohibido por máquina", true, "prohibido"},
		{"directiva ilegible", true, "ilegible"},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			excepciones.SetForTest(t, "cms_rsa_v15_leer", caso.usuario)
			switch caso.maquina {
			case "ausente":
				machinepolicy.UnsetForTest(t, "permitir_cms_rsa_v15_leer")
			case "prohibido":
				machinepolicy.SetForTest(t, "permitir_cms_rsa_v15_leer", false)
			case "ilegible":
				machinepolicy.ErrorForTest(t, "permitir_cms_rsa_v15_leer", errors.New("fixture ilegible"))
			}
			out, err := newTrustedCMSTestProtector(root).Unprotect(context.Background(), protected, []domain.ProtectionKeyMaterial{key})
			if caso.nombre != "ajuste de usuario" {
				comprobarBloqueoCMS(t, err, "cms_rsa_v15_leer")
				return
			}
			if err != nil || !slices.Contains(out.Avisos, excepciones.Warning("cms_rsa_v15_leer")) {
				t.Fatalf("SignedAndEnveloped autorizado: %v, avisos=%v", err, out.Avisos)
			}
		})
	}
}

func TestCMSSignedEnvelopedCrearPKCS1v15ExigeAjuste(t *testing.T) {
	private, cert := newCMSRSAIdentity(t, x509.KeyUsageKeyEncipherment|x509.KeyUsageDigitalSignature)
	doc, err := domain.NewDocument("signed.bin", []byte("contenido firmado"), "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	recipient := newCMSRecipient(t, "dest", cert)
	signer := &commonsigner.LocalSigningKey{ID: "firmante", Signer: private, Certificate: cert}
	job := domain.ProtectionJob{Document: doc, Profile: domain.ProtectionProfileCompat, Options: map[string]string{
		"container": "signedandenvelopeddata", "cms_key_transport": "pkcs1v15",
	}}
	for _, caso := range []struct {
		nombre  string
		usuario bool
		maquina string
	}{
		{"sin ajuste", false, "ausente"},
		{"ajuste de usuario", true, "ausente"},
		{"prohibido por máquina", true, "prohibido"},
		{"directiva ilegible", true, "ilegible"},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			excepciones.SetForTest(t, "cms_rsa_v15_crear", caso.usuario)
			switch caso.maquina {
			case "ausente":
				machinepolicy.UnsetForTest(t, "permitir_cms_rsa_v15_crear")
			case "prohibido":
				machinepolicy.SetForTest(t, "permitir_cms_rsa_v15_crear", false)
			case "ilegible":
				machinepolicy.ErrorForTest(t, "permitir_cms_rsa_v15_crear", errors.New("fixture ilegible"))
			}
			ctx, warnings := domain.ConAvisosOperacion(context.Background())
			out, err := NuevoCMSSignedEnvelopedProtector().ProtectAndSign(ctx, job, []domain.ProtectionRecipient{recipient}, signer)
			if caso.nombre != "ajuste de usuario" {
				comprobarBloqueoCMS(t, err, "cms_rsa_v15_crear")
				return
			}
			if err != nil || !slices.Contains(warnings.Claves(), excepciones.Warning("cms_rsa_v15_crear")) {
				t.Fatalf("SAED PKCS1v15 autorizado: %v, warnings=%v", err, warnings.Claves())
			}
			if got := inspectCMSAlgorithms(t, out.Document.Content).keyEncryption[0]; !got.Equal(oidCMSRSAPKCS1v15) {
				t.Fatalf("transporte esperado PKCS1v15: %v", got)
			}
		})
	}
}

func TestCMSSignedEnvelopedCBCExigeAjusteYRespetaProhibicion(t *testing.T) {
	protected, key, root, content := legacySAEDFixture(t, cryptobinpkcs7.AES256CBC)
	keys := []domain.ProtectionKeyMaterial{key}
	for _, caso := range []struct {
		nombre  string
		usuario bool
		maquina string
	}{
		{"sin ajuste", false, "ausente"},
		{"ajuste de usuario", true, "ausente"},
		{"prohibido por máquina", true, "prohibido"},
		{"directiva ilegible", true, "ilegible"},
		{"prohibición de cifrados heredados", true, "prohibicion_heredados"},
		{"prohibición ilegible", true, "prohibicion_ilegible"},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			excepciones.SetForTest(t, "cms_cbc", caso.usuario)
			machinepolicy.SetForTest(t, "permitir_cms_rsa_v15_leer", true)
			machinepolicy.UnsetForTest(t, machinepolicy.ProhibirCifradoCMSHeredado)
			switch caso.maquina {
			case "ausente":
				machinepolicy.UnsetForTest(t, "permitir_cms_cbc")
			case "prohibido":
				machinepolicy.SetForTest(t, "permitir_cms_cbc", false)
			case "ilegible":
				machinepolicy.ErrorForTest(t, "permitir_cms_cbc", errors.New("fixture ilegible"))
			case "prohibicion_heredados":
				machinepolicy.UnsetForTest(t, "permitir_cms_cbc")
				machinepolicy.SetForTest(t, machinepolicy.ProhibirCifradoCMSHeredado, true)
			case "prohibicion_ilegible":
				machinepolicy.UnsetForTest(t, "permitir_cms_cbc")
				machinepolicy.ErrorForTest(t, machinepolicy.ProhibirCifradoCMSHeredado, errors.New("fixture ilegible"))
			}
			out, err := newTrustedCMSTestProtector(root).Unprotect(context.Background(), protected, keys)
			if caso.nombre != "ajuste de usuario" {
				comprobarBloqueoCMS(t, err, "cms_cbc")
				return
			}
			if err != nil || !bytes.Equal(out.Document.Content, content) || !slices.Contains(out.Avisos, excepciones.Warning("cms_cbc")) {
				t.Fatalf("SignedAndEnveloped CBC: %v, avisos=%v", err, out.Avisos)
			}
		})
	}
}

func legacySAEDFixture(t *testing.T, cipher cryptobinpkcs7.Cipher) (domain.Document, domain.ProtectionKeyMaterial, *x509.Certificate, []byte) {
	t.Helper()
	rootKey, root := newCMSRootCA(t, "CA CBC de prueba")
	signerKey, signer := newCMSSignedLeaf(t, rootKey, root, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	compatSigner, err := cryptobinx509.ParseCertificate(signer.Raw)
	if err != nil {
		t.Fatal(err)
	}
	compatRoot, err := cryptobinx509.ParseCertificate(root.Raw)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("contenido firmado y cifrado")
	saed, err := cryptobinpkcs7.NewSignedAndEnvelopedData(content, cipher)
	if err != nil {
		t.Fatal(err)
	}
	saed.SetDigestAlgorithm(cryptobinpkcs7.OidDigestAlgorithmSHA256)
	if err := saed.AddSigner(compatSigner, signerKey); err != nil {
		t.Fatal(err)
	}
	saed.AddCertificate(compatRoot)
	if err := saed.AddRecipient(compatSigner); err != nil {
		t.Fatal(err)
	}
	blob, err := saed.Finish()
	if err != nil {
		t.Fatal(err)
	}
	protected, err := domain.NewDocument("signed-cbc.p7m", blob, domain.MIMETypeProtectedCMS)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(signerKey)
	if err != nil {
		t.Fatal(err)
	}
	return protected, domain.ProtectionKeyMaterial{RecipientID: "cbc", RSAOAEP256PrivateKeyPKCS8: privateDER, CertificateDER: signer.Raw}, root, content
}

func TestCMSReaderRespetaDirectorioDeAjustesDeLaOperacion(t *testing.T) {
	dir := t.TempDir()
	machinepolicy.UnsetForTest(t, "permitir_cms_cbc")
	machinepolicy.UnsetForTest(t, machinepolicy.ProhibirCifradoCMSHeredado)
	if _, err := excepciones.ActivateLocal(excepciones.ForLocalCLI(context.Background()), dir, excepciones.ActivationRequest{
		ID: "cms_cbc", Accepted: true, AcceptanceVersion: excepciones.AcceptanceVersion,
	}); err != nil {
		t.Fatal(err)
	}
	secret := bytes.Repeat([]byte{0x72}, protectionAES256KeyBytes)
	doc := encryptedConCifrado(t, cryptobinpkcs7.AES256CBC, secret)
	keys := []domain.ProtectionKeyMaterial{{RecipientID: "psk", SymmetricKey: secret}}
	ctx := excepciones.WithConfigDir(context.Background(), dir)
	out, err := NuevoCMSProtector().Unprotect(ctx, doc, keys)
	if err != nil || !slices.Contains(out.Avisos, excepciones.Warning("cms_cbc")) {
		t.Fatalf("ajuste de la operación: %v, avisos=%v", err, out.Avisos)
	}
}
