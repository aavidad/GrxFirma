// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	pdf "github.com/digitorus/pdf"
)

func TestPAdESV2_DSSExigeDERPorCategoria(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "testdata", "dictamen-v2", "16_dss_valido", "firmado.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	valid, err := InspeccionarPAdESV2(data, 20, 20)
	if err != nil || valid.CambiosPosteriores.Estado != "permitidos" {
		t.Fatalf("DSS válido: %+v, %v", valid.CambiosPosteriores, err)
	}
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	dss := r.Trailer().Key("Root").Key("DSS")
	for _, category := range []string{"Certs", "CRLs", "OCSPs"} {
		t.Run(category, func(t *testing.T) {
			stream := dss.Key(category).Index(0)
			reader := stream.Reader()
			der, err := io.ReadAll(reader)
			_ = reader.Close()
			if err != nil {
				t.Fatal(err)
			}
			mutated := append([]byte(nil), data...)
			start := bytes.LastIndex(mutated, der)
			if start < 0 {
				t.Fatal("flujo DSS ausente")
			}
			mutated[start] = 0 // Misma longitud y xref: DER inválido.
			inspection, err := InspeccionarPAdESV2(mutated, 20, 20)
			if err != nil || inspection.CambiosPosteriores.Estado != "no_comprobados" {
				t.Fatalf("%s corrupto: %+v, %v", category, inspection.CambiosPosteriores, err)
			}
		})
	}
	count, total := maxDSSV2Streams, 0
	if checkDSSSingle(dss.Key("Certs").Index(0), "Certs", valid.Revisiones[len(valid.Revisiones)-1], map[uint32]bool{}, &count, &total) {
		t.Fatal("se aceptó un flujo por encima del límite de número")
	}
	count, total = 0, maxDSSV2TotalBytes
	if checkDSSSingle(dss.Key("Certs").Index(0), "Certs", valid.Revisiones[len(valid.Revisiones)-1], map[uint32]bool{}, &count, &total) {
		t.Fatal("se aceptó un flujo por encima del límite de bytes")
	}
	changed := pdf.ChangedXRefObjects(valid.Revisiones[len(valid.Revisiones)-2], valid.Revisiones[len(valid.Revisiones)-1])
	if comprobarDSSV2(data, valid.Revisiones[len(valid.Revisiones)-2], valid.Revisiones[len(valid.Revisiones)-1], append(changed, 4)) {
		t.Fatal("un objeto de contenido de página no puede clasificarse como DSS")
	}
}
