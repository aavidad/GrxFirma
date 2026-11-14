// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certpicker

import (
	"context"
	"strings"
	"unicode"
)

type claveContextoSolicitud struct{}

// ContextoSolicitud describe quién pide usar el certificado y para qué. Se
// muestra en el selector para que el usuario no elija certificado sin saber
// qué sede lo solicita.
type ContextoSolicitud struct {
	Origen    string
	Operacion string
	// OrigenEsDestino indica que Origen no es quien pide la operación sino el
	// servidor que recibirá el resultado (afirma:// directo, donde no se sabe
	// qué web lanzó la URI).
	OrigenEsDestino bool
}

// ConContextoSolicitud adjunta el origen y la operación al contexto.
func ConContextoSolicitud(ctx context.Context, origen, operacion string) context.Context {
	return context.WithValue(ctx, claveContextoSolicitud{}, ContextoSolicitud{
		Origen:    textoSeguroSolicitud(origen, 256),
		Operacion: textoSeguroSolicitud(operacion, 64),
	})
}

// ConContextoDestino adjunta el servidor que recibirá el resultado cuando no
// se conoce quién pidió la operación.
func ConContextoDestino(ctx context.Context, destino, operacion string) context.Context {
	return context.WithValue(ctx, claveContextoSolicitud{}, ContextoSolicitud{
		Origen:          textoSeguroSolicitud(destino, 256),
		Operacion:       textoSeguroSolicitud(operacion, 64),
		OrigenEsDestino: true,
	})
}

// ContextoSolicitudDe recupera el contexto de la solicitud, si existe.
func ContextoSolicitudDe(ctx context.Context) (ContextoSolicitud, bool) {
	if ctx == nil {
		return ContextoSolicitud{}, false
	}
	v, ok := ctx.Value(claveContextoSolicitud{}).(ContextoSolicitud)
	return v, ok && (v.Origen != "" || v.Operacion != "")
}

// textoSeguroSolicitud elimina controles y marcas bidireccionales que
// permitirían disfrazar el origen mostrado.
func textoSeguroSolicitud(valor string, maximo int) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.TrimSpace(valor) {
		if n >= maximo {
			break
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) || unicode.Is(unicode.Cf, r) || r == 0x200B || r == 0xFEFF {
			r = ' '
		}
		b.WriteRune(r)
		n++
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
