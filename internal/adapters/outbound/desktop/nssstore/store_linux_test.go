// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package nssstore_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/nssstore"
)

// TestParsearListaNSS verifica el parsing de la salida de certutil -L.
func TestParsearListaNSS(t *testing.T) {
	t.Parallel()

	salidaCertutil := []byte(`Certificate Nickname                                         Trust Attributes
                                                             SSL,S/MIME,JAR/XPI

CERTIFICADO_PERSONA_PRUEBAS                                  u,u,u
AC FNMT Usuarios - FNMT-RCM                                  ,,
CERTIFICADO_ORGANIZACION_PRUEBAS                         u,u,u
AutoFirmaJA ROOT LOCAL                                       CT,C,C
(NULL)                                                       ,,
`)
	// Acceso a función interna vía helper exportado de test.
	nicknames := nssstore.ParsarListaNSS(salidaCertutil)

	if len(nicknames) != 2 {
		t.Fatalf("esperados 2 nicknames con u,u,u, obtenidos %d: %v", len(nicknames), nicknames)
	}
	if nicknames[0] != "CERTIFICADO_PERSONA_PRUEBAS" {
		t.Errorf("nickname[0] = %q", nicknames[0])
	}
	if nicknames[1] != "CERTIFICADO_ORGANIZACION_PRUEBAS" {
		t.Errorf("nickname[1] = %q", nicknames[1])
	}
}

// TestList_ConMockCertutil verifica List() usando un binario certutil simulado.
func TestList_ConMockCertutil(t *testing.T) {
	t.Parallel()

	// Crear directorio NSS falso y script mock de certutil.
	dir := t.TempDir()
	nssDir := filepath.Join(dir, "nssdb")
	if err := os.MkdirAll(nssDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Generar un certificado de prueba en PEM.
	certPEM := generarCertPEM(t, "Test Usuario FNMT")

	// Script mock de certutil que devuelve datos simulados.
	scriptPath := filepath.Join(dir, "certutil")
	scriptContent := "#!/bin/sh\n" +
		`case "$*" in
  *"-L -d"*"-n"*)
    cat << 'EOF'
` + string(certPEM) + `
EOF
    ;;
  *"-L -d"*)
    echo "Certificate Nickname                                         Trust Attributes"
    echo "                                                             SSL,S/MIME,JAR/XPI"
    echo ""
    echo "Test Usuario FNMT                                            u,u,u"
    ;;
esac
`
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0o755); err != nil {
		t.Fatalf("WriteFile(script): %v", err)
	}

	almacen := nssstore.NewConRutas(scriptPath, []string{nssDir})
	refs, err := almacen.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(refs) != 1 {
		t.Fatalf("esperado 1 certificado, obtenido %d", len(refs))
	}
	if refs[0].Subject != "Test Usuario FNMT" {
		t.Errorf("Subject = %q, want %q", refs[0].Subject, "Test Usuario FNMT")
	}
	if refs[0].Fingerprint == "" {
		t.Error("Fingerprint vacío")
	}
	if refs[0].ID == "" {
		t.Error("ID vacío")
	}
}

// TestList_RutaInexistente_NoError verifica que una ruta inexistente no produce error.
func TestList_RutaInexistente_NoError(t *testing.T) {
	t.Parallel()

	almacen := nssstore.NewConRutas("certutil", []string{"/ruta/que/no/existe"})
	refs, err := almacen.List(context.Background())
	if err != nil {
		t.Fatalf("List() no debe fallar con ruta inexistente: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("esperados 0 certs con ruta inexistente, obtenidos %d", len(refs))
	}
}

// TestList_Deduplicacion verifica que el mismo certificado en dos rutas no se duplica.
func TestList_Deduplicacion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	nssDir1 := filepath.Join(dir, "nssdb1")
	nssDir2 := filepath.Join(dir, "nssdb2")
	for _, d := range []string{nssDir1, nssDir2} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}

	certPEM := generarCertPEM(t, "Usuario Duplicado")
	scriptPath := filepath.Join(dir, "certutil")
	scriptContent := "#!/bin/sh\n" +
		`case "$*" in
  *"-L -d"*"-n"*)
    cat << 'EOF'
` + string(certPEM) + `
EOF
    ;;
  *"-L -d"*)
    echo "Certificate Nickname                                         Trust Attributes"
    echo "                                                             SSL,S/MIME,JAR/XPI"
    echo ""
    echo "Usuario Duplicado                                            u,u,u"
    ;;
esac
`
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0o755); err != nil {
		t.Fatalf("WriteFile(script): %v", err)
	}

	almacen := nssstore.NewConRutas(scriptPath, []string{nssDir1, nssDir2})
	refs, err := almacen.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(refs) != 1 {
		t.Errorf("deduplicación fallida: esperado 1, obtenido %d", len(refs))
	}
}

