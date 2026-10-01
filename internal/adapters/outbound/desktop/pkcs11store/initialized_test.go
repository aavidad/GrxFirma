// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Modela el slot de aprovisionamiento adicional de SoftHSM. El orden fijo
// acredita que un fallo posterior no convierte un catálogo parcial en éxito.
type provisionSlotModule struct {
	*fakeModule
	openAttempts int
	infoError    error
}

func (m *provisionSlotModule) Slots() ([]uint, error) { return []uint{1, 2}, nil }
func (m *provisionSlotModule) TokenInfo(slot uint) (tokenInfo, error) {
	if slot == 2 && m.infoError != nil {
		return tokenInfo{}, m.infoError
	}
	return m.fakeModule.TokenInfo(slot)
}
func (m *provisionSlotModule) OpenSession(slot uint) (sessionHandle, error) {
	m.openAttempts++
	if slot == 2 {
		return 0, ErrTokenUnavailable
	}
	return m.fakeModule.OpenSession(slot)
}

func TestListSkipsOnlyKnownUninitializedTokens(t *testing.T) {
	for _, tc := range []struct {
		name        string
		initialized bool
		infoError   error
		wantError   bool
		wantOpens   int
	}{
		{"provisioning_slot", false, nil, false, 1},
		{"initialized_token_failure", true, nil, true, 2},
		{"unknown_token_state", false, ErrTokenUnavailable, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := newFakeModule(t)
			base.tokens[2] = tokenInfo{initialized: tc.initialized}
			addIdentity(t, base, 1, 1, ecdsaKey(t), time.Now())
			m := &provisionSlotModule{fakeModule: base, infoError: tc.infoError}
			store := testStore(t, base, Options{})
			store.load = func(string) (module, error) { return m, nil }
			refs, err := store.List(context.Background())
			if tc.wantError {
				if !errors.Is(err, ErrTokenUnavailable) || len(refs) != 0 {
					t.Fatalf("fallo real ocultado: refs=%d err=%v", len(refs), err)
				}
			} else if err != nil || len(refs) != 1 {
				t.Fatalf("slot sin inicializar destruyó catálogo: refs=%d err=%v", len(refs), err)
			}
			if m.openAttempts != tc.wantOpens || base.logins != 0 || base.opens != base.closes {
				t.Fatalf("sesiones/PIN: attempts=%d opens=%d closes=%d logins=%d", m.openAttempts, base.opens, base.closes, base.logins)
			}
		})
	}
}
