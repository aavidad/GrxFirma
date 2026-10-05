package sign

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/pdf"
)

// Pruebas de AutoFirmaV2 con árboles de páginas y formularios maliciosos:
// sin los límites colgaban la firma, agotaban la memoria o la pila.

func pdfSinteticoSign(objs []string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

func lectorSintetico(t *testing.T, objs []string) *pdf.Reader {
	t.Helper()
	data := pdfSinteticoSign(objs)
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// conPlazoSign falla si f no termina a tiempo.
func conPlazoSign(t *testing.T, f func()) {
	t.Helper()
	hecho := make(chan any, 1)
	go func() {
		defer func() { hecho <- recover() }()
		f()
	}()
	plazo := 10 * time.Second
	if d, ok := t.Deadline(); ok && time.Until(d) < plazo {
		plazo = time.Until(d) / 2
	}
	select {
	case p := <-hecho:
		if p != nil {
			panic(p)
		}
	case <-time.After(plazo):
		t.Fatal("el recorrido no terminó: bucle sin límite ante un PDF malicioso")
	}
}

func arbolCiclicoSign() []string {
	return []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 5 >>",
		"<< /Type /Pages /Parent 2 0 R /Kids [2 0 R] /Count 5 >>",
	}
}

func arbolProfundoSign(n int) []string {
	objs := []string{"<< /Type /Catalog /Pages 2 0 R >>"}
	for i := 0; i < n; i++ {
		objs = append(objs, fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R] /Count 1 >>", 3+i))
	}
	return append(objs, "<< /Type /Page /MediaBox [0 0 595 842] >>")
}

func TestFindPageByNumber_ArbolMalicioso(t *testing.T) {
	for _, c := range []struct {
		nombre string
		objs   []string
	}{
		{"ciclo", arbolCiclicoSign()},
		{"profundidad-10000", arbolProfundoSign(10000)},
		{"autorreferencia", []string{
			"<< /Type /Catalog /Pages 2 0 R >>",
			"<< /Type /Pages /Kids [2 0 R] /Count 1 >>",
		}},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			r := lectorSintetico(t, c.objs)
			conPlazoSign(t, func() {
				pages := r.Trailer().Key("Root").Key("Pages")
				if _, err := findPageByNumber(pages, 1); err == nil {
					t.Error("se esperaba un error")
				}
				if _, err := findFirstPage(pages); err == nil {
					t.Error("findFirstPage: se esperaba un error")
				}
			})
		})
	}
	// Árbol normal con nodos intermedios.
	r := lectorSintetico(t, []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 3 >>",
		"<< /Type /Page /Parent 2 0 R /N 1 >>",
		"<< /Type /Pages /Parent 2 0 R /Kids [5 0 R 6 0 R] /Count 2 >>",
		"<< /Type /Page /Parent 4 0 R /N 2 >>",
		"<< /Type /Page /Parent 4 0 R /N 3 >>",
	})
	pages := r.Trailer().Key("Root").Key("Pages")
	for n := uint32(1); n <= 3; n++ {
		p, err := findPageByNumber(pages, n)
		if err != nil || p.Key("N").Int64() != int64(n) {
			t.Fatalf("página %d: %v %v", n, p.Key("N"), err)
		}
	}
	if _, err := findPageByNumber(pages, 4); err == nil {
		t.Fatal("la página 4 no existe")
	}
}

