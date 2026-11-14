// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build darwin

package localtlstrust

import (
	"context"
	"crypto/sha1" // #nosec G505 -- identificador que usa `security`.
	"encoding/hex"
	"encoding/pem"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// llaveroFalso simula `security` sobre un llavero en memoria.
type llaveroFalso struct {
	pem     []byte
	ordenes [][]string
}

func (l *llaveroFalso) run(_ context.Context, args ...string) ([]byte, error) {
	l.ordenes = append(l.ordenes, append([]string(nil), args...))
	switch args[0] {
	case "find-certificate":
		return l.pem, nil
	case "add-trusted-cert":
		return nil, nil
	case "delete-certificate":
		l.pem = nil
		return nil, nil
	}
	return nil, nil
}

func TestDarwinManagedTrust_InventariaYRetira(t *testing.T) {
	ca, _ := caLocalDePrueba(t, 601, true)
	falso := &llaveroFalso{}
	store := &darwinKeychainStore{keychain: "/tmp/login.keychain-db", run: falso.run}
	certFile := filepath.Join(t.TempDir(), "websocket-localhost-root.crt.pem")

	if err := ensureManagedTrustedWithStore(context.Background(), certFile, ca, store); err != nil {
		t.Fatalf("instalar: %v", err)
	}
	var alta []string
	for _, orden := range falso.ordenes {
		if orden[0] == "add-trusted-cert" {
			alta = orden
		}
	}
	if !slices.Contains(alta, "-p") || alta[slices.Index(alta, "-p")+1] != "ssl" {
		t.Fatalf("la raíz debe confiarse solo para SSL: %v", alta)
	}
	if !ManagedTrustLifecycleSupported() {
		t.Fatal("macOS debe anunciar el ciclo completo de alta y retirada")
	}

	falso.pem = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})
	if err := removeManagedTrust(context.Background(), certFile, store); err != nil {
		t.Fatalf("retirar: %v", err)
	}
	sum := sha1.Sum(ca.Raw)
	want := strings.ToUpper(hex.EncodeToString(sum[:]))
	borrada := false
	for _, orden := range falso.ordenes {
		if orden[0] == "delete-certificate" && slices.Contains(orden, want) {
			borrada = true
		}
	}
	if !borrada {
		t.Fatalf("no se borró la CA inventariada: %v", falso.ordenes)
	}
}
