// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"net"
	"strings"
)

// Estado del cliente IPC respecto a la interfaz que lanzó el motor. La firma
// remota CSC (nueva en la 0.0.125) solo se admite desde la interfaz
// vinculada por PID: otro proceso del mismo usuario no puede configurar un
// servicio, abrir la autorización ni firmar con una credencial remota. Lo
// anterior (firmas locales con un --server manual sin PID) sigue igual, con
// aviso en el registro y en el diagnóstico.
const (
	interfazVerificada   = "verified"
	interfazSinVinculo   = "unbound"
	interfazNoVerificada = "unverified"
)

type estadoInterfazContextKey struct{}

// requiereEstadoInterfaz indica las acciones que necesitan saber si el
// cliente es la interfaz vinculada.
func requiereEstadoInterfaz(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "sign", "verify", "unsafe_exceptions_list", "unsafe_exceptions_set", "unsafe_exceptions_activate", "unsafe_operation_retry", "csc_configure", "csc_connect", "csc_send_otp", "export_diagnostic", "generate_eni_file":
		return true
	}
	return isRemoteSigningAction(strings.ToLower(strings.TrimSpace(action)))
}

// estadoInterfaz comprueba en cada petición, como la configuración de
// tarjetas, el PID que acredita el sistema operativo.
func (s *Servidor) estadoInterfaz(ctx context.Context, conn net.Conn) string {
	state, _, _ := s.peerPID.snapshot()
	if state == vinculacionSinConfigurar {
		return interfazSinVinculo
	}
	if state != vinculacionPublicada || ctx.Err() != nil {
		return interfazNoVerificada
	}
	pid, err := verificarPeer(conn)
	if err != nil || pid == 0 || s.peerPID.autorizar(ctx, pid) != nil {
		return interfazNoVerificada
	}
	return interfazVerificada
}

func conEstadoInterfaz(ctx context.Context, estado string) context.Context {
	return context.WithValue(ctx, estadoInterfazContextKey{}, estado)
}

func estadoInterfazDe(ctx context.Context) string {
	if estado, ok := ctx.Value(estadoInterfazContextKey{}).(string); ok {
		return estado
	}
	return interfazNoVerificada
}

func esInterfazVerificada(ctx context.Context) bool {
	return estadoInterfazDe(ctx) == interfazVerificada
}
