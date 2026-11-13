package sign

// Prueba de regresión de E2-06: las cadenas con caracteres no ASCII se leen
// igual que se escribieron y no alteran el diccionario que las contiene.

import (
	"strings"
	"testing"
)

// leerCadenaPDF mete la cadena en un diccionario y la lee con el lector de
// PDF, como haría quien abre el documento firmado.
func leerCadenaPDF(t *testing.T, cadena string) (string, []string) {
	t.Helper()
	r := lectorPrueba(t, pdfConObjetos(
		"<< /Type /Catalog /Pages 2 0 R /Motivo "+cadena+" /Despues /Fin >>",
		"<< /Type /Pages /Kids [] /Count 0 >>",
	))
	root := r.Trailer().Key("Root")
	return root.Key("Motivo").Text(), root.Keys()
}

func TestLimites_PDFStringUTF16SeLeeIgual(t *testing.T) {
	for _, texto := range []string{"ñ)", "小", "ĩ(", "小)/Inyectada (x", "Motivo con acentos: señal \\ fin"} {
		t.Run(texto, func(t *testing.T) {
			leido, claves := leerCadenaPDF(t, pdfString(texto))
			if leido != texto {
				t.Fatalf("se escribió %q y se leyó %q", texto, leido)
			}
			if strings.Join(claves, ",") != "Despues,Motivo,Pages,Type" {
				t.Fatalf("el texto alteró el diccionario: %v", claves)
			}
		})
	}
}
