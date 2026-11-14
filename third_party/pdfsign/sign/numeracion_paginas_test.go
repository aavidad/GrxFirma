package sign

import (
	"testing"
)

// KD-04: el lector (pdf.Reader.Page, con el que el motor calcula el giro y la
// caja del sello) y el firmador (findPageByNumber, que coloca el widget)
// tienen que dar la misma hoja para cada número de página, aunque un /Count
// intermedio mienta. Antes el lector bajaba por los /Count y el firmador
// recorría las hojas: con un nodo intermedio /Count 0 la «página 1» del
// lector era la hoja girada P2 y la del firmador, P1.
func TestNumeracionPaginasLectorYFirmadorCoinciden(t *testing.T) {
	r := lectorSintetico(t, []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>",
		"<< /Type /Pages /Parent 2 0 R /Kids [4 0 R] /Count 0 >>",
		"<< /Type /Page /Parent 3 0 R /MediaBox [0 0 595 842] /Rotate 0 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 842 595] /Rotate 90 >>",
	})
	paginas := r.Trailer().Key("Root").Key("Pages")
	for n := 1; n <= 2; n++ {
		lector := r.Page(n).V
		firmador, err := findPageByNumber(paginas, uint32(n))
		if err != nil {
			t.Fatalf("página %d: %v", n, err)
		}
		idLector, _ := lector.ObjectReference()
		idFirmador, _ := firmador.ObjectReference()
		if idLector != idFirmador {
			t.Fatalf("página %d: el lector da el objeto %d (/Rotate %d) y el firmador el %d (/Rotate %d)",
				n, idLector, lector.Key("Rotate").Int64(), idFirmador, firmador.Key("Rotate").Int64())
		}
	}
	if got := r.Page(1).V.Key("Rotate").Int64(); got != 0 {
		t.Fatalf("la primera hoja en orden es P1 sin giro; el lector da /Rotate %d", got)
	}
}

// Un árbol coherente sigue dando las mismas páginas que antes.
func TestNumeracionPaginasArbolCoherente(t *testing.T) {
	r := lectorSintetico(t, []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 6 0 R] /Count 3 >>",
		"<< /Type /Pages /Parent 2 0 R /Kids [4 0 R 5 0 R] /Count 2 >>",
		"<< /Type /Page /Parent 3 0 R /MediaBox [0 0 1 1] >>",
		"<< /Type /Page /Parent 3 0 R /MediaBox [0 0 2 2] >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 3 3] >>",
	})
	for n, esperado := range map[int]uint32{1: 4, 2: 5, 3: 6} {
		if id, _ := r.Page(n).V.ObjectReference(); id != esperado {
			t.Fatalf("página %d: objeto %d, esperado %d", n, id, esperado)
		}
	}
	if !r.Page(4).V.IsNull() || !r.Page(0).V.IsNull() {
		t.Fatal("una página fuera del documento debe ser nula")
	}
}
