// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package machinepolicy

import "sync"

var (
	overridesMu sync.RWMutex
	overrides   = map[string]bool{}
)

// OptIn indica si la política de máquina habilita expresamente una excepción
// de seguridad (compatibilidad heredada). Solo un administrador puede
// fijarla: registro HKLM en Windows o /etc/grxfirma/policy.json en el
// resto. Las variables de entorno ya no bastan: cualquier web o guía podía
// pedir al usuario que las definiera para rebajar la seguridad.
func OptIn(name string) bool {
	overridesMu.RLock()
	v, ok := overrides[name]
	overridesMu.RUnlock()
	if ok {
		return v
	}
	enabled, present, err := readOptIn(name)
	return err == nil && present && enabled
}

// testHandle es el subconjunto de testing.TB necesario; evita enlazar el
// paquete testing en los binarios de producción.
type testHandle interface {
	Helper()
	Cleanup(func())
}

// SetForTest fija temporalmente una excepción de política durante un test.
func SetForTest(tb testHandle, name string, value bool) {
	tb.Helper()
	overridesMu.Lock()
	previous, existed := overrides[name]
	overrides[name] = value
	overridesMu.Unlock()
	tb.Cleanup(func() {
		overridesMu.Lock()
		defer overridesMu.Unlock()
		if existed {
			overrides[name] = previous
		} else {
			delete(overrides, name)
		}
	})
}
