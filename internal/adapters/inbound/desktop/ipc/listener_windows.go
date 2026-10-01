// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package ipc

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

const (
	prefijoNamedPipeWindows = `\\.\pipe\`
	bufferNamedPipeWindows  = 64 * 1024
)

func escucharIPC(socketPath string) (net.Listener, error) {
	if err := validarNamedPipeLocal(socketPath); err != nil {
		return nil, err
	}
	descriptor, err := descriptorSeguridadNamedPipe()
	if err != nil {
		return nil, err
	}

	// go-winio crea el pipe con FILE_PIPE_REJECT_REMOTE_CLIENTS. La DACL
	// protegida permite acceso unicamente a la sesion de logon.
	return winio.ListenPipe(socketPath, &winio.PipeConfig{
		SecurityDescriptor: descriptor,
		MessageMode:        false,
		InputBufferSize:    bufferNamedPipeWindows,
		OutputBufferSize:   bufferNamedPipeWindows,
	})
}

func validarNamedPipeLocal(socketPath string) error {
	if !strings.HasPrefix(strings.ToLower(socketPath), prefijoNamedPipeWindows) {
		return errors.New(`la ruta IPC de Windows debe comenzar por \\.\pipe\`)
	}
	nombre := socketPath[len(prefijoNamedPipeWindows):]
	if nombre == "" || strings.ContainsAny(nombre, `\/`) {
		return errors.New("el nombre del named pipe local no es valido")
	}
	if !utf8.ValidString(socketPath) ||
		len(utf16.Encode([]rune(socketPath))) > 256 ||
		strings.IndexFunc(nombre, unicode.IsControl) >= 0 {
		return errors.New("el nombre del named pipe contiene caracteres no validos o es demasiado largo")
	}
	return nil
}

func descriptorSeguridadNamedPipe() (string, error) {
	token := windows.GetCurrentProcessToken()
	tokenUser, err := token.GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("obteniendo el SID del usuario actual: %w", err)
	}
	sidUsuario := tokenUser.User.Sid
	if sidUsuario == nil || !sidUsuario.IsValid() {
		return "", errors.New("el token del proceso no contiene un SID de usuario valido")
	}
	sidUsuarioTexto := sidUsuario.String()
	if sidUsuarioTexto == "" {
		return "", errors.New("no se pudo serializar el SID del usuario actual")
	}

	tokenGroups, err := token.GetTokenGroups()
	if err != nil {
		return "", fmt.Errorf("obteniendo los grupos del token actual: %w", err)
	}
	var sidLogonTexto string
	for _, grupo := range tokenGroups.AllGroups() {
		if grupo.Attributes&windows.SE_GROUP_LOGON_ID != windows.SE_GROUP_LOGON_ID {
			continue
		}
		if grupo.Sid == nil || !grupo.Sid.IsValid() {
			return "", errors.New("el token del proceso contiene un SID de logon no valido")
		}
		sidLogonTexto = grupo.Sid.String()
		break
	}
	if sidLogonTexto == "" {
		return "", errors.New("el token del proceso no contiene un SID de logon")
	}

	return fmt.Sprintf(
		"O:%[1]sD:P(A;;RC;;;OW)(A;;0x12019f;;;%[2]s)",
		sidUsuarioTexto,
		sidLogonTexto,
	), nil
}

// Los named pipes no dejan una entrada de sistema de archivos tras cerrar el
// ultimo handle.
func limpiarIPC(_ string) error { return nil }

func socketPathPorDefecto() string {
	nombreUsuario := "default"
	if home, err := os.UserHomeDir(); err == nil {
		if base := filepath.Base(home); base != "" && base != "." {
			nombreUsuario = base
		}
	}
	nombreUsuario = strings.NewReplacer(`\`, "_", `/`, "_").Replace(nombreUsuario)
	return prefijoNamedPipeWindows + "grxfirma_ipc_" + nombreUsuario
}
