// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build darwin

package localtlstrust

import (
	"bytes"
	"context"
	"crypto/sha1" // #nosec G505 -- `security delete-certificate -Z` identifica el certificado por su SHA-1; no se usa para seguridad.
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// En macOS la CA gestionada usa el mismo inventario de propiedad que Windows
// (B-07): se instala en el llavero de inicio de sesión confiada solo para SSL
// y se retira por las huellas inventariadas, nunca por nombre o sujeto.

type securityRunner func(ctx context.Context, args ...string) ([]byte, error)

func runSecurity(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// #nosec G204 -- `security` es la herramienta fija del sistema; los
	// argumentos se pasan por separado y nunca a un intérprete.
	out, err := exec.CommandContext(ctx, "security", args...).CombinedOutput()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return out, ErrHerramientaNoDisponible
		}
		return out, fmt.Errorf("security %s: %w: %s", args[0], err, string(bytes.TrimSpace(out)))
	}
	return out, nil
}

type darwinKeychainStore struct {
	keychain string
	run      securityRunner
}

func newDarwinKeychainStore() *darwinKeychainStore {
	return &darwinKeychainStore{keychain: loginKeychain(), run: runSecurity}
}

func (store *darwinKeychainStore) certificates(ctx context.Context) ([]*x509.Certificate, error) {
	out, err := store.run(ctx, "find-certificate", "-a", "-p", store.keychain)
	if err != nil {
		return nil, err
	}
	var certs []*x509.Certificate
	rest := out
	for len(rest) > 0 {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = next
		if block.Type != "CERTIFICATE" {
			continue
		}
		if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
			certs = append(certs, cert)
		}
	}
	return certs, nil
}

func (store *darwinKeychainStore) find(ctx context.Context, fingerprint string) (*x509.Certificate, error) {
	certs, err := store.certificates(ctx)
	if err != nil {
		return nil, err
	}
	for _, cert := range certs {
		if fingerprintSHA256(cert) == fingerprint {
			return cert, nil
		}
	}
	return nil, nil
}

func (store *darwinKeychainStore) Contains(ctx context.Context, fingerprint string) (bool, error) {
	cert, err := store.find(ctx, fingerprint)
	return cert != nil, err
}

func (store *darwinKeychainStore) Add(ctx context.Context, cert *x509.Certificate) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if cert == nil || len(cert.Raw) == 0 {
		return false, errors.New("certificado vacío")
	}
	path, cleanup, err := writeTemporaryCertificate(cert)
	if err != nil {
		return false, err
	}
	defer cleanup()
	if _, err := store.run(ctx, darwinAddTrustedArgs(store.keychain, path)...); err != nil {
		// Una inserción concurrente del mismo DER no concede propiedad.
		if present, lookupErr := store.Contains(ctx, fingerprintSHA256(cert)); lookupErr == nil && present {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (store *darwinKeychainStore) Remove(ctx context.Context, fingerprint string) (bool, error) {
	cert, err := store.find(ctx, fingerprint)
	if err != nil || cert == nil {
		return false, err
	}
	if !IsManagedLocalCAOwned(cert) {
		return false, errors.New("localtlstrust: la huella inventariada no es una CA de GrxFirma")
	}
	path, cleanup, err := writeTemporaryCertificate(cert)
	if err != nil {
		return false, err
	}
	defer cleanup()
	// Primero se retira la confianza y después el certificado. Si la
	// confianza ya no existía, `remove-trusted-cert` falla y se sigue.
	_, _ = store.run(ctx, "remove-trusted-cert", "-d", path)
	sum := sha1.Sum(cert.Raw) // #nosec G401 -- identificador que exige `security`.
	if _, err := store.run(ctx, "delete-certificate", "-Z", strings.ToUpper(hex.EncodeToString(sum[:])), store.keychain); err != nil {
		return false, err
	}
	return true, nil
}

func writeTemporaryCertificate(cert *x509.Certificate) (string, func(), error) {
	file, err := os.CreateTemp("", "grxfirma-trusted-cert-*.pem")
	if err != nil {
		return "", nil, fmt.Errorf("localtlstrust: crear certificado temporal: %w", err)
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
	if err := file.Chmod(0o600); err == nil {
		_, err = file.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err == nil {
			return path, cleanup, nil
		}
	} else {
		_ = file.Close()
	}
	cleanup()
	return "", nil, fmt.Errorf("localtlstrust: escribir certificado temporal: %w", err)
}

func ensureManagedTrustedPlatform(ctx context.Context, certFile string, _ string, cert *x509.Certificate) error {
	return ensureManagedTrustedWithStore(ctx, certFile, cert, newDarwinKeychainStore())
}

func ensureManagedTrustedWithStore(ctx context.Context, certFile string, cert *x509.Certificate, store managedCertificateStore) error {
	if err := finishPendingManagedTrustRemoval(ctx, certFile, store); err != nil {
		return err
	}
	return reconcileManagedTrust(ctx, certFile, cert, store)
}

func removeManagedTrustedPlatform(ctx context.Context, certFile string) error {
	return removeManagedTrust(ctx, certFile, newDarwinKeychainStore())
}

func managedTrustLifecycleSupportedPlatform() bool {
	return true
}

func managedTrustChangeTokenPlatform(certFile string) (string, error) {
	inv, _, err := loadManagedTrustInventory(managedTrustInventoryPath(certFile))
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(inv)
	return string(raw), err
}
