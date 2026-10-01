// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package secmem

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"runtime"
	"testing"
	"unsafe"
)

func TestLockedBlobAlignedAndBounded(t *testing.T) {
	page := os.Getpagesize()
	for _, size := range []int{1, page - 1, page, page + 1, MaxLockedSize} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			t.Parallel()
			var lockedRegion []byte
			unlocks := 0
			b, err := newBlob(size, true, memoryOperations{
				lock: func(region []byte) bool {
					lockedRegion = region
					if uintptr(unsafe.Pointer(&region[0]))%uintptr(page) != 0 || len(region)%page != 0 {
						t.Fatal("bloqueo fuera de páginas enteras alineadas")
					}
					if !bytes.Equal(region, make([]byte, len(region))) {
						t.Fatal("secreto escrito antes de bloquear")
					}
					return true
				},
				unlock: func(region []byte) {
					unlocks++
					if &region[0] != &lockedRegion[0] || len(region) != len(lockedRegion) {
						t.Fatal("desbloqueo fuera de la región propia")
					}
					if !bytes.Equal(region, make([]byte, len(region))) {
						t.Fatal("región desbloqueada antes de borrarla completamente")
					}
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer b.Destroy()
			base := uintptr(unsafe.Pointer(&b.backing[0]))
			start := uintptr(unsafe.Pointer(&b.region[0]))
			if start < base || start+uintptr(len(b.region)) > base+uintptr(len(b.backing)) {
				t.Fatal("región no contenida en backing propio")
			}
			if b.Len() != size || len(b.Bytes()) != size || cap(b.Bytes()) != size || !b.Locked() {
				t.Fatal("vista no acotada al tamaño solicitado")
			}
			view := b.Bytes()
			for i := range b.region {
				b.region[i] = 0xa5 // comprobar también el padding no expuesto
			}
			b.Destroy()
			b.Destroy()
			if unlocks != 1 || b.Locked() || b.Len() != 0 || b.Bytes() != nil || b.backing != nil || b.region != nil {
				t.Fatal("destrucción no definitiva/idempotente")
			}
			if !bytes.Equal(view, make([]byte, len(view))) {
				t.Fatal("la vista anterior no conserva ceros")
			}
		})
	}
}

func TestDestroyDoesNotUnlockOtherBlobPages(t *testing.T) {
	t.Parallel()
	page := uintptr(os.Getpagesize())
	locked := map[uintptr]bool{}
	ops := memoryOperations{
		lock: func(region []byte) bool {
			start := uintptr(unsafe.Pointer(&region[0]))
			for p := start; p < start+uintptr(len(region)); p += page {
				if locked[p] {
					t.Fatal("dos Blobs comparten página bloqueada")
				}
				locked[p] = true
			}
			return true
		},
		unlock: func(region []byte) {
			start := uintptr(unsafe.Pointer(&region[0]))
			for p := start; p < start+uintptr(len(region)); p += page {
				if !locked[p] {
					t.Fatal("desbloqueo duplicado o de página ajena")
				}
				delete(locked, p)
			}
		},
	}
	a, err := newBlob(1, true, ops)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Destroy()
	b, err := newBlob(int(page)+1, true, ops)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Destroy()
	b.Bytes()[0] = 42
	a.Destroy()
	if len(locked) != 2 || !b.Locked() || b.Bytes()[0] != 42 {
		t.Fatal("destruir A cambió las páginas o datos de B")
	}
	b.Destroy()
	if len(locked) != 0 {
		t.Fatal("páginas sin liberar")
	}
}

func TestLockFailureStrictVersusLegacy(t *testing.T) {
	t.Parallel()
	ops := memoryOperations{lock: func([]byte) bool { return false }, unlock: func([]byte) { t.Fatal("unlock sin lock") }}
	b, err := newBlob(16, true, ops)
	var typed *LockError
	if b != nil || !errors.Is(err, ErrLockUnavailable) || !errors.As(err, &typed) || typed.Size != 16 {
		t.Fatalf("reserva estricta degradada: blob=%v error=%v", b != nil, err)
	}
	b, err = newBlob(16, false, ops)
	if err != nil || b == nil || b.Locked() {
		t.Fatal("legacy no conserva best-effort")
	}
	view := b.Bytes()
	copy(view, "sintetico")
	b.Destroy()
	if !bytes.Equal(view, make([]byte, len(view))) {
		t.Fatal("legacy sin mlock no borró memoria")
	}
}

func TestLockedSizeValidationAndOverflow(t *testing.T) {
	t.Parallel()
	maxInt := int(^uint(0) >> 1)
	for _, size := range []int{-1, MaxLockedSize + 1, maxInt} {
		b, err := NewLockedSize(size)
		var typed *SizeError
		if b != nil || !errors.Is(err, ErrInvalidSize) || !errors.As(err, &typed) || typed.Size != size {
			t.Fatalf("tamaño %d: %v", size, err)
		}
	}
	for _, tc := range [][2]int{{maxInt, 4096}, {maxInt - 2, 3}, {1, maxInt}, {1, 0}, {0, 4096}} {
		if _, _, err := alignedSizes(tc[0], tc[1]); !errors.Is(err, ErrInvalidSize) {
			t.Fatalf("overflow/tamaño no rechazado: %v", tc)
		}
	}
	b, err := NewLockedSize(0)
	if err != nil || b == nil || b.Len() != 0 || b.Locked() {
		t.Fatal("contrato de reserva vacía")
	}
	b.Destroy()
}

func TestBlobBackingAndOwnerRemainLiveDuringUse(t *testing.T) {
	t.Parallel()
	unlocks := 0
	b, err := newBlob(32, true, memoryOperations{lock: func([]byte) bool { return true }, unlock: func([]byte) { unlocks++ }})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Destroy() // el dueño sigue vivo hasta después de consumir la vista
	view := b.Bytes()
	copy(view, "QA-sintetica")
	address := unsafe.Pointer(&view[0])
	for i := 0; i < 3; i++ {
		runtime.GC()
		if unsafe.Pointer(&b.Bytes()[0]) != address || !bytes.HasPrefix(view, []byte("QA-sintetica")) || unlocks != 0 {
			t.Fatal("GC alteró memoria en uso")
		}
	}
	runtime.KeepAlive(b)
	b.Destroy()
	if unlocks != 1 || !bytes.Equal(view, make([]byte, len(view))) {
		t.Fatal("vista destruida no conserva ceros")
	}
}
