// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"grxfirma/internal/adapters/outbound/common/secmem"
)

func TestClientReservesPINBeforePrompt(t *testing.T) {
	client := Client{pinBuffer: func(int) (*secmem.Blob, error) { return nil, errors.New("OS private text") },
		PINSource: func(context.Context, PINMode) ([]byte, error) { t.Fatal("prompt before lock"); return nil, nil }}
	var output bytes.Buffer
	if err := client.replyToPINChallenge(context.Background(), &output, PINChallenge{}); !errors.Is(err, ErrPINMemoryUnavailable) || output.Len() != 0 {
		t.Fatal("lock failure not propagated before prompt")
	}
}

func TestClientPINTransportOwnerLifetime(t *testing.T) {
	for _, failWrite := range []bool{false, true} {
		t.Run(fmt.Sprint(failWrite), func(t *testing.T) {
			var owner *secmem.Blob
			var view []byte
			pin := []byte("synthetic QA PIN")
			client := Client{pinBuffer: func(size int) (*secmem.Blob, error) {
				var err error
				owner, err = secmem.NewLockedSize(size)
				if err == nil {
					view = owner.Bytes()
				}
				return owner, err
			}, PINSource: func(context.Context, PINMode) ([]byte, error) {
				if owner == nil || !owner.Locked() {
					t.Error("unlocked prompt")
				}
				return pin, nil
			}}
			var out bytes.Buffer
			var writer io.Writer = &out
			if failWrite {
				writer = stalledWriter{}
			}
			err := client.replyToPINChallenge(context.Background(), writer, PINChallenge{})
			if failWrite && !errors.Is(err, ErrOperationFailed) || !failWrite && err != nil {
				t.Fatal(err)
			}
			if owner == nil || owner.Locked() || owner.Len() != 0 || !bytes.Equal(view, make([]byte, len(view))) || !bytes.Equal(pin, make([]byte, len(pin))) {
				t.Fatal("PIN owner or callback memory not released")
			}
		})
	}
}

func TestClientPinpadDoesNotAllocateSecretPages(t *testing.T) {
	client := Client{pinBuffer: func(int) (*secmem.Blob, error) { t.Fatal("pinpad allocated secret pages"); return nil, nil },
		PINSource: func(_ context.Context, mode PINMode) ([]byte, error) {
			if !mode.ProtectedAuthenticationPath {
				t.Fatal("lost protected path")
			}
			return nil, nil
		}}
	var output bytes.Buffer
	if err := client.replyToPINChallenge(context.Background(), &output, PINChallenge{ProtectedAuthenticationPath: true}); err != nil {
		t.Fatal(err)
	}
	kind, reply, err := ReadFrame(&output, MaxPINBytes+1)
	if err != nil || kind != FramePINReply || !bytes.Equal(reply, []byte{1}) {
		t.Fatal("wrong consent marker")
	}
}

func TestClientMapsRemoteMemoryFailure(t *testing.T) {
	client := fixtureClient(t, "memoryerror")
	_, err := client.Execute(context.Background(), clientSignRequest())
	if !errors.Is(err, ErrPINMemoryUnavailable) {
		t.Fatal(err)
	}
}
