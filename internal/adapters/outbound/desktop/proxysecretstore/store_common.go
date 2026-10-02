// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package proxysecretstore

import (
	"errors"
	"strings"
	"unicode"
)

const windowsBackendName = "dpapi-user"

const maxProxySecretIDBytes = 128

var ErrInvalidSecretID = errors.New("proxysecretstore: id invalido")

// normalizeSecretID limita los identificadores opacos a un subconjunto que es
// seguro tanto como atributo de Secret Service/Keychain como al formar la ruta
// del blob DPAPI en Windows. Los identificadores generados por randomID son
// hexadecimales y cumplen este contrato.
func normalizeSecretID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > maxProxySecretIDBytes {
		return "", ErrInvalidSecretID
	}
	for _, r := range id {
		if r <= unicode.MaxASCII &&
			(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.') {
			continue
		}
		return "", ErrInvalidSecretID
	}
	if id == "." || id == ".." {
		return "", ErrInvalidSecretID
	}
	base := strings.ToUpper(strings.SplitN(id, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" ||
		(len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) &&
			base[3] >= '1' && base[3] <= '9') {
		return "", ErrInvalidSecretID
	}
	return id, nil
}

func zeroSecretBytes(secret []byte) {
	for i := range secret {
		secret[i] = 0
	}
}
