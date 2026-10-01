// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package wincertstore_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"os"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/wincertstore"
	"grxfirma/internal/domain"
)

// TestList_Windows_DevuelveSliceValido verifica que List() no falla y devuelve
// un slice (puede estar vacío en CI sin certificados instalados).
func TestList_Windows_DevuelveSliceValido(t *testing.T) {
	t.Parallel()

	almacen := wincertstore.New()
	refs, err := almacen.List(context.Background())
	if err != nil {
		t.Fatalf("List() error inesperado = %v", err)
	}
	// En un entorno de CI sin certificados el slice puede estar vacío; eso es válido.
	t.Logf("certificados encontrados en MY store: %d", len(refs))

	for i, ref := range refs {
		if ref.ID == "" {
			t.Errorf("refs[%d].ID está vacío", i)
		}
		if ref.Fingerprint == "" {
			t.Errorf("refs[%d].Fingerprint está vacío", i)
		}
		if ref.Subject == "" {
			t.Errorf("refs[%d].Subject está vacío", i)
		}
	}
}

// TestList_Windows_ContextoCancelado verifica que la cancelación de contexto
// se respeta y no produce panic.
func TestList_Windows_ContextoCancelado(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelar antes de llamar

	almacen := wincertstore.New()
	_, err := almacen.List(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("List() error = %v, want context.Canceled", err)
	}
}

func TestKeyFor_Windows_ContextoCancelado(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := wincertstore.New().KeyFor(ctx, domain.CertificateRef{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("KeyFor() error = %v, want context.Canceled", err)
	}
}

func TestKeyFor_Windows_CertificadoInexistente(t *testing.T) {
	t.Parallel()

	_, err := wincertstore.New().KeyFor(context.Background(), domain.CertificateRef{
		Fingerprint: strings.Repeat("00", sha256.Size),
	})
	if err == nil {
		t.Fatal("KeyFor() debía fallar para un certificado inexistente")
	}
}

// Esta prueba usa deliberadamente una identidad ya aprovisionada en el
// almacén del usuario. Se activa en una máquina Windows de validación con
// GRXFIRMA_TEST_WINDOWS_CERT_FINGERPRINT=<sha256> y puede mostrar el diálogo
// de PIN del proveedor o de la tarjeta criptográfica.
func TestKeyFor_Windows_FirmaReal(t *testing.T) {
	fingerprint := strings.TrimSpace(os.Getenv("GRXFIRMA_TEST_WINDOWS_CERT_FINGERPRINT"))
	if fingerprint == "" {
		t.Skip("define GRXFIRMA_TEST_WINDOWS_CERT_FINGERPRINT para probar una firma real")
	}

	almacen := wincertstore.New()
	clave, err := almacen.KeyFor(context.Background(), domain.CertificateRef{Fingerprint: fingerprint})
	if err != nil {
		t.Fatalf("KeyFor() error = %v", err)
	}
	cadena := clave.CertificateChainDER()
	if len(cadena) == 0 {
		t.Fatal("CertificateChainDER() no contiene el certificado firmante")
	}
	certificado, err := x509.ParseCertificate(cadena[0])
	if err != nil {
		t.Fatalf("ParseCertificate() error = %v", err)
	}

	firmante, ok := clave.(interface {
		SignDigest([]byte, crypto.Hash) ([]byte, error)
	})
	if !ok {
		t.Fatalf("la clave %T no permite firma de digest", clave)
	}
	digest := sha256.Sum256([]byte("grxfirma-windows-certstore-integration"))
	firma, err := firmante.SignDigest(digest[:], crypto.SHA256)
	if err != nil {
		t.Fatalf("SignDigest() error = %v", err)
	}

	switch publica := certificado.PublicKey.(type) {
	case *rsa.PublicKey:
		if err := rsa.VerifyPKCS1v15(publica, crypto.SHA256, digest[:], firma); err != nil {
			t.Fatalf("VerifyPKCS1v15() error = %v", err)
		}
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(publica, digest[:], firma) {
			t.Fatal("VerifyASN1() rechazó la firma ECDSA")
		}
	default:
		t.Fatalf("tipo de clave pública inesperado: %T", publica)
	}
}
