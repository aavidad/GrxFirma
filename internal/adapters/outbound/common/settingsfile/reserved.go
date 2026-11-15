// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package settingsfile

import (
	"encoding/json"
	"errors"
)

var ErrReservedAlias = errors.New("nombre reservado ambiguo en preferencias")

// PreserveReserved copies only the exact current exception settings to next.
// Generic writers cannot create or restore stale exception values; callers
// must invoke this while holding WithLock for the whole read/modify/write.
func PreserveReserved(current, next map[string]json.RawMessage) error {
	for key := range current {
		if Reserved(key) && key != UserField && key != AcceptanceField {
			return ErrReservedAlias
		}
	}
	for key := range next {
		if Reserved(key) && key != UserField && key != AcceptanceField {
			return ErrReservedAlias
		}
	}
	for _, key := range []string{UserField, AcceptanceField} {
		delete(next, key)
		if raw, ok := current[key]; ok {
			next[key] = append(json.RawMessage(nil), raw...)
		}
	}
	return nil
}
