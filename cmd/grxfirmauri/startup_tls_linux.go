// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package main

import (
	"context"
	"path/filepath"

	resttls "grxfirma/internal/adapters/inbound/common/rest"
	"grxfirma/internal/adapters/outbound/desktop/localtlstrust"
)

func ensureStartupLocalTLSTrustPlatform(ctx context.Context, configDir string) (bool, error) {
	var changed bool
	err := localtlstrust.WithLocalTLSStartupLock(ctx, configDir, func() error {
		certDir := filepath.Join(configDir, "tls")
		_, _, rootCertFile, _, err := resttls.EnsureBrowserCompatibleLocalhostCertificate(certDir, "websocket-localhost")
		if err != nil {
			return err
		}
		changed, err = localtlstrust.EnsureManagedTrustedWithResult(ctx, rootCertFile)
		return err
	})
	return changed, err
}
