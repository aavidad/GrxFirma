// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
)

// pdfObjetosPrueba escribe un PDF con los objetos indicados; el primero es
// el catálogo.
func pdfObjetosPrueba(objs []string) []byte {
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

// arbolesMaliciosos son árboles de páginas que antes colgaban la firma,
// agotaban la memoria o la pila.
func arbolesMaliciosos() map[string][]string {
	profundo := []string{"<< /Type /Catalog /Pages 2 0 R >>"}
	for i := 0; i < 10000; i++ {
		profundo = append(profundo, fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R] /Count 1 >>", 3+i))
	}
	profundo = append(profundo, "<< /Type /Page /MediaBox [0 0 595 842] /Resources << >> >>")
	return map[string][]string{
		"kids-ciclico": {
			"<< /Type /Catalog /Pages 2 0 R >>",
			"<< /Type /Pages /Kids [3 0 R] /Count 5 >>",
			"<< /Type /Pages /Parent 2 0 R /Kids [2 0 R] /Count 5 >>",
		},
		"profundidad-10000": profundo,
		"count-enorme": {
			"<< /Type /Catalog /Pages 2 0 R >>",
			"<< /Type /Pages /Kids [3 0 R] /Count 2000000000 >>",
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << >> >>",
		},
		"parent-ciclico": {
			"<< /Type /Catalog /Pages 2 0 R >>",
			"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			"<< /Type /Page /Parent 4 0 R /Resources << >> >>",
			"<< /Type /Pages /Parent 5 0 R >>",
			"<< /Type /Pages /Parent 4 0 R >>",
		},
	}
}

// Cada modo de sello recorre el árbol de páginas de una forma distinta:
// página única, todas, leyenda CSV en cada página, imagen de AutoFirma Java
// en todas las páginas y el campo de firma existente.
func TestMotorFirmaGo_PAdESArbolDePaginasMaliciosoNoCuelga(t *testing.T) {
	img := base64.StdEncoding.EncodeToString(pngDosColores(t))
	modos := map[string]map[string]string{
		"sello-pagina-1": {"visibleSeal": "true", "page": "1"},
		"sello-todas":    {"visibleSeal": "true", "page": "all"},
		"leyenda-csv":    {"csv": "ABCD-1234", "csvUrl": "https://sede.example.es/cotejo?csv={csv}"},
		"imagen-todas": {"image": img, "imagePage": "0",
			"imagePositionOnPageLowerLeftX": "10", "imagePositionOnPageLowerLeftY": "10",
			"imagePositionOnPageUpperRightX": "50", "imagePositionOnPageUpperRightY": "30"},
		"campo-firma": {"signatureField": "Firma"},
	}
	// Combinaciones que deben fallar: el árbol no permite situar la estampa.
	debeFallar := map[string]bool{
		"kids-ciclico/sello-pagina-1": true, "kids-ciclico/sello-todas": true,
		"profundidad-10000/sello-pagina-1": true, "profundidad-10000/sello-todas": true,
		"count-enorme/sello-todas": true, "count-enorme/leyenda-csv": true, "count-enorme/imagen-todas": true,
		"parent-ciclico/sello-pagina-1": true, "parent-ciclico/sello-todas": true,
	}
	priv, cert := generarCertRSA(t)
	for nombre, objs := range arbolesMaliciosos() {
		pdfData := pdfObjetosPrueba(objs)
		for modo, opciones := range modos {
			caso := nombre + "/" + modo
			t.Run(caso, func(t *testing.T) {
				doc, _ := domain.NewDocument("malicioso.pdf", pdfData, "application/pdf")
				hecho := make(chan error, 1)
				go func() {
					defer func() {
						if p := recover(); p != nil {
							hecho <- fmt.Errorf("pánico: %v", p)
						}
					}()
					_, err := signer.NuevoMotorFirmaGo(nil).Sign(context.Background(), domain.SignatureJob{
						Document: doc, Format: domain.FormatPAdES, Action: domain.ActionSign, Options: opciones,
					}, signer.NuevaClaveLocal(priv, cert))
					hecho <- err
				}()
				plazo := 20 * time.Second
				if d, ok := t.Deadline(); ok && time.Until(d) < plazo {
					plazo = time.Until(d) / 2
				}
				select {
				case err := <-hecho:
					if debeFallar[caso] && err == nil {
						t.Fatal("la firma debía rechazar el árbol de páginas malicioso")
					}
				case <-time.After(plazo):
					t.Fatal("la firma no terminó: bucle sin límite ante un árbol de páginas malicioso")
				}
			})
		}
	}
}
