// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package protector

import (
	"context"
	"grxfirma/internal/domain"
)

func listWindowsAddressBook(context.Context) ([]domain.ProtectionRecipient, error) { return nil, nil }
