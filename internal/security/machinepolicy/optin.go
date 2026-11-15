// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package machinepolicy

import "sync"

// valorPrueba es una excepción fijada en una prueba; fijado=false simula que
// el administrador no ha puesto la directiva.
type valorPrueba struct {
	valor, fijado bool
	err           error
}

var (
	overridesMu sync.RWMutex
	overrides   = map[string]valorPrueba{}
)

// OptIn indica si la política de máquina habilita expresamente una excepción
// de seguridad (compatibilidad heredada). Solo un administrador puede
// fijarla: registro HKLM en Windows o /etc/grxfirma/policy.json en el
// resto. Las variables de entorno ya no bastan: cualquier web o guía podía
// pedir al usuario que las definiera para rebajar la seguridad.
func OptIn(name string) bool {
	habilitado, fijado := OptInFijado(name)
	return fijado && habilitado
}

// OptInFijado distingue una excepción que el administrador ha fijado
// expresamente (habilitada o no) de una que no ha tocado. Sirve para las
// directivas en las que «deshabilitada» cierra algo que sin la directiva se
// admite por compatibilidad (por ejemplo, permitir_sha1_legacy a 0 o false
// cierra el ENI de una firma SHA-1 antigua). Si la política no se puede leer,
// devuelve false y fijado=true para cerrar las habilitaciones. Para
// restricciones negativas debe usarse ReadOptIn y comprobar el error.
func OptInFijado(name string) (habilitado, fijado bool) {
	habilitado, fijado, err := ReadOptIn(name)
	if err != nil {
		return false, true
	}
	return habilitado, fijado
}

// ReadOptIn distingue ausencia de una directiva ilegible. Los consumidores
// de restricciones negativas deben tratar el error como una prohibición.
func ReadOptIn(name string) (bool, bool, error) {
	overridesMu.RLock()
	v, ok := overrides[name]
	overridesMu.RUnlock()
	if ok {
		return v.valor, v.fijado, v.err
	}
	return readOptIn(name)
}

// ErrorForTest simula una directiva ilegible sin acceder a la del equipo.
func ErrorForTest(tb testHandle, name string, err error) {
	tb.Helper()
	fijarParaPrueba(tb, name, valorPrueba{err: err})
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
	fijarParaPrueba(tb, name, valorPrueba{valor: value, fijado: true})
}

// UnsetForTest simula durante un test que la directiva no está fijada.
func UnsetForTest(tb testHandle, name string) {
	tb.Helper()
	fijarParaPrueba(tb, name, valorPrueba{})
}

func fijarParaPrueba(tb testHandle, name string, value valorPrueba) {
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
