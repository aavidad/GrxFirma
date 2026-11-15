// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package securefile

import "os"

// En Windows OpenDir ya rechaza uniones y enlaces. El propietario no se
// compara: un proceso elevado del mismo usuario crea carpetas cuyo
// propietario es Administradores. La carpeta vive en %LOCALAPPDATA%, que el
// perfil ya reserva al usuario, y EnsurePrivateDir le aplica la DACL privada.
func checkOpenedDirOwner(*os.File) error { return nil }

func checkOpenedDirPrivate(*os.File) error { return nil }
