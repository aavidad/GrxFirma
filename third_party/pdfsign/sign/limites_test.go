package sign

// Pruebas de regresión de E2-03: árbol de páginas en ciclo. Los casos que
// antes no terminaban se ejecutan con plazo.

import (
	"bytes"
	"fmt"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/pdf"
)

// pdfConObjetos escribe un PDF con xref clásica; objetos[i] es el cuerpo del
// objeto i+1 y el 1 es el catálogo.
func pdfConObjetos(objetos ...string) []byte {
	var file bytes.Buffer
	file.WriteString("%PDF-1.4\n%" + strings.Repeat("x", 230) + "\n")
	offsets := make([]int, len(objetos))
	for i, cuerpo := range objetos {
		offsets[i] = file.Len()
		fmt.Fprintf(&file, "%d 0 obj\n%s\nendobj\n", i+1, cuerpo)
	}
	xref := file.Len()
	fmt.Fprintf(&file, "xref\n0 %d\n0000000000 65535 f \n", len(objetos)+1)
	for _, off := range offsets {
		fmt.Fprintf(&file, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&file, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objetos)+1, xref)
	return file.Bytes()
}

func lectorPrueba(t *testing.T, data []byte) *pdf.Reader {
	t.Helper()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func conPlazoFirma(t *testing.T, f func() error) error {
	t.Helper()
	hecho := make(chan error, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				hecho <- fmt.Errorf("pánico recuperado: %v", p)
			}
		}()
		hecho <- f()
	}()
	select {
	case err := <-hecho:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("no terminó en 3 s: bucle o recursión sin límite")
		return nil
	}
}

func pilaReducidaFirma(t *testing.T) {
	t.Helper()
	anterior := debug.SetMaxStack(64 << 20)
	t.Cleanup(func() { debug.SetMaxStack(anterior) })
}

func TestLimites_FindFirstPageConKidsEnCiclo(t *testing.T) {
	pilaReducidaFirma(t)
	r := lectorPrueba(t, pdfConObjetos(
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [2 0 R] /Count 1 >>",
	))
	err := conPlazoFirma(t, func() error {
		_, err := findFirstPage(r.Trailer().Key("Root").Key("Pages"))
		return err
	})
	if err == nil {
		t.Fatal("se encontró una primera página en un árbol en ciclo")
	}
}

func TestLimites_FindPageByNumberConKidsEnCiclo(t *testing.T) {
	pilaReducidaFirma(t)
	for _, caso := range []struct {
		nombre  string
		objetos []string
	}{
		{"a sí mismo", []string{
			"<< /Type /Catalog /Pages 2 0 R >>",
			"<< /Type /Pages /Kids [2 0 R] /Count 1 >>",
		}},
		// Muchos hijos que apuntan al mismo nodo: sin conjunto de visitados el
		// recorrido crece de forma exponencial aunque se limite la profundidad.
		{"hijos repetidos", []string{
			"<< /Type /Catalog /Pages 2 0 R >>",
			"<< /Type /Pages /Kids [3 0 R 3 0 R 3 0 R 3 0 R] /Count 1 >>",
			"<< /Type /Pages /Kids [2 0 R 3 0 R 3 0 R 3 0 R] /Count 1 >>",
		}},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			r := lectorPrueba(t, pdfConObjetos(caso.objetos...))
			err := conPlazoFirma(t, func() error {
				_, err := findPageByNumber(r.Trailer().Key("Root").Key("Pages"), 1)
				return err
			})
			if err == nil {
				t.Fatal("se encontró la página 1 en un árbol en ciclo")
			}
		})
	}
}

// El recorrido correcto sigue encontrando páginas en árboles de varios niveles.
func TestLimites_FindPageByNumberArbolCorrecto(t *testing.T) {
	r := lectorPrueba(t, pdfConObjetos(
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 3 >>",
		"<< /Type /Pages /Kids [5 0 R 6 0 R] /Count 2 /Parent 2 0 R >>",
		"<< /Type /Page /Parent 2 0 R >>",
		"<< /Type /Page /Parent 3 0 R >>",
		"<< /Type /Page /Parent 3 0 R >>",
	))
	pages := r.Trailer().Key("Root").Key("Pages")
	for numero, id := range map[uint32]uint32{1: 5, 2: 6, 3: 4} {
		page, err := findPageByNumber(pages, numero)
		if err != nil {
			t.Fatalf("página %d: %v", numero, err)
		}
		if got, _ := page.ObjectReference(); got != id {
			t.Fatalf("página %d: objeto %d, se esperaba %d", numero, got, id)
		}
	}
	if _, err := findPageByNumber(pages, 4); err == nil {
		t.Fatal("se encontró una página inexistente")
	}
	first, err := findFirstPage(pages)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := first.ObjectReference(); got != 5 {
		t.Fatalf("primera página: objeto %d", got)
	}
}
