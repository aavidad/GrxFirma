// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package localtlstrust

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsRootStore struct {
	handle windows.Handle
}

func ensureTrustedPlatform(ctx context.Context, _, _ string, cert *x509.Certificate) error {
	store, err := openWindowsRootStore()
	if err != nil {
		return err
	}
	defer store.Close()

	present, err := store.Contains(ctx, fingerprintSHA256(cert))
	if err != nil {
		return err
	}
	if present {
		return nil
	}
	if _, err := store.Add(ctx, cert); err != nil {
		return fmt.Errorf("localtlstrust: instalar certificado en CurrentUser/ROOT: %w", err)
	}
	return nil
}

func ensureManagedTrustedPlatform(
	ctx context.Context,
	certFile string,
	_ string,
	cert *x509.Certificate,
) error {
	store, err := openWindowsRootStore()
	if err != nil {
		return err
	}
	defer store.Close()
	if err := finishPendingManagedTrustRemoval(ctx, certFile, store); err != nil {
		return err
	}
	return reconcileManagedTrust(ctx, certFile, cert, store)
}

func removeManagedTrustedPlatform(ctx context.Context, certFile string) error {
	store, err := openWindowsRootStore()
	if err != nil {
		return err
	}
	defer store.Close()
	return removeManagedTrust(ctx, certFile, store)
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

func openWindowsRootStore() (*windowsRootStore, error) {
	storeName, err := windows.UTF16PtrFromString(managedTrustStore)
	if err != nil {
		return nil, fmt.Errorf("localtlstrust: UTF16 ROOT: %w", err)
	}
	// Se abre el almacén de sistema ROOT de CurrentUser por la vía normal.
	// Abrirlo como registro «no protegido» (SYSTEM_REGISTRY +
	// UNPROTECTED_FLAG) escribía la CA sin pasar por la lista de raíces
	// protegidas: Windows no confiaba en ella y el navegador no podía conectar
	// con el canal local (SEC_E_UNTRUSTED_ROOT). Por esta vía, Windows pide al
	// usuario una única confirmación al añadir o retirar la CA local. La
	// colección lógica también expone raíces de LocalMachine; las bajas solo
	// afectan a certificados con el marcador de propiedad de GrxFirma.
	handle, err := windows.CertOpenStore(
		windows.CERT_STORE_PROV_SYSTEM_W,
		0,
		0,
		windows.CERT_SYSTEM_STORE_CURRENT_USER,
		uintptr(unsafe.Pointer(storeName)),
	)
	if err != nil {
		return nil, fmt.Errorf("localtlstrust: abrir CurrentUser/ROOT: %w", err)
	}
	return &windowsRootStore{handle: handle}, nil
}

func (store *windowsRootStore) Close() {
	if store == nil || store.handle == 0 {
		return
	}
	_ = windows.CertCloseStore(store.handle, 0)
	store.handle = 0
}

func (store *windowsRootStore) Contains(ctx context.Context, fingerprint string) (bool, error) {
	certContext, err := store.find(ctx, fingerprint, false)
	if err != nil {
		return false, err
	}
	if certContext == nil {
		return false, nil
	}
	_ = windows.CertFreeCertificateContext(certContext)
	return true, nil
}

func (store *windowsRootStore) Add(ctx context.Context, cert *x509.Certificate) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if cert == nil || len(cert.Raw) == 0 {
		return false, fmt.Errorf("certificado vacío")
	}
	if len(cert.Raw) > maxTrustPEMFileBytes {
		return false, fmt.Errorf(
			"certificado demasiado grande: %d bytes (máximo %d)",
			len(cert.Raw),
			maxTrustPEMFileBytes,
		)
	}
	rawLength := uint32(len(cert.Raw)) // #nosec G115 -- limitado explícitamente a 1 MiB
	certContext, err := windows.CertCreateCertificateContext(
		windows.X509_ASN_ENCODING|windows.PKCS_7_ASN_ENCODING,
		&cert.Raw[0],
		rawLength,
	)
	if err != nil {
		return false, fmt.Errorf("CertCreateCertificateContext: %w", err)
	}
	defer windows.CertFreeCertificateContext(certContext)

	err = windows.CertAddCertificateContextToStore(
		store.handle,
		certContext,
		windows.CERT_STORE_ADD_NEW,
		nil,
	)
	if err == nil {
		return true, nil
	}
	// Una inserción concurrente del mismo DER no nos concede propiedad.
	// Se vuelve a enumerar y, si ya existe, se trata como preexistente.
	present, lookupErr := store.Contains(ctx, fingerprintSHA256(cert))
	if lookupErr == nil && present {
		return false, nil
	}
	return false, fmt.Errorf("CertAddCertificateContextToStore: %w", err)
}

func (store *windowsRootStore) Remove(ctx context.Context, fingerprint string) (bool, error) {
	certContext, err := store.find(ctx, fingerprint, true)
	if err != nil {
		return false, err
	}
	if certContext == nil {
		return false, nil
	}
	// CertDeleteCertificateFromStore libera siempre el contexto recibido.
	if err := windows.CertDeleteCertificateFromStore(certContext); err != nil {
		return false, fmt.Errorf("CertDeleteCertificateFromStore: %w", err)
	}
	return true, nil
}

func (store *windowsRootStore) find(
	ctx context.Context,
	fingerprint string,
	requireManagedMarker bool,
) (*windows.CertContext, error) {
	if store == nil || store.handle == 0 {
		return nil, fmt.Errorf("almacén CurrentUser/ROOT cerrado")
	}
	var previous *windows.CertContext
	for {
		if err := ctx.Err(); err != nil {
			if previous != nil {
				_ = windows.CertFreeCertificateContext(previous)
			}
			return nil, err
		}
		current, _ := windows.CertEnumCertificatesInStore(store.handle, previous)
		// La enumeración libera previous incluso al alcanzar el final.
		previous = nil
		if current == nil {
			return nil, nil
		}
		der := unsafe.Slice(current.EncodedCert, current.Length)
		raw := append([]byte(nil), der...)
		cert, err := x509.ParseCertificate(raw)
		if err == nil &&
			fingerprintSHA256(cert) == fingerprint &&
			(!requireManagedMarker || IsManagedLocalCA(cert)) {
			return current, nil
		}
		previous = current
	}
}
