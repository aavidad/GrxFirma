// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build darwin

package localtlstrust

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func ensureTrustedPlatform(ctx context.Context, _ string, validatedCertFile string, cert *x509.Certificate) error {
	keychain := loginKeychain()
	instalado, err := certificadoInstaladoMacOS(ctx, keychain, cert)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return ErrHerramientaNoDisponible
		}
		return err
	}
	if instalado {
		return nil
	}

	cmd := exec.CommandContext(
		ctx,
		"security",
		"add-trusted-cert",
		"-d",
		"-r",
		"trustAsRoot",
		"-k",
		keychain,
		validatedCertFile,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return ErrHerramientaNoDisponible
		}
		return fmt.Errorf("localtlstrust: instalar en keychain: %w: %s", err, string(bytes.TrimSpace(out)))
	}
	return nil
}

func certificadoInstaladoMacOS(ctx context.Context, keychain string, cert *x509.Certificate) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "security", "find-certificate", "-a", "-p", keychain)
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("localtlstrust: find-certificate: %w", err)
	}
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
		instalado, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		if fingerprintSHA256(instalado) == fingerprintSHA256(cert) && certVigente(instalado, time.Now()) {
			return true, nil
		}
	}
	return false, nil
}

func loginKeychain() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "login.keychain-db"
	}
	return filepath.Join(home, "Library", "Keychains", "login.keychain-db")
}
