// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package purity verifica que el proyecto no tiene dependencias externas no autorizadas.
//
// Se prefieren implementaciones en Go nativo. Las dependencias externas se
// mantienen detrás de puertos cuando son inevitables.
//
// Las dependencias externas actualmente bloqueadas son las que se habian
// colado en fases anteriores y han sido reemplazadas por implementaciones
// en Go nativo:
//   - github.com/beevik/etree (XML; reemplazado por encoding/xml stdlib)
//   - github.com/russellhaering/goxmldsig (canonicalizacion XML; reemplazado)
//   - github.com/digitorus/pdfsign se reintroduce solo cuando hace falta
//     firma PDF incremental real con sello visible sobre el documento original.
//
// Si en el futuro se aprueba una dependencia externa inevitable (ej. pkcs11,
// biometria nativa), debe anadirse explicitamente a la lista de permitidas
// en este fichero junto con el ADR que justifica su inclusion.
package purity_test

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// dependenciasProhibidas lista paquetes que no deben aparecer en go.mod.
// Cada entrada incluye el motivo de la prohibicion para facilitar la auditoria.
var dependenciasProhibidas = []struct {
	modulo string
	motivo string
}{
	{"github.com/beevik/etree", "reemplazado por encoding/xml de la stdlib"},
	{"github.com/russellhaering/goxmldsig", "reemplazado por implementacion XAdES nativa"},
}

// TestSinDependenciasProhibidas falla si go.mod declara alguna dependencia prohibida.
// Ejecutar con: go test ./test/purity/...
func TestSinDependenciasProhibidas(t *testing.T) {
	gomod := encontrarGoMod(t)
	lineas := leerLineas(t, gomod)

	for _, dep := range dependenciasProhibidas {
		for i, linea := range lineas {
			if strings.Contains(linea, dep.modulo) {
				t.Errorf("linea %d: dependencia prohibida detectada en go.mod: %q\n  motivo: %s",
					i+1, strings.TrimSpace(linea), dep.motivo)
			}
		}
	}
}

