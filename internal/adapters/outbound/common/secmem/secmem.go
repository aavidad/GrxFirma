// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package secmem proporciona utilidades para manejar material criptográfico sensible
// en memoria de forma segura, zerorizando el contenido cuando ya no es necesario.
//
// Principios (ADR-002):
//   - El material sensible (claves privadas, PINs, claves de sesión) nunca persiste
//     más tiempo del estrictamente necesario.
//   - Al liberar, el contenido se sobreescribe con ceros para dificultar la
//     recuperación del material desde volcados de memoria.
//   - La API es explícita: el caller decide cuándo destruir el material.
package secmem

import (
	"os"
	"runtime"
	"unsafe"
)

// Blob contiene bytes sensibles que serán zerorizados al llamar a Destroy().
// Crear siempre con New o NewLockedSize; no copiar el struct (los slices y el
// estado de fijado pertenecen a una única instancia).
type Blob struct {
	data, backing, region []byte
	locked                bool
	pinner                runtime.Pinner
	unlock                func([]byte)
}

// New crea un Blob copiando src. El original src no se modifica.
// Las páginas del buffer se fijan en RAM (mlock/VirtualLock) de forma
// best-effort para que el material no acabe en swap; si el sistema lo
// deniega (RLIMIT_MEMLOCK, contenedores) el Blob funciona igual sin fijar.
func New(src []byte) *Blob {
	b, err := newBlob(len(src), false, memoryOperations{lockMemory, unlockMemory})
	if err != nil {
		// Igual que make, New no puede representar un tamaño imposible.
		panic(err)
	}
	copy(b.data, src)
	return b
}

// NewLockedSize reserva bytes inicialmente cero y bloquea sus páginas ANTES
// de que el llamador escriba secretos. No degrada a memoria sin fijar. Un tamaño
// cero devuelve un Blob vacío, sin páginas que bloquear y con Locked()==false.
// El límite estricto acota la presión sobre RLIMIT_MEMLOCK; no se aplica a New.
func NewLockedSize(size int) (*Blob, error) {
	return newBlob(size, true, memoryOperations{lockMemory, unlockMemory})
}

// Operaciones por instancia: los tests no reemplazan estado global compartido.
type memoryOperations struct {
	lock   func([]byte) bool
	unlock func([]byte)
}

func newBlob(size int, strict bool, operations memoryOperations) (*Blob, error) {
	if size < 0 || (strict && size > MaxLockedSize) {
		return nil, &SizeError{Size: size}
	}
	if size == 0 {
		return &Blob{}, nil
	}
	page := os.Getpagesize()
	regionSize, backingSize, err := alignedSizes(size, page)
	if err != nil {
		return nil, err
	}
	b := &Blob{backing: make([]byte, backingSize), unlock: operations.unlock}
	// Mantener ubicación fija es distinto de fijar páginas físicas con mlock.
	// Pin antes de calcular la dirección; nunca fabricar un puntero desde uintptr.
	b.pinner.Pin(&b.backing[0])
	address := uintptr(unsafe.Pointer(&b.backing[0]))
	offset := (page - int(address%uintptr(page))) % page
	b.region = b.backing[offset : offset+regionSize : offset+regionSize]
	b.data = b.region[:size:size]
	b.locked = operations.lock(b.region)
	runtime.KeepAlive(b.backing)
	if strict && !b.locked {
		b.Destroy()
		return nil, &LockError{Size: size}
	}
	runtime.SetFinalizer(b, func(b *Blob) { b.Destroy() })
	return b, nil
}

// Bytes presta una vista mutable con capacidad limitada a Len(). El llamador
// debe mantener vivo al Blob hasta terminar de usarla (normalmente defer
// Destroy; si procede runtime.KeepAlive). No usarla concurrentemente con
// Destroy. Las vistas anteriores quedan a cero tras Destroy, sin desmapearse.
func (b *Blob) Bytes() []byte {
	return b.data
}

// Len retorna la longitud del material almacenado.
func (b *Blob) Len() int { return len(b.data) }

// Locked indica si las páginas del Blob quedaron fijadas en RAM.
// Diagnóstico: false no es un error (el fijado es best-effort).
func (b *Blob) Locked() bool { return b.locked }

// Destroy sobreescribe con ceros y libera la memoria.
// Seguro llamarlo más de una vez.
func (b *Blob) Destroy() {
	clear(b.region)
	if b.locked {
		b.unlock(b.region)
		b.locked = false
	}
	runtime.KeepAlive(b.backing)
	b.pinner.Unpin()
	b.data = nil
	b.region = nil
	b.backing = nil
	b.unlock = nil
	runtime.SetFinalizer(b, nil)
}

// Zeroize sobreescribe con ceros el slice dado en el sitio.
// Útil para zeroizar buffers temporales que no son Blob.
func Zeroize(buf []byte) {
	for i := range buf {
		buf[i] = 0
	}
}

// UseReadOnlyString presta temporalmente src como string sin crear una copia
// inmutable y borra el buffer al terminar, también si fn entra en pánico. Está
// destinado exclusivamente a APIs criptográficas heredadas que todavía exigen
// string. fn no debe conservar la vista después de retornar.
func UseReadOnlyString(src []byte, fn func(string) error) error {
	defer Zeroize(src)
	if len(src) == 0 {
		return fn("")
	}
	view := unsafe.String(unsafe.SliceData(src), len(src))
	err := fn(view)
	runtime.KeepAlive(src)
	return err
}

// WithBlob ejecuta fn con el material desprotegido y zeroiza inmediatamente
// después, incluso si fn entra en pánico.
func WithBlob(src []byte, fn func([]byte)) {
	b := New(src)
	defer b.Destroy()
	fn(b.Bytes())
}
