// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package tokenpin

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/secmem"
	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
)

type fragmentedPINReader struct {
	io.Reader
	fragment int
}

func (r fragmentedPINReader) Read(p []byte) (int, error) {
	if len(p) > r.fragment {
		p = p[:r.fragment]
	}
	return r.Reader.Read(p)
}

func TestAssuanLockedOwnershipAndCleanup(t *testing.T) {
	for _, tc := range []struct {
		name, wire, want string
		err              error
	}{
		{"success", "OK\nD a%25%00%0A%C3%B1\nD z\nOK\nOK\n", "a%\x00\nñz", nil},
		{"maximum", "OK\nD " + strings.Repeat("%31", pkcs11worker.MaxPINBytes) + "\nOK\nOK\n", strings.Repeat("1", pkcs11worker.MaxPINBytes), nil},
		{"decode_error", "OK\nD 1234%XX\nOK\n", "", ErrProtocol},
		{"cancel", "OK\nD 1234\nERR 99 synthetic\n", "", pkcs11worker.ErrPINCancelled},
		{"bye_error", "OK\nD 1234\nOK\nERR 1 synthetic\n", "", ErrFailed},
		{"trailing_data", "OK\nD 1234\nOK\nOK\nsecret", "", ErrProtocol},
	} {
		for _, fragment := range []int{1, 7, receiveBufferSize} {
			t.Run(tc.name+"/"+strconv.Itoa(fragment), func(t *testing.T) {
				var owner *secmem.Blob
				var memory []byte
				r, err := newProtocolReader(fragmentedPINReader{strings.NewReader(tc.wire), fragment}, false, func(size int) (*secmem.Blob, error) {
					var err error
					owner, err = secmem.NewLockedSize(size)
					if owner != nil {
						memory = owner.Bytes()
					}
					return owner, err
				})
				if err != nil {
					t.Fatal(err)
				}
				defer r.close()
				if len(memory) != pinMemorySize || cap(r.buffer) != receiveBufferSize || cap(r.line) != maxLine || cap(r.pin) != pkcs11worker.MaxPINBytes {
					t.Fatal("unexpected owned region bounds")
				}
				var commands bytes.Buffer
				pin, err := r.exchange(&commands, nil, false)
				if !errors.Is(err, tc.err) || string(pin) != tc.want {
					t.Fatalf("unexpected result: %v, length=%d", err, len(pin))
				}
				if !owner.Locked() {
					t.Fatal("released memory before consumer completed")
				}
				if len(pin) > 0 && &pin[0] != &r.pin[0] {
					t.Fatal("PIN decoded outside owned region")
				}
				r.close()
				if owner.Locked() || !bytes.Equal(memory, make([]byte, len(memory))) || !bytes.Equal(pin, make([]byte, len(pin))) {
					t.Fatal("owned memory or borrowed PIN not erased")
				}
			})
		}
	}
}

func TestAssuanAllocationFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name     string
		allocate pinMemoryAllocator
	}{
		{"failure", func(int) (*secmem.Blob, error) { return nil, secmem.ErrLockUnavailable }},
		{"nil", func(int) (*secmem.Blob, error) { return nil, nil }},
		{"unlocked", func(int) (*secmem.Blob, error) { return secmem.New(nil), nil }},
		{"wrong_size", func(int) (*secmem.Blob, error) { return secmem.NewLockedSize(1) }},
		{"owner_and_error", func(size int) (*secmem.Blob, error) { return secmem.New(make([]byte, size)), secmem.ErrLockUnavailable }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var owner *secmem.Blob
			var memory []byte
			r, err := newProtocolReader(nil, false, func(size int) (*secmem.Blob, error) {
				var err error
				owner, err = tc.allocate(size)
				if owner != nil {
					memory = owner.Bytes()
				}
				return owner, err
			})
			if r != nil || !errors.Is(err, pkcs11worker.ErrPINMemoryUnavailable) {
				t.Fatalf("allocation failure not propagated: %v", err)
			}
			if owner != nil && (owner.Locked() || !bytes.Equal(memory, make([]byte, len(memory)))) {
				t.Fatal("failed allocation not destroyed")
			}
		})
	}
}

func TestAssuanProtectedDoesNotRequireLockedMemory(t *testing.T) {
	for _, wire := range []string{"OK\nOK\nOK\n", "OK\nD 1234\nOK\nOK\n"} {
		r, err := newProtocolReader(strings.NewReader(wire), true, func(int) (*secmem.Blob, error) {
			t.Fatal("protected authentication attempted memory locking")
			return nil, secmem.ErrLockUnavailable
		})
		if err != nil {
			t.Fatal(err)
		}
		defer r.close()
		var commands bytes.Buffer
		pin, err := r.exchange(&commands, nil, true)
		if pin != nil || r.owner != nil || r.pin != nil || strings.Contains(commands.String(), "GETPIN") {
			t.Fatal("protected authentication transported PIN")
		}
		if strings.Contains(wire, "D ") && !errors.Is(err, ErrProtocol) || !strings.Contains(wire, "D ") && err != nil {
			t.Fatalf("protected response: %v", err)
		}
	}
}