// dependenciasPermitidas lista las dependencias externas aprobadas explicitamente.
// Cada entrada requiere justificacion. Anadir aqui antes de anadir a go.mod.
var dependenciasPermitidas = []string{
	// github.com/gowebpki/jcs: implementación fijada de RFC 8785 necesaria para
	// interoperar byte a byte en identidad-reforzada/v1. Se confina al adaptador
	// outbound/common/identityjcs y su licencia Apache-2.0 está inventariada.
	"github.com/gowebpki/jcs",
	// software.sslmate.com/src/go-pkcs12: fork mantenido y compatible con PKCS#12 modernos.
	// Se usa exclusivamente detras del adaptador common/pkcs12importer porque
	// golang.org/x/crypto/pkcs12 esta congelado y falla con P12 modernos.
	"software.sslmate.com/src/go-pkcs12",
	// golang.org/x/crypto: dependencia transitiva del modulo go-pkcs12.
	// Se acepta solo como soporte interno de esa libreria encapsulada.
	"golang.org/x/crypto",
	// github.com/miekg/pkcs11: adaptador PKCS#11 para DNIe y tarjetas hardware (T033, aprobado en CONSENSO.md).
	// No existe implementacion Go nativa equivalente; wrapping CGo del estandar PKCS#11.
	"github.com/miekg/pkcs11",
	// github.com/digitorus/pdfsign: requerido para firma PDF incremental real
	// y sello visible sobre el PDF original. La implementación nativa actual
	// cubre PAdES básico, pero no apariencias visuales avanzadas ni preservación
	// robusta del documento original.
	"github.com/digitorus/pdfsign",
	// Dependencias transitivas directas de pdfsign.
	"github.com/digitorus/pdf",
	"github.com/digitorus/pkcs7",
	"github.com/digitorus/timestamp",
	"github.com/mattetti/filebuffer",
	// github.com/skip2/go-qrcode: generación local del QR embebido en el sello visible PAdES.
	// Se usa solo para componer la apariencia visual del sello en el adaptador desktop/signer.
	"github.com/skip2/go-qrcode",
	// github.com/makiuchi-d/gozxing: lectura del QR tributario Veri*Factu desde
	// una imagen o un PDF rasterizado. Adaptación a Go puro de ZXing (sin CGo ni
	// red), licencia MIT con el aviso Apache-2.0 de ZXing. Implementar a mano la
	// detección, la corrección de perspectiva y Reed-Solomon sería claramente
	// peor. Confinada a outbound/common/signer/verifactu_qr_imagen.go, que limita
	// bytes, dimensiones, píxeles y tiempo antes de llamarla.
	"github.com/makiuchi-d/gozxing",
	// golang.org/x/xerrors: dependencia transitiva de gozxing (equipo Go).
	"golang.org/x/xerrors",
	// golang.org/x/sys: syscalls de plataforma para Windows CertStore (T034) y macOS Keychain (T035).
	// Misma familia que golang.org/x/crypto; mantenida por el equipo Go.
	"golang.org/x/sys",
	// github.com/Microsoft/go-winio: implementacion mantenida por Microsoft de
	// named pipes con I/O cancelable, rechazo de clientes remotos y DACL por
	// sesion de logon. Confinada al listener IPC de Windows 10 y
	// aprobada en docs/ADR-005-ipc-windows-named-pipe.md.
	"github.com/Microsoft/go-winio",
	// golang.org/x/term: lectura de contraseñas sin eco desde terminal en Linux,
	// macOS y consolas de Windows 10 sin introducir dependencias nativas.
	"golang.org/x/term",
	// github.com/zalando/go-keyring: implementación de ports.SecureStorage (T057)
	// sobre el almacén nativo (Secret Service D-Bus en Linux en Go puro, Keychain
	// en macOS, Credential Manager en Windows). Implementar el protocolo Secret
	// Service a mano sería claramente peor. Solo se importa desde el adaptador
	// common/keyringstore.
	"github.com/zalando/go-keyring",
	// github.com/danieljoos/wincred: dependencia transitiva de go-keyring para
	// el Credential Manager de Windows (respaldado por DPAPI).
	"github.com/danieljoos/wincred",
	// github.com/godbus/dbus/v5: dependencia transitiva de go-keyring para el
	// protocolo Secret Service en Linux (D-Bus en Go puro, sin CGo).
	"github.com/godbus/dbus/v5",

	// --- Fyne v2 y sus dependencias transitivas ---
	// fyne.io/fyne/v2: toolkit GUI desktop aprobado en CONSENSO.md para T038, T040.
	// Solo se importa desde presentation/desktop/; nunca cruza a internal/.
	"fyne.io/fyne/v2",
	// fyne.io/systray: sistema de bandeja, dependencia de Fyne v2.
	"fyne.io/systray",
	// Dependencias de renderizado y gráficos de Fyne v2:
	"github.com/fyne-io/gl-js",
	"github.com/fyne-io/glfw-js",
	"github.com/fyne-io/image",
	"github.com/fyne-io/oksvg",
	"github.com/go-gl/gl",
	"github.com/go-gl/glfw",
	"github.com/go-text/render",
	"github.com/go-text/typesetting",
	"github.com/srwiley/oksvg",
	"github.com/srwiley/rasterx",
	"github.com/nfnt/resize",
	"github.com/jsummers/gobmp",
	// Dependencias de i18n y localización de Fyne v2:
	"github.com/jeandeaual/go-locale",
	"github.com/nicksnyder/go-i18n",
	"golang.org/x/text",
	// Dependencias de URI y configuración de Fyne v2:
	"github.com/fredbi/uri",
	"github.com/BurntSushi/toml",
	// Dependencias de notificaciones y D-Bus de Fyne v2 (Linux):
	"github.com/godbus/dbus",
	"github.com/rymdport/portal",
	// Dependencias WebAssembly de Fyne v2:
	"github.com/hack-pad/go-indexeddb",
	"github.com/hack-pad/safejs",
	// Dependencias de filesystem de Fyne v2:
	"github.com/fsnotify/fsnotify",
	// Dependencias de Markdown en widgets de Fyne v2:
	"github.com/yuin/goldmark",
	// Dependencias de imagen de Fyne v2:
	"golang.org/x/image",
	// Dependencias de red de Fyne v2 (WebAssembly):
	"golang.org/x/net",
	// github.com/deatil/go-cryptobin: necesario para CMS SignedAndEnveloped (sobre TODO T-CMS-02).
	// github.com/digitorus/pkcs7 y la stdlib no implementan SignedAndEnvelopedData; esta librería sí.
	// Confinada a internal/adapters/outbound/common/protector/.
	"github.com/deatil/go-cryptobin",
	// Dependencias de test transitivas (go-pkcs12, Fyne):
	"github.com/davecgh/go-spew",
	"github.com/pmezard/go-difflib",
	"github.com/stretchr/testify",
	"github.com/kr/text",
	"gopkg.in/yaml.v3",
}

