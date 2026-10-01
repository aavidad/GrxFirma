// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

// loteFirmaSimulado firma todos los trabajos salvo los que contienen "falla".
type loteFirmaSimulado struct{ nombres []string }

func (m *loteFirmaSimulado) Execute(_ context.Context, cmd application.ProcessBatchCommand) (application.BatchResult, error) {
	res := application.BatchResult{Errores: map[int]error{}}
	for i, job := range cmd.Jobs {
		m.nombres = append(m.nombres, job.Document.Name)
		if strings.Contains(job.Document.Name, "falla") {
			res.Errores[i] = errors.New("documento no firmable")
			continue
		}
		res.Results = append(res.Results, application.SignResult{Result: domain.SignatureResult{
			Format: domain.FormatCAdES, Data: append([]byte("firma:"), job.Document.Content...),
		}})
	}
	return res, nil
}

func TestRunLoteCarpeta_GuardaCadaFirma(t *testing.T) {
	dir := t.TempDir()
	for nombre, contenido := range map[string]string{"a.txt": "A", "b-falla.txt": "B", "c.txt": "C", ".oculto": "X"} {
		if err := os.WriteFile(filepath.Join(dir, nombre), []byte(contenido), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "subcarpeta"), 0o700); err != nil {
		t.Fatal(err)
	}
	salida := t.TempDir()
	lote := &loteFirmaSimulado{}
	var stdout, stderr strings.Builder
	a := New(nil, lote)
	a.Stdout, a.Stderr = &stdout, &stderr
	rc := a.Run(context.Background(), []string{"-modo-cli", "-operacion", "firmar", "-formato", "cades", "-lote", dir, "-salida", salida})
	if rc != 0 {
		t.Fatalf("rc=%d stdout=%q stderr=%q", rc, stdout.String(), stderr.String())
	}
	sort.Strings(lote.nombres)
	if strings.Join(lote.nombres, ",") != "a.txt,b-falla.txt,c.txt" {
		t.Fatalf("documentos del lote: %v", lote.nombres)
	}
	for nombre, esperado := range map[string]string{"a.csig": "firma:A", "c.csig": "firma:C"} {
		data, err := os.ReadFile(filepath.Join(salida, nombre))
		if err != nil || string(data) != esperado {
			t.Fatalf("%s: %q %v; stdout=%q", nombre, data, err, stdout.String())
		}
	}
	if _, err := os.Stat(filepath.Join(salida, "b-falla.csig")); err == nil {
		t.Fatal("no debe guardarse la firma de un documento fallido")
	}
	if !strings.Contains(stdout.String(), "b-falla.txt") {
		t.Fatalf("el fallo debe indicarse por documento: %q", stdout.String())
	}
}
