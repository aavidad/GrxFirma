// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"grxfirma/internal/security/excepciones"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/pkcs7"

	"grxfirma/internal/adapters/outbound/common/localizador"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/security/machinepolicy"
)

func cadesImplicitaSHA1Prueba(t *testing.T, datos []byte) []byte {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(2012), Subject: pkix.Name{CommonName: "Firmante de 2012"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	firmado, err := pkcs7.NewSignedData(datos)
	if err != nil {
		t.Fatal(err)
	}
	firmado.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA1)
	if err := firmado.AddSigner(cert, priv, pkcs7.SignerInfoConfig{}); err != nil {
		t.Fatal(err)
	}
	cms, err := firmado.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return cms
}

func generarENISHA1(t *testing.T, cms []byte) (rc int, stderr string, destino string) {
	t.Helper()
	tmp := t.TempDir()
	firma := filepath.Join(tmp, "antigua.csig")
	if err := os.WriteFile(firma, cms, 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut strings.Builder
	a := New(nil, nil)
	a.Localizador = localizador.Para("es")
	a.Stdout, a.Stderr = &out, &errOut
	rc = a.Run(context.Background(), []string{"-operacion", "generar-eni", "-entrada", firma,
		"-opcion", "eni.organo=L01180877", "-opcion", "eni.origen=ciudadano", "-opcion", "eni.tipoDocumental=TD14",
		"-opcion", "eni.formato=TXT"})
	return rc, errOut.String(), filepath.Join(tmp, "antigua_eni.xml")
}

// Lote 16c (KD-05 y SHA-1): la CLI envuelve una CAdES implícita SHA-1 que
// corresponde y lo avisa en la salida de error con el texto del catálogo.
func TestRunGenerarENI_CAdESSHA1QueCorrespondeAvisa(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_sha1_legacy")
	excepciones.SetForTest(t, "sha1_conservar", true)
	machinepolicy.UnsetForTest(t, machinepolicy.PermitirSHA1Legacy)
	rc, stderr, destino := generarENISHA1(t, cadesImplicitaSHA1Prueba(t, []byte("acta de la sesión de 2012")))
	aviso := localizador.Para("es").T(commonsigner.AvisoFirmaSHA1Corresponde)
	if rc != 0 || !strings.Contains(stderr, aviso) || !strings.Contains(stderr, localizador.Para("es").T("cli.aviso.prefix")) {
		t.Fatalf("rc=%d stderr=%s", rc, stderr)
	}
	if _, err := os.Stat(destino); err != nil {
		t.Fatalf("no se generó el ENI: %v", err)
	}
}

func TestRunGenerarENI_CAdESSHA1AlteradaSeRechaza(t *testing.T) {
	machinepolicy.UnsetForTest(t, machinepolicy.PermitirSHA1Legacy)
	cms := cadesImplicitaSHA1Prueba(t, []byte("acta de la sesión de 2012"))
	i := strings.Index(string(cms), "2012")
	if i < 0 {
		t.Fatal("no se encontró el contenido encapsulado")
	}
	cms[i] = '3'
	rc, stderr, destino := generarENISHA1(t, cms)
	if rc == 0 || !strings.Contains(stderr, localizador.Para("es").T("eni.error.signature_mismatch")) {
		t.Fatalf("rc=%d stderr=%s", rc, stderr)
	}
	if _, err := os.Stat(destino); err == nil {
		t.Fatal("se generó el ENI con una CAdES SHA-1 alterada")
	}
}

func TestRunGenerarENI_CAdESSHA1LoCierraLaPolitica(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirSHA1Legacy, false)
	rc, stderr, destino := generarENISHA1(t, cadesImplicitaSHA1Prueba(t, []byte("acta de la sesión de 2012")))
	if rc == 0 || !strings.Contains(stderr, localizador.Para("es").T("excepciones.sha1_conservar.bloqueo")) {
		t.Fatalf("rc=%d stderr=%s", rc, stderr)
	}
	if _, err := os.Stat(destino); err == nil {
		t.Fatal("se generó el ENI pese a la política que prohíbe SHA-1")
	}
}
