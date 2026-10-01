// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import (
	"bytes"
	"context"
	"crypto"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"grxfirma/internal/adapters/outbound/common/secmem"
)

func TestPINReplyBufferRequiresLockedOwner(t *testing.T) {
	for _, mode := range []string{"failure", "nil", "wrong size", "owner plus error"} {
		t.Run(mode, func(t *testing.T) {
			var owner *secmem.Blob
			var view []byte
			allocate := func(size int) (*secmem.Blob, error) {
				if size != MaxPINBytes+1 {
					t.Fatal("unbounded allocation")
				}
				switch mode {
				case "failure":
					return nil, errors.New("private OS diagnostic")
				case "nil":
					return nil, nil
				case "wrong size":
					owner = secmem.New([]byte("synthetic"))
				case "owner plus error":
					owner = secmem.New(make([]byte, size))
				}
				view = owner.Bytes()
				if mode == "owner plus error" {
					return owner, errors.New("private OS diagnostic")
				}
				return owner, nil
			}
			buffer, release, err := newPINReplyBuffer(false, allocate)
			if !errors.Is(err, ErrPINMemoryUnavailable) || buffer != nil || release != nil {
				t.Fatal("invalid allocation accepted or raw error exposed")
			}
			if owner != nil && (owner.Len() != 0 || owner.Locked() || !bytes.Equal(view, make([]byte, len(view)))) {
				t.Fatal("failed allocation owner not destroyed")
			}
		})
	}
}

func TestPINReplyBufferOwnsLockedPagesUntilRelease(t *testing.T) {
	var owner *secmem.Blob
	buffer, release, err := newPINReplyBuffer(false, func(size int) (*secmem.Blob, error) {
		var err error
		owner, err = secmem.NewLockedSize(size)
		return owner, err
	})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if !owner.Locked() || len(buffer) != MaxPINBytes+1 || cap(buffer) != len(buffer) {
		t.Fatal("incorrect locked reservation")
	}
	buffer[len(buffer)-1] = 1
	release()
	if owner.Locked() || owner.Len() != 0 || !bytes.Equal(buffer, make([]byte, len(buffer))) {
		t.Fatal("release retained PIN or page lock")
	}
}

func TestPINReplyHeaderCheckedBeforeSecretRead(t *testing.T) {
	for _, tc := range []struct {
		name     string
		kind     byte
		size     uint32
		capacity int
	}{
		{"wrong kind", FrameRequest, 2, MaxPINBytes + 1},
		{"empty", FramePINReply, 0, MaxPINBytes + 1},
		{"over limit", FramePINReply, MaxPINBytes + 2, MaxPINBytes + 1},
		{"overflow", FramePINReply, ^uint32(0), MaxPINBytes + 1},
		{"pinpad secret", FramePINReply, 2, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var header [5]byte
			header[0] = tc.kind
			binary.BigEndian.PutUint32(header[1:], tc.size)
			input := bytes.NewReader(append(header[:], []byte("QA only unread payload")...))
			buffer := bytes.Repeat([]byte{42}, tc.capacity)
			reply, err := readPINReplyInto(input, buffer)
			if !errors.Is(err, ErrProtocol) || reply != nil || input.Len() != len("QA only unread payload") {
				t.Fatal("invalid header read payload")
			}
			if !bytes.Equal(buffer, make([]byte, len(buffer))) {
				t.Fatal("error retained input memory")
			}
		})
	}
}

func TestPINReplyClearsPartialReadsAndWrites(t *testing.T) {
	for _, data := range [][]byte{nil, {FramePINReply}, {FramePINReply, 0, 0, 0, 4, 1, 'Q'}} {
		buffer := bytes.Repeat([]byte{42}, MaxPINBytes+1)
		if reply, err := readPINReplyInto(bytes.NewReader(data), buffer); err == nil || reply != nil {
			t.Fatal("accepted incomplete frame")
		}
		if !bytes.Equal(buffer, make([]byte, len(buffer))) {
			t.Fatal("partial PIN retained")
		}
	}
	for _, writer := range []io.Writer{io.Discard, stalledWriter{}} {
		buffer := bytes.Repeat([]byte{42}, MaxPINBytes+1)
		_ = writePINReplyInto(writer, []byte("QA-only"), false, buffer)
		if !bytes.Equal(buffer, make([]byte, len(buffer))) {
			t.Fatal("transport buffer retained after write")
		}
	}
	var input bytes.Buffer
	_ = WriteFrame(&input, FramePINReply, []byte{1, 'Q', 0, 255}, MaxPINBytes+1)
	buffer := make([]byte, MaxPINBytes+1)
	reply, err := readPINReplyInto(&input, buffer)
	if err != nil || !bytes.Equal(reply, []byte{1, 'Q', 0, 255}) || cap(reply) != len(reply) || &reply[0] != &buffer[0] {
		t.Fatal("reply did not borrow supplied region")
	}
	clear(buffer)
}

func TestServerReservesPINBeforeChallengeAndPreservesPinpad(t *testing.T) {
	for _, protected := range []bool{false, true} {
		t.Run(map[bool]string{false: "PIN", true: "pinpad"}[protected], func(t *testing.T) {
			r, backend, key := fixture(t)
			allocations := 0
			server := Server{pinBuffer: func(int) (*secmem.Blob, error) {
				allocations++
				return nil, errors.New("private allocator error")
			}, Factory: func(_ string, source PINSource) (Backend, error) {
				key.sign = func(_ []byte, _ crypto.SignerOpts) ([]byte, error) {
					pin, err := source(context.Background(), PINMode{ProtectedAuthenticationPath: protected})
					if len(pin) != 0 {
						t.Fatal("unexpected PIN")
					}
					if err != nil {
						return nil, err
					}
					return []byte{1, 2, 3}, nil
				}
				return backend, nil
			}}
			response, challenges, _ := execute(t, server, r, []byte{1})
			if protected {
				if allocations != 0 || response.Code != "ok" || len(challenges) != 1 {
					t.Fatal("pinpad depends on mlock")
				}
			} else if allocations != 1 || response.Code != "pin_memory_unavailable" || len(challenges) != 0 {
				t.Fatal("PIN challenge sent without locked reservation")
			}
			if !backend.closed || !key.closed {
				t.Fatal("native resources not closed")
			}
		})
	}
}

func TestServerPreservesMemoryFailureAfterBackendNormalization(t *testing.T) {
	for _, ignoreFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "normalized to cancellation", true: "ignored by driver"}[ignoreFailure], func(t *testing.T) {
			r, backend, key := fixture(t)
			server := Server{
				pinBuffer: func(int) (*secmem.Blob, error) { return nil, secmem.ErrLockUnavailable },
				Factory: func(_ string, source PINSource) (Backend, error) {
					key.sign = func([]byte, crypto.SignerOpts) ([]byte, error) {
						_, err := source(context.Background(), PINMode{})
						if !errors.Is(err, ErrPINMemoryUnavailable) {
							t.Fatal("missing strict failure")
						}
						if ignoreFailure {
							return []byte{1, 2, 3}, nil
						}
						return nil, ErrPINCancelled // same normalization as native adapter
					}
					return backend, nil
				},
			}
			response, challenges, _ := execute(t, server, r)
			if response.Code != "pin_memory_unavailable" || len(response.Signature) != 0 || len(challenges) != 0 {
				t.Fatal("backend hid local memory failure")
			}
		})
	}
}
