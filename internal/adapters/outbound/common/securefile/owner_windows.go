// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package securefile

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// currentUserSID devuelve el usuario del token del proceso. Con UAC, el token
// elevado y el filtrado de la misma sesión comparten este SID; lo que cambia
// es el propietario por defecto, que en el elevado es Administradores.
func currentUserSID() (*windows.SID, error) {
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	return tokenUser.User.Sid.Copy()
}

// privateFileSecurityAttributes describe un fichero nuevo del usuario: él es
// el propietario y la DACL protegida solo le da acceso a él, a SYSTEM y a
// Administradores. Se aplica en la propia creación, así un proceso elevado
// no deja ficheros con propietario Administradores que luego el mismo usuario
// sin elevar no puede abrir.
func privateFileSecurityAttributes() (*windows.SecurityAttributes, error) {
	user, err := currentUserSID()
	if err != nil {
		return nil, err
	}
	descriptor, err := windows.SecurityDescriptorFromString(fmt.Sprintf(
		"O:%[1]sD:P(A;;FA;;;%[1]s)(A;;FA;;;SY)(A;;FA;;;BA)", user.String(),
	))
	if err != nil {
		return nil, err
	}
	attributes := &windows.SecurityAttributes{SecurityDescriptor: descriptor}
	attributes.Length = uint32(unsafe.Sizeof(*attributes))
	return attributes, nil
}
