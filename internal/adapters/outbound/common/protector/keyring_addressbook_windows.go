// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package protector

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"grxfirma/internal/domain"
)

func listWindowsAddressBook(ctx context.Context) ([]domain.ProtectionRecipient, error) {
	name, err := windows.UTF16PtrFromString("AddressBook")
	if err != nil {
		return nil, err
	}
	store, err := windows.CertOpenStore(
		windows.CERT_STORE_PROV_SYSTEM_W, 0, 0,
		windows.CERT_SYSTEM_STORE_CURRENT_USER|windows.CERT_STORE_READONLY_FLAG,
		uintptr(unsafe.Pointer(name)),
	)
	if err != nil {
		return nil, fmt.Errorf("abriendo Otras personas: %w", err)
	}
	defer windows.CertCloseStore(store, 0)
	out := make([]domain.ProtectionRecipient, 0)
	var previous *windows.CertContext
	defer func() {
		if previous != nil {
			_ = windows.CertFreeCertificateContext(previous)
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		current, err := windows.CertEnumCertificatesInStore(store, previous)
		previous = nil // CryptoAPI libera el contexto anterior.
		if err != nil {
			if errors.Is(err, syscall.Errno(0x80092004)) {
				return out, nil
			}
			return nil, fmt.Errorf("enumerando Otras personas: %w", err)
		}
		previous = current
		if current == nil || current.EncodedCert == nil || current.Length == 0 || current.Length > maxPublicCertificateBytes {
			continue
		}
		der := append([]byte(nil), unsafe.Slice(current.EncodedCert, int(current.Length))...)
		recipient, err := publicRecipient(der, "otras_personas")
		if err == nil {
			out = append(out, recipient)
		}
		if len(out) >= maxPublicRecipients {
			return out, nil
		}
	}
}
