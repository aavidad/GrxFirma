// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package tokenpin requests one-time token authorization through a local
// pinentry dialog. It never starts gpg-agent or enables password caching.
package tokenpin

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/domain"
)

var (
	ErrUnavailable = pkcs11worker.ErrPINUnavailable
	ErrProtocol    = fmt.Errorf("%w: respuesta no válida", pkcs11worker.ErrPINPromptFailed)
	ErrFailed      = pkcs11worker.ErrPINPromptFailed
)

type Prompt struct{ locale string }

func New(locale string) *Prompt { return &Prompt{locale: locale} }

// Request transfers ownership of the returned mutable buffer to the caller,
// which must clear it. The reference must originate from the local catalog.
// Toolkit/OS memory is outside the Go buffer-erasure guarantee.
func (p *Prompt) Request(ctx context.Context, ref domain.CertificateRef, mode pkcs11worker.PINMode) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(ref.Fingerprint) != 64 {
		return nil, ErrProtocol
	}
	if _, err := hex.DecodeString(ref.Fingerprint); err != nil {
		return nil, ErrProtocol
	}
	return requestLocal(ctx, p.locale, strings.ToUpper(ref.Fingerprint), mode)
}

func dialogCommands(locale, fingerprint string, mode pkcs11worker.PINMode) []string {
	english := strings.HasPrefix(strings.ToLower(locale), "en")
	title, desc, ok, cancel := "GrxFirma · Autorizar firma", "Introduzca el PIN de su tarjeta o dispositivo para esta firma.", "Autorizar", "Cancelar"
	if english {
		title, desc, ok, cancel = "GrxFirma · Authorize signature", "Enter your card or device PIN for this signature.", "Authorize", "Cancel"
	}
	if mode.ContextSpecific {
		desc = "El dispositivo requiere una nueva autorización para esta firma. Introduzca su PIN."
		if english {
			desc = "The device requires a new authorization for this signature. Enter your PIN."
		}
	}
	if mode.ProtectedAuthenticationPath {
		desc = "El PIN se introducirá en el teclado del lector, no en el ordenador. Pulse Autorizar y siga las indicaciones del dispositivo."
		if english {
			desc = "Enter the PIN on the reader keypad, not on the computer. Select Authorize and follow the device instructions."
		}
	}
	// Only validated hex is interpolated: no subject, markup or text from a
	// browser/driver can impersonate the dialog instructions or Assuan commands.
	desc += "\nSHA-256: " + fingerprint
	return []string{"SETTITLE " + escapeText(title), "SETDESC " + escapeText(desc), "SETPROMPT PIN:", "SETOK " + ok, "SETCANCEL " + cancel, "SETTIMEOUT 90"}
}

func escapeText(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(s)
}
