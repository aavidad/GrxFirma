// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package localtlstrust

import (
	"context"
	"crypto/x509"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const windowsCertStoreIntegrationEnv = "GRXFIRMA_WINDOWS_CERT_STORE_INTEGRATION"

type fakeWindowsTrustStore struct {
	*fakeManagedCertificateStore
	closed bool
}

func (store *fakeWindowsTrustStore) Close() { store.closed = true }

func TestWindowsTrustStoreInyectado(t *testing.T) {
	cert := newManagedLocalCATestCertificate(t, 1002, true)
	store := &fakeWindowsTrustStore{fakeManagedCertificateStore: newFakeManagedCertificateStore()}
	open := func() (windowsTrustStore, error) { store.closed = false; return store, nil }
	certFile := filepath.Join(t.TempDir(), "websocket-localhost-root.crt.pem")
	ctx := context.Background()
	if err := ensureManagedTrustedWithWindowsStore(ctx, certFile, cert, open); err != nil {
		t.Fatal(err)
	}
	if !store.closed || len(store.certificates) != 1 {
		t.Fatal("el alta gestionada no cerró el almacén inyectado o no añadió la CA")
	}
	if err := removeManagedTrustedWithWindowsStore(ctx, certFile, open); err != nil {
		t.Fatal(err)
	}
	if !store.closed || len(store.certificates) != 0 {
		t.Fatal("la baja gestionada no cerró el almacén inyectado o dejó la CA")
	}
	if err := ensureTrustedWithWindowsStore(ctx, cert, open); err != nil {
		t.Fatal(err)
	}
	if !store.closed || len(store.certificates) != 1 {
		t.Fatal("el alta simple no usó el almacén inyectado")
	}
}

func TestWindowsRootStore_AddRechazaCertificadoSobredimensionado(t *testing.T) {
	t.Parallel()

	store := &windowsRootStore{}
	_, err := store.Add(context.Background(), &x509.Certificate{
		Raw: make([]byte, maxTrustPEMFileBytes+1),
	})
	if err == nil || !strings.Contains(err.Error(), "certificado demasiado grande") {
		t.Fatalf("Add() error = %v, want certificado demasiado grande", err)
	}
}

func TestWindowsRootStore_AddContainsRemove(t *testing.T) {
	if os.Getenv(windowsCertStoreIntegrationEnv) != "1" {
		t.Skip("prueba de integración con CurrentUser/ROOT no habilitada")
	}

	ctx := context.Background()
	cert := newManagedLocalCATestCertificate(t, 1001, true)
	fingerprint := fingerprintSHA256(cert)
	store, err := openWindowsRootStore()
	if err != nil {
		t.Fatalf("openWindowsRootStore() error = %v", err)
	}
	defer store.Close()
	addedByTest := false
	defer func() {
		if !addedByTest {
			return
		}
		// El borrado sigue exigiendo huella exacta y el marcador de CA propia.
		if _, cleanupErr := store.Remove(ctx, fingerprint); cleanupErr != nil {
			t.Errorf("limpieza de la CA de prueba: %v", cleanupErr)
		}
	}()

	if present, containsErr := store.Contains(ctx, fingerprint); containsErr != nil {
		t.Fatalf("Contains() inicial error = %v", containsErr)
	} else if present {
		t.Fatal("la CA efímera ya existía antes de la prueba")
	}
	if added, addErr := store.Add(ctx, cert); addErr != nil {
		t.Fatalf("Add() error = %v", addErr)
	} else if !added {
		t.Fatal("Add() no atribuyó la CA efímera a la prueba")
	}
	addedByTest = true
	if present, containsErr := store.Contains(ctx, fingerprint); containsErr != nil {
		t.Fatalf("Contains() tras Add error = %v", containsErr)
	} else if !present {
		t.Fatal("la CA efímera no aparece tras Add")
	}
	if removed, removeErr := store.Remove(ctx, fingerprint); removeErr != nil {
		t.Fatalf("Remove() error = %v", removeErr)
	} else if !removed {
		t.Fatal("Remove() no retiró la CA efímera")
	}
	addedByTest = false
	if present, containsErr := store.Contains(ctx, fingerprint); containsErr != nil {
		t.Fatalf("Contains() final error = %v", containsErr)
	} else if present {
		t.Fatal("la CA efímera sigue presente tras Remove")
	}
}
