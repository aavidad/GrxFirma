// Pruebas del descifrado añadido por AutoFirmaV2 (AES-128, AES-256 y
// contraseñas de usuario y propietario). Los PDF de testdata son un
// formulario sintético cifrado con qpdf.

package pdf

import (
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
)

func abrirPrueba(t *testing.T, nombre, password string) (*Reader, error) {
	t.Helper()
	f, err := os.Open("testdata/" + nombre)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	info, _ := f.Stat()
	if password == "" {
		return NewReader(f, info.Size())
	}
	return NewReaderConContrasena(f, info.Size(), password)
}

func comprobarContenido(t *testing.T, nombre string, r *Reader) {
	t.Helper()
	campo := r.Trailer().Key("Root").Key("AcroForm").Key("Fields").Index(0)
	if v := campo.Key("V").Text(); v != "Ana" {
		t.Errorf("%s: valor del campo descifrado = %q", nombre, v)
	}
	contenido, err := io.ReadAll(r.Page(1).V.Key("Contents").Reader())
	if err != nil || !strings.Contains(string(contenido), "Solicitud") {
		t.Errorf("%s: contenido de la página = %q, %v", nombre, contenido, err)
	}
}

func TestCifrado_SinContrasenaDeUsuario(t *testing.T) {
	for _, c := range []struct {
		nombre      string
		aes, aes256 bool
		version     int
	}{
		{"form-rc4.pdf", false, false, 2},
		{"form-aes128.pdf", true, false, 4},
		{"form-aes256.pdf", true, true, 5},
	} {
		r, err := abrirPrueba(t, c.nombre, "")
		if err != nil {
			t.Fatalf("%s: %v", c.nombre, err)
		}
		p := r.Cifrado()
		if !p.Presente || p.AES != c.aes || p.AES256 != c.aes256 || p.V != c.version || len(p.Clave) == 0 || p.Propietario {
			t.Errorf("%s: parámetros inesperados %+v", c.nombre, p)
		}
		comprobarContenido(t, c.nombre, r)
	}
}

func TestCifrado_ContrasenasDeUsuarioYPropietario(t *testing.T) {
	for _, nombre := range []string{"form-aes128-usuario.pdf", "form-aes256-usuario.pdf"} {
		if _, err := abrirPrueba(t, nombre, ""); err != ErrInvalidPassword {
			t.Errorf("%s: sin contraseña debe fallar con ErrInvalidPassword: %v", nombre, err)
		}
		if _, err := abrirPrueba(t, nombre, "otra"); err == nil {
			t.Errorf("%s: una contraseña incorrecta debe fallar", nombre)
		}
		for password, propietario := range map[string]bool{"usuario": false, "propietario": true} {
			r, err := abrirPrueba(t, nombre, password)
			if err != nil {
				t.Fatalf("%s con %s: %v", nombre, password, err)
			}
			if r.Cifrado().Propietario != propietario {
				t.Errorf("%s con %s: propietario=%v", nombre, password, r.Cifrado().Propietario)
			}
			comprobarContenido(t, nombre, r)
		}
	}
}

// Una tabla de referencias con subsecciones enormes o negativas agotaba la
// memoria o abortaba con un pánico; ahora se rechaza.
func TestXrefMaliciosaSeRechaza(t *testing.T) {
	for _, sub := range []string{"99999999999 1", "-5 1", "0 99999999999"} {
		cuerpo := "%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n"
		xref := len(cuerpo)
		pdf := cuerpo + "xref\n" + sub + "\n0000000009 00000 n \ntrailer\n<< /Size 2 /Root 1 0 R >>\nstartxref\n" + strconv.Itoa(xref) + "\n%%EOF\n"
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("subsección %q: pánico %v", sub, r)
				}
			}()
			if _, err := NewReader(strings.NewReader(pdf), int64(len(pdf))); err == nil {
				t.Errorf("subsección %q: se esperaba un error", sub)
			}
		}()
	}
}