// Con «todas las páginas», un /Count enorme reservaba memoria sin límite.
func TestResolveAppearancePages_CountMalicioso(t *testing.T) {
	for _, c := range []struct {
		nombre string
		objs   []string
	}{
		{"count-enorme", []string{
			"<< /Type /Catalog /Pages 2 0 R >>",
			"<< /Type /Pages /Kids [3 0 R] /Count 2000000000 >>",
			"<< /Type /Page /Parent 2 0 R >>",
		}},
		{"count-incoherente", []string{
			"<< /Type /Catalog /Pages 2 0 R >>",
			"<< /Type /Pages /Kids [3 0 R] /Count 15000 >>",
			"<< /Type /Page /Parent 2 0 R >>",
		}},
		{"ciclo", arbolCiclicoSign()},
		{"profundidad-10000", arbolProfundoSign(10000)},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			ctx := SignContext{PDFReader: lectorSintetico(t, c.objs)}
			ctx.SignData.Appearance.AllPages = true
			conPlazoSign(t, func() {
				if pages, err := ctx.resolveAppearancePages(); err == nil {
					t.Errorf("se esperaba un error, no %d páginas", len(pages))
				}
			})
		})
	}
	ctx := SignContext{PDFReader: lectorSintetico(t, []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		"<< /Type /Page /Parent 2 0 R >>",
		"<< /Type /Page /Parent 2 0 R >>",
	})}
	ctx.SignData.Appearance.AllPages = true
	if pages, err := ctx.resolveAppearancePages(); err != nil || len(pages) != 2 {
		t.Fatalf("documento correcto: %v %v", pages, err)
	}
}

// Una referencia del objeto a sí mismo se expandía sin fin al reescribirlo.
func TestSerializeCatalogEntry_AutorreferenciaNoAgotaLaPila(t *testing.T) {
	r := lectorSintetico(t, []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /Yo 3 0 R >>",
	})
	ctx := SignContext{PDFReader: r}
	page := r.Page(1).V
	conPlazoSign(t, func() {
		defer func() {
			if p := recover(); p == nil || !strings.Contains(fmt.Sprint(p), "sí mismo") {
				t.Errorf("se esperaba un error controlado, no %v", p)
			}
		}()
		var b bytes.Buffer
		ctx.serializeCatalogEntry(&b, 3, page.Key("Yo"))
	})
}

// /Kids que repiten el mismo campo en cada nivel crecían de forma
// exponencial pese al límite de profundidad.
func TestBuscarCampoFirma_KidsRepetidos(t *testing.T) {
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] >> >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R >>",
	}
	const niveles = 30
	for i := 0; i < niveles; i++ {
		id := 4 + i
		objs = append(objs, fmt.Sprintf("<< /T (n%d) /Kids [%d 0 R %d 0 R] >>", i, id+1, id+1))
	}
	objs = append(objs, "<< /T (hoja) /FT /Tx >>")
	ctx := SignContext{PDFReader: lectorSintetico(t, objs)}
	conPlazoSign(t, func() {
		if _, err := ctx.buscarCampoFirma("Firma"); err == nil {
			t.Error("el campo no existe; se esperaba un error")
		}
	})
	// Un ciclo entre campos termina.
	ctx = SignContext{PDFReader: lectorSintetico(t, []string{
		"<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] >> >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R >>",
		"<< /T (a) /Kids [5 0 R] >>",
		"<< /T (b) /Kids [4 0 R] >>",
	})}
	conPlazoSign(t, func() {
		if _, err := ctx.buscarCampoFirma("Firma"); err == nil {
			t.Error("el campo no existe; se esperaba un error")
		}
	})
}

func TestPaginaCampoFirma(t *testing.T) {
	base := []string{
		"<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [5 0 R] >> >>",
		"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		"<< /Type /Page /Parent 2 0 R /N 1 >>",
		"<< /Type /Page /Parent 2 0 R /N 2 /Rotate 90 /Annots [5 0 R] >>",
		"<< /Type /Annot /Subtype /Widget /FT /Sig /T (Firma) /Rect [10 10 100 40] >>",
	}
	p, err := PaginaCampoFirma(lectorSintetico(t, base), "Firma")
	if err != nil || p.V.Key("N").Int64() != 2 {
		t.Fatalf("sin /P: página %v, %v", p.V.Key("N"), err)
	}
	conP := append([]string(nil), base...)
	conP[4] = "<< /Type /Annot /Subtype /Widget /FT /Sig /T (Firma) /Rect [10 10 100 40] /P 4 0 R >>"
	p, err = PaginaCampoFirma(lectorSintetico(t, conP), "Firma")
	if err != nil || p.V.Key("N").Int64() != 2 {
		t.Fatalf("con /P: página %v, %v", p.V.Key("N"), err)
	}
	if _, err := PaginaCampoFirma(lectorSintetico(t, base), "Otra"); err == nil {
		t.Fatal("un campo inexistente debe dar error")
	}
}
