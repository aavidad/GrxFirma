// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ports_test

import (
	"errors"
	"testing"

	"grxfirma/internal/ports"
)

type signingKeyWithoutClose struct{}

func (signingKeyWithoutClose) KeyID() string                 { return "without-close" }
func (signingKeyWithoutClose) CertificateChainDER() [][]byte { return nil }

type signingKeyClose struct {
	closed *int
	panic  bool
}

func (*signingKeyClose) KeyID() string                 { return "close" }
func (*signingKeyClose) CertificateChainDER() [][]byte { return nil }
func (key *signingKeyClose) Close() {
	(*key.closed)++
	if key.panic {
		panic("fallo simulado al cerrar")
	}
}

type signingKeyCloseError struct {
	closed *int
}

func (*signingKeyCloseError) KeyID() string                 { return "close-error" }
func (*signingKeyCloseError) CertificateChainDER() [][]byte { return nil }
func (key *signingKeyCloseError) Close() error {
	(*key.closed)++
	return errors.New("fallo simulado al cerrar")
}

func TestCloseSigningKeyEsBestEffort(t *testing.T) {
	t.Parallel()

	var typedNil *signingKeyClose
	ports.CloseSigningKey(nil)
	ports.CloseSigningKey(typedNil)
	ports.CloseSigningKey(signingKeyWithoutClose{})

	closed := 0
	ports.CloseSigningKey(&signingKeyClose{closed: &closed})
	ports.CloseSigningKey(&signingKeyCloseError{closed: &closed})
	ports.CloseSigningKey(&signingKeyClose{closed: &closed, panic: true})
	if closed != 3 {
		t.Fatalf("cierres ejecutados = %d, want 3", closed)
	}
}
