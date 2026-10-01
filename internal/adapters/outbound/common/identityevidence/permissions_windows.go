// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package identityevidence

import (
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
	"grxfirma/internal/adapters/outbound/common/securefile"
)

func atributosPrivadosRegistro(directory bool) (*windows.SecurityAttributes, error) {
	token, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	flags := ""
	if directory {
		flags = "OICI"
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;" + flags + ";FA;;;" + token.User.Sid.String() + ")(A;" + flags + ";FA;;;SY)(A;" + flags + ";FA;;;BA)")
	if err != nil {
		return nil, err
	}
	return &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}, nil
}

func prepararDirectorioPrivado(path string) error {
	if _, err := os.Lstat(path); err == nil {
		// Nunca modificar la DACL de un padre preexistente (p. ej. HOME).
		return comprobarPermisosPrivados(path, true)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	attrs, err := atributosPrivadosRegistro(true)
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	// La DACL privada se aplica en la creación atómica, no por nombre después.
	if err := windows.CreateDirectory(name, attrs); err != nil {
		return err
	}
	return comprobarPermisosPrivados(path, true)
}

func abrirFicheroRegistro(path string, crear bool) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	mode := uint32(windows.OPEN_EXISTING)
	var attrs *windows.SecurityAttributes
	if crear {
		mode = windows.CREATE_NEW
		attrs, err = atributosPrivadosRegistro(false)
		if err != nil {
			return nil, err
		}
	}
	// Sin WRITE_DAC ni truncado: un archivo existente sólo se valida. Mantener
	// el handle sin SHARE_WRITE/DELETE evita reemplazo/escritura durante append.
	h, err := windows.CreateFile(name, windows.FILE_APPEND_DATA|windows.READ_CONTROL,
		windows.FILE_SHARE_READ, attrs, mode,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil ||
		info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		_ = windows.CloseHandle(h)
		return nil, ErrConfiguracionInvalida
	}
	if err := validarDACLRegistro(h); err != nil {
		_ = windows.CloseHandle(h)
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}

func comprobarPermisosPrivados(path string, directory bool) error {
	open := securefile.OpenRead
	if directory {
		open = securefile.OpenDir
	}
	f, err := open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return validarDACLRegistro(windows.Handle(f.Fd()))
}

func validarDACLRegistro(handle windows.Handle) error {
	sd, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	token, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	user := token.User.Sid.String()
	allowed := map[string]bool{user: true, "S-1-5-18": true, "S-1-5-32-544": true}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !allowed[owner.String()] {
		return ErrConfiguracionInvalida
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return ErrConfiguracionInvalida
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return ErrConfiguracionInvalida
	}
	currentEffective := false
	all := windows.ACCESS_MASK(windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff)
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return err
		}
		if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return ErrConfiguracionInvalida
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
		if sid == "S-1-3-4" { // OWNER RIGHTS sólo si el propietario real es privado.
			sid = owner.String()
		}
		if !allowed[sid] || (ace.Mask != windows.GENERIC_ALL && ace.Mask&all != all) {
			return ErrConfiguracionInvalida
		}
		if sid == user && ace.Header.AceFlags&windows.INHERIT_ONLY_ACE == 0 {
			currentEffective = true
		}
	}
	if !currentEffective {
		return ErrConfiguracionInvalida
	}
	return nil
}
