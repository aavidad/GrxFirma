// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package certaccess

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"software.sslmate.com/src/go-pkcs12"
)

func crearPerfil(t *testing.T, base, nombre, ficheroDB string) string {
	t.Helper()
	ruta := filepath.Join(base, nombre)
	if err := os.MkdirAll(ruta, 0o700); err != nil {
		t.Fatal(err)
	}
	if ficheroDB != "" {
		if err := os.WriteFile(filepath.Join(ruta, ficheroDB), []byte("db"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return ruta
}

func TestNSSDBDisponible(t *testing.T) {
	base := t.TempDir()

	conCert9 := crearPerfil(t, base, "con-cert9", "cert9.db")
	conCert8 := crearPerfil(t, base, "con-cert8", "cert8.db")
	sinDB := crearPerfil(t, base, "sin-db", "")
	otroFichero := crearPerfil(t, base, "otro", "key4.db")

	casos := []struct {
		nombre     string
		ruta       string
		disponible bool
	}{
		{"cert9.db", conCert9, true},
		{"cert8.db heredado", conCert8, true},
		{"directorio sin base NSS", sinDB, false},
		{"otro fichero no cuenta", otroFichero, false},
		{"ruta vacia", "", false},
		{"solo espacios", "   ", false},
		{"ruta inexistente", filepath.Join(base, "no-existe"), false},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			if got := nssDBDisponible(caso.ruta); got != caso.disponible {
				t.Fatalf("nssDBDisponible(%q) = %v, se esperaba %v", caso.ruta, got, caso.disponible)
			}
		})
	}
}

// nssstore ya buscaba perfiles en las tres variantes de empaquetado; certaccess
// solo miraba la nativa, asi que con Firefox de Flatpak o Snap el perfil no
// aparecia como destino de importacion aunque sus certificados si se listaran.
func TestDescubrirPerfilesFirefox_CubreNativoSnapYFlatpak(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	nativo := crearPerfil(t, filepath.Join(home, ".mozilla", "firefox"), "abc.default", "cert9.db")
	snap := crearPerfil(t, filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox"), "def.default", "cert9.db")
	flatpak := crearPerfil(t, filepath.Join(home, ".var", "app", "org.mozilla.firefox", ".mozilla", "firefox"), "ghi.default", "cert8.db")

	// Ruido que no debe aparecer: perfil sin base NSS.
	crearPerfil(t, filepath.Join(home, ".mozilla", "firefox"), "sin-db", "")

	perfiles := descubrirPerfilesFirefox()

	for _, esperado := range []string{nativo, snap, flatpak} {
		if !slices.Contains(perfiles, esperado) {
			t.Errorf("falta el perfil %q en %#v", esperado, perfiles)
		}
	}
	if len(perfiles) != 3 {
		t.Fatalf("se esperaban 3 perfiles y hay %d: %#v", len(perfiles), perfiles)
	}
	if !slices.IsSorted(perfiles) {
		t.Errorf("la lista debe salir ordenada: %#v", perfiles)
	}
}

func TestDescubrirPerfilesFirefox_SinHomeNoRompe(t *testing.T) {
	t.Setenv("HOME", "")
	if perfiles := descubrirPerfilesFirefox(); perfiles != nil {
		t.Fatalf("sin HOME no debe descubrirse nada, y devolvio %#v", perfiles)
	}
}

func TestDescubrirPerfilesFirefox_HomeVacioDevuelveNada(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if perfiles := descubrirPerfilesFirefox(); len(perfiles) != 0 {
		t.Fatalf("un home sin Firefox no debe dar perfiles: %#v", perfiles)
	}
}

// Los gestores se ofrecen a la interfaz: la lista debe salir ordenada por
// etiqueta, sin identificadores repetidos y sin comandos vacios.
func TestListarGestores_ListaCoherente(t *testing.T) {
	gestores := ListarGestores(t.TempDir())

	vistos := map[string]struct{}{}
	etiquetas := make([]string, 0, len(gestores))
	for _, g := range gestores {
		if len(g.Comando) == 0 {
			t.Errorf("el gestor %q no declara comando", g.ID)
		}
		if g.ID == "" || g.Etiqueta == "" {
			t.Errorf("gestor incompleto: %#v", g)
		}
		if _, repetido := vistos[g.ID]; repetido {
			t.Errorf("identificador de gestor repetido: %q", g.ID)
		}
		vistos[g.ID] = struct{}{}
		etiquetas = append(etiquetas, g.Etiqueta)
	}
	if !slices.IsSorted(etiquetas) {
		t.Errorf("los gestores deben salir ordenados por etiqueta: %#v", etiquetas)
	}
}

// El comando se copia al construir el gestor; mutar el slice devuelto no debe
// afectar a llamadas posteriores.
func TestListarGestores_NoCompartteElSliceDeComando(t *testing.T) {
	dir := t.TempDir()
	primera := ListarGestores(dir)
	if len(primera) == 0 {
		t.Skip("no hay gestores disponibles en este entorno")
	}
	original := append([]string(nil), primera[0].Comando...)
	primera[0].Comando[0] = "mutado"

	segunda := ListarGestores(dir)
	for _, g := range segunda {
		if g.ID == primera[0].ID && g.Comando[0] != original[0] {
			t.Fatalf("mutar el resultado altero llamadas posteriores: %q", g.Comando[0])
		}
	}
}

func TestAdapterImport_GuiadaAUnPerfilNSSReal(t *testing.T) {
	certutilPath, certutilErr := exec.LookPath("certutil")
	if certutilErr != nil {
		t.Skip("certutil no disponible")
	}
	if _, err := exec.LookPath("pk12util"); err != nil {
		t.Skip("pk12util no disponible")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	profile := filepath.Join(home, ".mozilla", "firefox", "test.default")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(certutilPath, "-N", "-d", "sql:"+profile, "--empty-password").CombinedOutput(); err != nil {
		t.Skipf("no se pudo preparar NSS de prueba: %v (%s)", err, output)
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(73),
		Subject:      pkix.Name{CommonName: "Import NSS guiado"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	p12, err := pkcs12.Legacy.Encode(key, certificate, nil, "clave-prueba")
	if err != nil {
		t.Fatal(err)
	}

	adapter := New("")
	options, err := adapter.Options(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	targetID := "nss:" + profile
	found := false
	for _, target := range options.ImportTargets {
		if target.ID == targetID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("el perfil Firefox no se ofreció como destino: %#v", options.ImportTargets)
	}
	if err := adapter.Import(context.Background(), targetID, p12, "clave-prueba"); err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	output, err := exec.Command(certutilPath, "-L", "-d", "sql:"+profile).CombinedOutput()
	if err != nil {
		t.Fatalf("certutil -L error = %v (%s)", err, output)
	}
	if !strings.Contains(string(output), "Import NSS guiado") {
		t.Fatalf("el certificado no apareció en NSS: %s", output)
	}
}