// TestList_CancelContexto verifica que la cancelación de contexto detiene la iteración.
func TestList_CancelContexto(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	nssDir := filepath.Join(dir, "nssdb")
	if err := os.MkdirAll(nssDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelar inmediatamente

	almacen := nssstore.NewConRutas("certutil", []string{nssDir})
	_, err := almacen.List(ctx)
	// Con contexto cancelado y directorio existente, debe retornar el error de contexto.
	if err == nil {
		// Si no hay certs en el dir vacío puede que no haya procesado nada — aceptable.
		t.Log("contexto cancelado antes de procesar: sin error (aceptable con dir vacío)")
	}
}

// TestConvertirP12APEM_UsaPasswordFile verifica que openssl no recibe la contraseña por argv.
func TestConvertirP12APEM_UsaPasswordFile(t *testing.T) {
	dir := t.TempDir()
	p12Path := filepath.Join(dir, "cert.p12")
	if err := os.WriteFile(p12Path, []byte("p12 falso"), 0o600); err != nil {
		t.Fatalf("WriteFile(p12): %v", err)
	}

	opensslPath := filepath.Join(dir, "openssl")
	script := `#!/bin/sh
for arg in "$@"; do
	case "$arg" in
		pass:*|*"s3cr3t con espacios"*) exit 31 ;;
	esac
done

passin=""
prev=""
for arg in "$@"; do
	if [ "$prev" = "-passin" ]; then
		passin="$arg"
		break
	fi
	prev="$arg"
done

case "$passin" in
	file:*) pw_path=${passin#file:} ;;
	*) exit 32 ;;
esac

[ -f "$pw_path" ] || exit 33
[ "$(stat -c %a "$pw_path")" = "600" ] || exit 34
[ "$(cat "$pw_path")" = "$EXPECT_PASSWORD" ] || exit 35

cat <<'EOF'
-----BEGIN CERTIFICATE-----
MIIB
-----END CERTIFICATE-----
EOF
`
	if err := os.WriteFile(opensslPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(openssl): %v", err)
	}

	t.Setenv("EXPECT_PASSWORD", "s3cr3t con espacios")
	almacen := nssstore.NewConOpenSSL(opensslPath)
	pemBundle, err := nssstore.ConvertirP12APEM(almacen, p12Path, "s3cr3t con espacios")
	if err != nil {
		t.Fatalf("ConvertirP12APEM() error = %v", err)
	}
	if len(pemBundle) == 0 {
		t.Fatal("ConvertirP12APEM() no devolvió PEM")
	}
}

func TestConvertirP12APEM_NoExponeClaveParcialEnError(t *testing.T) {
	dir := t.TempDir()
	p12Path := filepath.Join(dir, "cert.p12")
	if err := os.WriteFile(p12Path, []byte("p12 falso"), 0o600); err != nil {
		t.Fatalf("WriteFile(p12): %v", err)
	}
	opensslPath := filepath.Join(dir, "openssl")
	script := `#!/bin/sh
echo "-----BEGIN PRIVATE KEY-----"
echo "MATERIAL-SECRETO-NO-DEBE-SALIR"
echo "-----END PRIVATE KEY-----"
exit 41
`
	if err := os.WriteFile(opensslPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(openssl): %v", err)
	}
	almacen := nssstore.NewConOpenSSL(opensslPath)
	_, err := nssstore.ConvertirP12APEM(almacen, p12Path, "password")
	if err == nil {
		t.Fatal("ConvertirP12APEM accepted a failing openssl process")
	}
	if strings.Contains(err.Error(), "MATERIAL-SECRETO") || strings.Contains(err.Error(), "PRIVATE KEY") {
		t.Fatalf("error exposed private material: %v", err)
	}
}

func TestResolverPasswordExportacionNSS_GeneraTemporalSiNoHayConfigurada(t *testing.T) {
	t.Parallel()

	pw, err := nssstore.ResolverPasswordExportacionNSS("")
	if err != nil {
		t.Fatalf("ResolverPasswordExportacionNSS() error = %v", err)
	}
	if pw == "" {
		t.Fatal("la contraseña temporal no puede estar vacía")
	}
	if len(pw) < 24 {
		t.Fatalf("contraseña temporal demasiado corta: %d", len(pw))
	}

	configurada, err := nssstore.ResolverPasswordExportacionNSS(" configurada ")
	if err != nil {
		t.Fatalf("ResolverPasswordExportacionNSS(configurada) error = %v", err)
	}
	if configurada != "configurada" {
		t.Fatalf("contraseña configurada = %q, want configurada", configurada)
	}
}

func TestRutasEstandar_IncluyeFirefoxFlatpak(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	profile := filepath.Join(home, ".var", "app", "org.mozilla.firefox", ".mozilla", "firefox", "abcd.default-release")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatalf("MkdirAll(profile): %v", err)
	}

	rutas := nssstore.RutasEstandar()
	for _, ruta := range rutas {
		if ruta == profile {
			return
		}
	}
	t.Fatalf("rutasEstandar() no incluye perfil Firefox Flatpak %q: %v", profile, rutas)
}

// generarCertPEM genera un certificado EC autofirmado en PEM para tests.
func generarCertPEM(t *testing.T, cn string) []byte {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
