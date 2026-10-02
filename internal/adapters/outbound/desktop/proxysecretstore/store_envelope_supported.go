// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux || windows || (darwin && cgo)

package proxysecretstore

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

type secretEnvelope struct {
	Realm       string `json:"realm"`
	Username    string `json:"username"`
	PasswordB64 string `json:"password_b64"`
}

func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("proxysecretstore: generando id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
