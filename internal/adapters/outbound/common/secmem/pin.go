// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package secmem

import (
	"errors"
	"unicode/utf8"
)

// ErrPINVacio se retorna cuando se intenta crear un PIN vacío.
var ErrPINVacio = errors.New("secmem: PIN no puede estar vacío")

// PIN almacena un PIN/contraseña sensible de forma segura.
// Internamente usa []byte para facilitar la zerorización; nunca almacena la
// contraseña como string (los strings de Go son inmutables y no zeorizables).
type PIN struct {
	blob *Blob
}

// NuevoPIN crea un PIN a partir de un slice de bytes.
// El contenido de src se copia; el caller puede zerizar src inmediatamente.
func NuevoPIN(src []byte) (*PIN, error) {
	if len(src) == 0 {
		return nil, ErrPINVacio
	}
	return &PIN{blob: New(src)}, nil
}

// NuevoPINDesdeString crea un PIN a partir de un string.
// El string se convierte a []byte y se copia; el original no se modifica.
// ATENCIÓN: los strings de Go no son zeorizables por el GC; usar esta
// función solo cuando el string ya proviene de input externo ineludible.
func NuevoPINDesdeString(s string) (*PIN, error) {
	if s == "" {
		return nil, ErrPINVacio
	}
	if !utf8.ValidString(s) {
		return nil, errors.New("secmem: PIN contiene bytes UTF-8 inválidos")
	}
	b := []byte(s)
	p, err := NuevoPIN(b)
	Zeroize(b)
	return p, err
}

// Bytes retorna una vista temporal del PIN.
// Solo válido mientras no se haya llamado a Destroy().
func (p *PIN) Bytes() []byte {
	if p.blob == nil {
		return nil
	}
	return p.blob.Bytes()
}

// Len retorna la longitud del PIN en bytes.
func (p *PIN) Len() int {
	if p.blob == nil {
		return 0
	}
	return p.blob.Len()
}

// Destroy zeriza y libera el PIN. Seguro llamarlo más de una vez.
func (p *PIN) Destroy() {
	if p.blob != nil {
		p.blob.Destroy()
		p.blob = nil
	}
}

// Use ejecuta fn con el contenido del PIN y zeriza una copia temporal después.
// Garantiza que la copia no persiste más allá de fn, incluso si entra en pánico.
func (p *PIN) Use(fn func([]byte)) {
	if p.blob == nil {
		return
	}
	WithBlob(p.blob.Bytes(), fn)
}
