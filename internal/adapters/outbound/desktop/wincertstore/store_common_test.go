// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package wincertstore

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

func TestNormalizarHuella(t *testing.T) {
	t.Parallel()

	esperada := strings.Repeat("ab", sha256.Size)
	conSeparadores := strings.ToUpper(strings.Join(partirPares(esperada), ":"))
	obtenida, err := normalizarHuella("  " + conSeparadores + "\n")
	if err != nil {
		t.Fatalf("normalizarHuella() error = %v", err)
	}
	if obtenida != esperada {
		t.Fatalf("normalizarHuella() = %q, want %q", obtenida, esperada)
	}

	for _, invalida := range []string{"", "abcd", strings.Repeat("z", sha256.Size*2)} {
		if _, err := normalizarHuella(invalida); err == nil {
			t.Errorf("normalizarHuella(%q) debía fallar", invalida)
		}
	}
}

func TestHuellaBuscadaPrefiereFingerprint(t *testing.T) {
	t.Parallel()

	fingerprint := strings.Repeat("01", sha256.Size)
	id := strings.Repeat("02", sha256.Size)
	obtenida, err := huellaBuscada(domain.CertificateRef{ID: id, Fingerprint: fingerprint})
	if err != nil {
		t.Fatalf("huellaBuscada() error = %v", err)
	}
	if obtenida != fingerprint {
		t.Fatalf("huellaBuscada() = %q, want %q", obtenida, fingerprint)
	}
}

func TestConstruirRefDesdeDER(t *testing.T) {
	t.Parallel()

	der := crearCertificadoAutofirmado(t, "Empleado público", true)
	ref, cert, err := construirRefDesdeDER(der)
	if err != nil {
		t.Fatalf("construirRefDesdeDER() error = %v", err)
	}
	suma := sha256.Sum256(der)
	esperada := hex.EncodeToString(suma[:])
	if ref.ID != esperada || ref.Fingerprint != esperada {
		t.Fatalf("huella = (%q, %q), want %q", ref.ID, ref.Fingerprint, esperada)
	}
	if ref.Subject != "Empleado público" {
		t.Fatalf("Subject = %q", ref.Subject)
	}
	if cert == nil || len(cert.Raw) == 0 {
		t.Fatal("certificado parseado vacío")
	}

	primerByte := cert.Raw[0]
	der[0] ^= 0xff
	if cert.Raw[0] != primerByte {
		t.Fatal("el certificado conserva una referencia al buffer DER del llamador")
	}
}

func TestResolverParametrosHash(t *testing.T) {
	t.Parallel()

	casos := []struct {
		hash   crypto.Hash
		nombre string
		algID  uint32
	}{
		{crypto.SHA1, "SHA1", calgSHA1},
		{crypto.SHA256, "SHA256", calgSHA256},
		{crypto.SHA384, "SHA384", calgSHA384},
		{crypto.SHA512, "SHA512", calgSHA512},
	}
	for _, caso := range casos {
		caso := caso
		t.Run(caso.nombre, func(t *testing.T) {
			t.Parallel()
			obtenido, err := resolverParametrosHash(caso.hash)
			if err != nil {
				t.Fatalf("resolverParametrosHash() error = %v", err)
			}
			if obtenido.nombreCNG != caso.nombre || obtenido.algIDCAPI != caso.algID {
				t.Fatalf("resolverParametrosHash() = %+v", obtenido)
			}
		})
	}
	if _, err := resolverParametrosHash(crypto.MD5); err == nil {
		t.Fatal("MD5 no debe aceptarse para firma")
	}
}

func TestValidarClavePublica(t *testing.T) {
	t.Parallel()

	if err := validarClavePublica(&rsa.PublicKey{}); err != nil {
		t.Fatalf("RSA debía estar soportada: %v", err)
	}
	if err := validarClavePublica(&ecdsa.PublicKey{}); err != nil {
		t.Fatalf("ECDSA debía estar soportada: %v", err)
	}
	if err := validarClavePublica(ed25519.PublicKey(make([]byte, ed25519.PublicKeySize))); err == nil {
		t.Fatal("Ed25519 no debe anunciarse como compatible con este proveedor")
	}
}

func TestCertificadoAptoParaCatalogo(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre string
		cert   *x509.Certificate
		want   bool
	}{
		{
			nombre: "firma_digital",
			cert:   &x509.Certificate{KeyUsage: x509.KeyUsageDigitalSignature},
			want:   true,
		},
		{
			nombre: "compromiso_de_contenido",
			cert:   &x509.Certificate{KeyUsage: x509.KeyUsageContentCommitment},
			want:   true,
		},
		{
			nombre: "ambos_usos_de_firma",
			cert: &x509.Certificate{
				KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
			},
			want: true,
		},
		{
			nombre: "sin_extension_key_usage",
			cert:   &x509.Certificate{},
			want:   true,
		},
		{
			nombre: "ca_aunque_permite_firma",
			cert: &x509.Certificate{
				IsCA:     true,
				KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
			},
			want: false,
		},
		{
			nombre: "cert_sign_sin_basic_constraints",
			cert:   &x509.Certificate{KeyUsage: x509.KeyUsageCertSign},
			want:   false,
		},
		{
			nombre: "cifrado_de_clave",
			cert:   &x509.Certificate{KeyUsage: x509.KeyUsageKeyEncipherment},
			want:   false,
		},
		{
			nombre: "acuerdo_de_clave",
			cert:   &x509.Certificate{KeyUsage: x509.KeyUsageKeyAgreement},
			want:   false,
		},
		{
			nombre: "nulo",
			cert:   nil,
			want:   false,
		},
	}

	for _, caso := range casos {
		caso := caso
		t.Run(caso.nombre, func(t *testing.T) {
			t.Parallel()
			if obtenido := certificadoAptoParaCatalogo(caso.cert); obtenido != caso.want {
				t.Fatalf("certificadoAptoParaCatalogo() = %t, want %t", obtenido, caso.want)
			}
		})
	}
}

