// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package settingsfile

import (
	"errors"
	"testing"
)

func TestSettingsOwnerDecisionSoloEscribeLosAjustesDelUsuario(t *testing.T) {
	cases := []struct {
		name   string
		owners []ownerClass
		ok     bool
	}{
		{"carpeta del usuario", []ownerClass{ownerCurrent}, true},
		{"creada elevada dentro del perfil del usuario", []ownerClass{ownerNotPerson, ownerCurrent}, true},
		{"solo grupos y sistema hasta la raíz", []ownerClass{ownerNotPerson, ownerNotPerson, ownerNotPerson}, true},
		{"sin datos de propietario", nil, true},
		{"carpeta de otra persona", []ownerClass{ownerOtherUser}, false},
		{"creada elevada en el perfil de otra persona", []ownerClass{ownerNotPerson, ownerOtherUser, ownerCurrent}, false},
		{"cuenta que Windows no sabe resolver", []ownerClass{ownerUnknown}, false},
		{"manda el primero que es persona", []ownerClass{ownerCurrent, ownerOtherUser}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := settingsOwnerDecision(tc.owners)
			if tc.ok {
				if err != nil {
					t.Fatalf("se rechazó: %v", err)
				}
				return
			}
			var explained *Error
			if !errors.As(err, &explained) || explained.Key != KeyOtherUser {
				t.Fatalf("error = %v; se esperaba la clave %s", err, KeyOtherUser)
			}
		})
	}
}
