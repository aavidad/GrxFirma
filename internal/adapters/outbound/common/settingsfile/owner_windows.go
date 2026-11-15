// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package settingsfile

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// maxOwnerDepth limita el recorrido hacia la raíz; una ruta de perfil real
// tiene muy pocos niveles.
const maxOwnerDepth = 32

// checkSettingsUser comprueba, antes de tocar permisos, que la carpeta de
// ajustes es del usuario del proceso. Con UAC el proceso elevado tiene el
// mismo usuario que el normal, así que pasa; un proceso elevado de otra
// cuenta (otro administrador, SYSTEM) que apunte a la carpeta de otra persona
// no. Sin elevar no se comprueba: el proceso solo llega aquí si el dueño de la
// carpeta ya le dio permiso para cambiarla, y así sigue funcionando lo que ya
// funcionaba.
func checkSettingsUser(dir *os.File, path string) error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return nil
	}
	user, err := processUser()
	if err != nil {
		return err
	}
	var owners []ownerClass
	sd, err := windows.GetSecurityInfo(windows.Handle(dir.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return &Error{Key: KeyOtherUser, Err: err}
	}
	owners = append(owners, classifyOwner(owner, user))
	current := path
	for range maxOwnerDepth {
		if owners[len(owners)-1] != ownerNotPerson {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
		sd, err := windows.GetNamedSecurityInfo(current, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
		if err != nil {
			break // Sin permiso para leer el propietario: no hay más datos.
		}
		owner, _, err := sd.Owner()
		if err != nil || owner == nil {
			break
		}
		owners = append(owners, classifyOwner(owner, user))
	}
	return settingsOwnerDecision(owners)
}

// claimSettingsDirectory hace al usuario del proceso propietario de la carpeta
// de ajustes cuando la creó un proceso elevado (propietario Administradores).
// checkSettingsUser ya descartó que sea de otra persona. Es una mejora: la
// DACL ya da acceso al usuario, así que un fallo aquí no impide guardar. Se
// reabre con WRITE_OWNER para no pedir ese derecho en OpenDir, y solo se toca
// si es la misma carpeta que ya está abierta.
func claimSettingsDirectory(dir *os.File, path string) {
	var opened windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(windows.Handle(dir.Fd()), &opened) != nil {
		return
	}
	user, err := processUser()
	if err != nil {
		return
	}
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	handle, err := windows.CreateFile(pathUTF16, windows.READ_CONTROL|windows.WRITE_OWNER,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(handle, &info) != nil ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 ||
		info.VolumeSerialNumber != opened.VolumeSerialNumber ||
		info.FileIndexHigh != opened.FileIndexHigh || info.FileIndexLow != opened.FileIndexLow {
		return
	}
	sd, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return
	}
	if owner, _, err := sd.Owner(); err != nil || owner == nil || owner.Equals(user) {
		return
	}
	_ = windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION, user, nil, nil, nil)
}

func processUser() (*windows.SID, error) {
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	return tokenUser.User.Sid.Copy()
}

// classifyOwner distingue al usuario del proceso, a otra persona y a los
// grupos o cuentas del sistema. Solo las cuentas locales, de dominio o de
// Microsoft Entra (S-1-5-21-…, S-1-12-1-…) pueden ser una persona; para ellas
// se pregunta a Windows el tipo y, si no responde, el propietario es dudoso.
func classifyOwner(owner, user *windows.SID) ownerClass {
	if owner.Equals(user) {
		return ownerCurrent
	}
	text := owner.String()
	if !strings.HasPrefix(text, "S-1-5-21-") && !strings.HasPrefix(text, "S-1-12-1-") {
		return ownerNotPerson
	}
	_, _, kind, err := owner.LookupAccount("")
	if err != nil {
		return ownerUnknown
	}
	if kind == windows.SidTypeUser {
		return ownerOtherUser
	}
	return ownerNotPerson
}
