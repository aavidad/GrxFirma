//go:build regression_v1

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package regression_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixturesV1_SiguenSincronizadosConLaFuente(t *testing.T) {
	root := os.Getenv("GRXFIRMA_V1_SAMPLES")
	if root == "" {
		t.Skip("defina GRXFIRMA_V1_SAMPLES para validar la sincronizacion con la fuente V1")
	}
	if _, err := os.Stat(root); err != nil {
		t.Skipf("muestras V1 no disponibles en %s", root)
	}

	raw, err := os.ReadFile(filepath.Join("fixtures", "v1", "manifest.json"))
	if err != nil {
		t.Fatalf("no se pudo leer el manifest: %v", err)
	}

	var manifest fixtureManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("no se pudo parsear el manifest: %v", err)
	}

	for _, relative := range manifest.Fixtures {
		if strings.Contains(filepath.Base(relative), "_signed") || relative == "samples/2.pdf" {
			continue
		}
		localPath := filepath.Join("fixtures", "v1", relative)
		sourcePath := filepath.Join(root, filepath.Base(relative))

		localData, err := os.ReadFile(localPath)
		if err != nil {
			t.Fatalf("no se pudo leer el fixture local %s: %v", localPath, err)
		}
		sourceData, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Fatalf("no se pudo leer el fixture fuente %s: %v", sourcePath, err)
		}
		if !bytes.Equal(localData, sourceData) {
			t.Fatalf("el fixture %s ya no coincide con la fuente V1", relative)
		}
	}
}