// TestJCSConfinadoAlAdaptador evita que JSON o la librería RFC 8785 entren en el núcleo.
func TestJCSConfinadoAlAdaptador(t *testing.T) {
	raiz := raizModulo(t)
	for _, relativo := range []string{"internal/domain", "internal/application", "internal/ports"} {
		directorio := filepath.Join(raiz, filepath.FromSlash(relativo))
		err := filepath.WalkDir(directorio, func(path string, entrada os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entrada.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			contenido, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(contenido), "github.com/gowebpki/jcs") {
				t.Errorf("JCS cruzó la frontera hexagonal: %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("revisar %s: %v", directorio, err)
		}
	}
}

// TestGozxingConfinadoAlLectorQR mantiene el decodificador QR detrás de los
// límites de verifactu_qr_imagen.go.
func TestGozxingConfinadoAlLectorQR(t *testing.T) {
	raiz := raizModulo(t)
	for _, relativo := range []string{"internal", "cmd", "presentation"} {
		directorio := filepath.Join(raiz, filepath.FromSlash(relativo))
		if _, err := os.Stat(directorio); err != nil {
			continue
		}
		err := filepath.WalkDir(directorio, func(path string, entrada os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entrada.IsDir() || filepath.Ext(path) != ".go" || filepath.Base(path) == "verifactu_qr_imagen.go" {
				return nil
			}
			contenido, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(contenido), "github.com/makiuchi-d/gozxing") {
				t.Errorf("gozxing fuera del lector QR acotado: %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("revisar %s: %v", directorio, err)
		}
	}
}

// TestGoModSoloPermitidas verifica que go.mod solo tiene dependencias de la lista aprobada.
// Para anadir una nueva dependencia: justificarla aqui y en dependenciasPermitidas.
func TestGoModSoloPermitidas(t *testing.T) {
	gomod := encontrarGoMod(t)
	lineas := leerLineas(t, gomod)

	inRequire := false
	for i, linea := range lineas {
		trim := strings.TrimSpace(linea)
		if strings.HasPrefix(trim, "require") {
			inRequire = true
		}
		if inRequire && trim == ")" {
			inRequire = false
		}
		if !inRequire {
			continue
		}
		// Ignorar lineas de apertura/cierre del bloque.
		if trim == "require" || trim == "require (" || trim == ")" || trim == "" {
			continue
		}
		// Verificar que la dependencia esta en la lista de permitidas.
		permitida := false
		for _, dep := range dependenciasPermitidas {
			if strings.Contains(linea, dep) {
				permitida = true
				break
			}
		}
		if !permitida {
			t.Errorf("linea %d: dependencia no aprobada en go.mod: %q\n"+
				"  Justificala en test/purity/purity_test.go antes de usarla.",
				i+1, trim)
		}
	}
}

// TestMobileNoServerSocket verifica que los adaptadores mobile no introducen listeners locales.
func TestMobileNoServerSocket(t *testing.T) {
	raiz := raizModulo(t)
	directorios := []string{
		filepath.Join(raiz, "internal", "adapters", "inbound", "mobile"),
		filepath.Join(raiz, "internal", "adapters", "outbound", "mobile"),
	}
	patronesProhibidos := []string{
		"net.Listen(",
		"net.ListenTCP(",
		"net.ListenUnix(",
		"http.ListenAndServe(",
		"http.ListenAndServeTLS(",
	}

	for _, dir := range directorios {
		filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				t.Fatalf("error recorriendo %s: %v", path, err)
			}
			if d.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("no se pudo leer %s: %v", path, err)
			}
			texto := string(data)
			for _, patron := range patronesProhibidos {
				if strings.Contains(texto, patron) {
					t.Fatalf("patron prohibido %q encontrado en %s", patron, path)
				}
			}
			return nil
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func encontrarGoMod(t *testing.T) string {
	t.Helper()
	ruta := filepath.Join(raizModulo(t), "go.mod")
	if _, err := os.Stat(ruta); err != nil {
		t.Fatalf("no se encontro go.mod en %s: %v", ruta, err)
	}
	return ruta
}

func raizModulo(t *testing.T) string {
	t.Helper()
	_, ficheroTest, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("no se pudo obtener la ruta del fichero de test")
	}
	return filepath.Join(filepath.Dir(ficheroTest), "..", "..")
}

func leerLineas(t *testing.T, ruta string) []string {
	t.Helper()
	f, err := os.Open(ruta)
	if err != nil {
		t.Fatalf("no se pudo abrir %s: %v", ruta, err)
	}
	defer f.Close()

	var lineas []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lineas = append(lineas, sc.Text())
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("error leyendo %s: %v", ruta, err)
	}
	return lineas
}