func TestLongitudSalPSS(t *testing.T) {
	t.Parallel()

	publica := &rsa.PublicKey{N: new(big.Int).Lsh(big.NewInt(1), 2047), E: 65537}
	casos := []struct {
		nombre   string
		longitud int
		want     uint32
	}{
		{"auto", rsa.PSSSaltLengthAuto, 222},
		{"igual_hash", rsa.PSSSaltLengthEqualsHash, sha256.Size},
		{"explicita", 20, 20},
	}
	for _, caso := range casos {
		caso := caso
		t.Run(caso.nombre, func(t *testing.T) {
			t.Parallel()
			obtenida, err := longitudSalPSS(publica, crypto.SHA256, &rsa.PSSOptions{SaltLength: caso.longitud, Hash: crypto.SHA256})
			if err != nil {
				t.Fatalf("longitudSalPSS() error = %v", err)
			}
			if obtenida != caso.want {
				t.Fatalf("longitudSalPSS() = %d, want %d", obtenida, caso.want)
			}
		})
	}
	if _, err := longitudSalPSS(publica, crypto.SHA256, &rsa.PSSOptions{SaltLength: 223}); err == nil {
		t.Fatal("una sal superior al máximo debe rechazarse")
	}
}

func TestFirmaECDSADesdeCNG(t *testing.T) {
	t.Parallel()

	publica := &ecdsa.PublicKey{Curve: elliptic.P256()}
	raw := make([]byte, 64)
	raw[31] = 1
	raw[63] = 2
	firma, err := firmaECDSADesdeCNG(raw, publica)
	if err != nil {
		t.Fatalf("firmaECDSADesdeCNG() error = %v", err)
	}
	var valores struct {
		R *big.Int
		S *big.Int
	}
	resto, err := asn1.Unmarshal(firma, &valores)
	if err != nil || len(resto) != 0 {
		t.Fatalf("firma ASN.1 inválida: err=%v resto=%x", err, resto)
	}
	if valores.R.Cmp(big.NewInt(1)) != 0 || valores.S.Cmp(big.NewInt(2)) != 0 {
		t.Fatalf("firma ASN.1 = (r=%s, s=%s)", valores.R, valores.S)
	}
	if _, err := firmaECDSADesdeCNG(raw[:63], publica); err == nil {
		t.Fatal("una firma raw truncada debe rechazarse")
	}
}

func TestInvertirCopiaNoMutaEntrada(t *testing.T) {
	t.Parallel()

	entrada := []byte{1, 2, 3, 4}
	salida := invertirCopia(entrada)
	if string(salida) != string([]byte{4, 3, 2, 1}) {
		t.Fatalf("invertirCopia() = %v", salida)
	}
	if string(entrada) != string([]byte{1, 2, 3, 4}) {
		t.Fatalf("invertirCopia() mutó la entrada: %v", entrada)
	}
}

func TestFiltrarCadenaOmiteHojaDuplicadosYRaiz(t *testing.T) {
	t.Parallel()

	raizDER := crearCertificadoAutofirmado(t, "Raíz", true)
	raiz, err := x509.ParseCertificate(raizDER)
	if err != nil {
		t.Fatal(err)
	}
	hoja := &x509.Certificate{Raw: []byte("hoja")}
	intermedia := &x509.Certificate{
		Raw:        []byte("intermedia"),
		RawSubject: []byte("intermedia"),
		RawIssuer:  []byte("raiz"),
	}
	resultado := filtrarCadena(hoja, []*x509.Certificate{hoja, intermedia, intermedia, raiz})
	if len(resultado) != 1 || resultado[0] != intermedia {
		t.Fatalf("filtrarCadena() = %#v", resultado)
	}
}

func partirPares(raw string) []string {
	resultado := make([]string, 0, len(raw)/2)
	for i := 0; i < len(raw); i += 2 {
		resultado = append(resultado, raw[i:i+2])
	}
	return resultado
}

func crearCertificadoAutofirmado(t *testing.T, commonName string, ca bool) []byte {
	t.Helper()
	privada, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ahora := time.Now().UTC()
	plantilla := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: commonName, Organization: []string{"Diputación de Granada"}},
		NotBefore:             ahora.Add(-time.Minute),
		NotAfter:              ahora.Add(time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  ca,
		KeyUsage:              x509.KeyUsageDigitalSignature,
	}
	if ca {
		plantilla.KeyUsage |= x509.KeyUsageCertSign
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &privada.PublicKey, privada)
	if err != nil {
		t.Fatal(err)
	}
	return der
}
