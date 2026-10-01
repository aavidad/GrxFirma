// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemporaryCredentialReadsOnlyBoundedRegularP12Files(t *testing.T) {
	dir := t.TempDir()
	for _, test := range []struct {
		name  string
		data  []byte
		valid bool
	}{
		{"certificate.PFX", []byte("synthetic fixture"), true},
		{"certificate.p12", []byte("synthetic fixture"), true},
		{"certificate.pem", []byte("synthetic fixture"), false},
		{"empty.p12", nil, false},
		{"oversize.p12", make([]byte, maxTemporaryCredentialBytes+1), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(dir, test.name)
			if err := os.WriteFile(path, test.data, 0600); err != nil {
				t.Fatal(err)
			}
			got, err := readTemporaryCredential(path)
			defer clear(got)
			if test.valid {
				if err != nil || !bytes.Equal(got, test.data) {
					t.Fatalf("lectura: %v", err)
				}
			} else if err == nil || len(got) != 0 {
				t.Fatal("debe rechazar y no devolver bytes")
			}
			if err != nil && strings.Contains(err.Error(), dir) {
				t.Fatal("el error expone la ruta")
			}
		})
	}
	if _, err := readTemporaryCredential(filepath.Join(dir, "missing.p12")); err == nil {
		t.Fatal("aceptó inexistente")
	}
	folder := filepath.Join(dir, "folder.p12")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := readTemporaryCredential(folder); err == nil {
		t.Fatal("aceptó directorio")
	}
	link := filepath.Join(dir, "link.p12")
	if err := os.Symlink(filepath.Join(dir, "certificate.p12"), link); err != nil {
		t.Skip("symlinks no disponibles")
	}
	if _, err := readTemporaryCredential(link); err == nil {
		t.Fatal("aceptó enlace simbólico")
	}
}
