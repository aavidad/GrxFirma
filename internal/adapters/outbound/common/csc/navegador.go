// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package csc

import (
	"context"
	"os/exec"
	"runtime"
)

// AbrirNavegadorSistema abre un URL https en el navegador predeterminado.
// No pasa por un intérprete de órdenes: el URL va como un único argumento y
// antes se exige que sea https sin credenciales embebidas.
func AbrirNavegadorSistema(ctx context.Context, destino string) error {
	if _, err := validarURLSegura(destinoSinConsulta(destino)); err != nil {
		return err
	}
	var orden *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// #nosec G204 -- programa fijo; el URL https validado es un único argumento.
		orden = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", destino)
	case "darwin":
		// #nosec G204 -- programa fijo; el URL https validado es un único argumento.
		orden = exec.CommandContext(ctx, "open", destino)
	default:
		// #nosec G204 -- programa fijo; el URL https validado es un único argumento.
		orden = exec.CommandContext(ctx, "xdg-open", destino)
	}
	if err := orden.Start(); err != nil {
		return err
	}
	go func() { _ = orden.Wait() }()
	return nil
}

// destinoSinConsulta quita la consulta para validar el resto del URL: la de
// autorización OAuth sí lleva parámetros legítimos.
func destinoSinConsulta(destino string) string {
	for i := 0; i < len(destino); i++ {
		if destino[i] == '?' {
			return destino[:i]
		}
	}
	return destino
}
