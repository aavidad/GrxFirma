// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
)

const (
	startupTLSInstalledID = "GrxFirma ha instalado su CA local para conectar de forma segura con los portales desde el navegador. Cierra Firefox por completo y vuelve a abrirlo antes de firmar."
	startupTLSFailedID    = "No se pudo instalar la CA local de GrxFirma en todos los navegadores. Cierra Firefox por completo y vuelve a abrir GrxFirma; si persiste, comprueba que certutil está instalado."
)

var ensureStartupLocalTLSTrust = ensureStartupLocalTLSTrustPlatform
var presentStartupTLSTrustFailure = presentStartupTLSTrustFailurePlatform

var startupTLSTrustNotice struct {
	mu   sync.RWMutex
	text string
}

func setStartupTLSTrustNotice(message string) {
	startupTLSTrustNotice.mu.Lock()
	startupTLSTrustNotice.text = message
	startupTLSTrustNotice.mu.Unlock()
}

func getStartupTLSTrustNotice() string {
	startupTLSTrustNotice.mu.RLock()
	defer startupTLSTrustNotice.mu.RUnlock()
	return startupTLSTrustNotice.text
}

func detailWithStartupTLSTrustNotice(detail string) string {
	notice := getStartupTLSTrustNotice()
	if notice == "" {
		return detail
	}
	if strings.TrimSpace(detail) == "" {
		return notice
	}
	return detail + "\n\n" + notice
}

// prepareStartupLocalTLSTrust runs as the actual user, before protocol
// forwarding or starting WSS. A later launch also discovers new Firefox
// profiles. The installer owns the decision whether anything changed.
func prepareStartupLocalTLSTrust(ctx context.Context, configDir string, stderr io.Writer) error {
	changed, err := ensureStartupLocalTLSTrust(ctx, configDir)
	if err != nil {
		message := tl(startupTLSFailedID)
		setStartupTLSTrustNotice(message)
		if stderr != nil {
			_, _ = fmt.Fprintf(stderr, "%s\n%s\n", message, tl("Detalle técnico: %s", err.Error()))
		}
		presentStartupTLSTrustFailure(message)
		return err
	}
	if changed {
		message := tl(startupTLSInstalledID)
		setStartupTLSTrustNotice(message)
		if stderr != nil {
			_, _ = fmt.Fprintln(stderr, message)
		}
	}
	return nil
}
