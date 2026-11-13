// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"net/url"
	"strings"
	"unicode"
)

// Longitudes máximas, en runas, de los campos que llegan de fuera y se
// enseñan en el diálogo de aprobación.
const (
	MaxRunasAprobacionCampo     = 160
	MaxRunasAprobacionSolicitud = 512
)

// OperacionAprobacion es lo que el usuario va a aprobar.
type OperacionAprobacion string

const (
	AprobacionFirma       OperacionAprobacion = "sign"
	AprobacionCofirma     OperacionAprobacion = "cosign"
	AprobacionContrafirma OperacionAprobacion = "countersign"
	AprobacionLote        OperacionAprobacion = "batch"
)

// DetalleAprobacion reúne lo que el usuario necesita ver para decidir: qué se
// firma, con qué certificado, quién lo pide y adónde va el resultado. Los
// adaptadores lo rellenan; un ComponedorAprobacion lo convierte en el texto
// del diálogo, traducido y con todo el texto externo neutralizado.
type DetalleAprobacion struct {
	Operacion   OperacionAprobacion
	Certificado string
	Formato     string
	// Documento y Tamano describen el documento de una firma simple.
	// Tamano en bytes; 0 si no se conoce.
	Documento string
	Tamano    int
	// Firmas es el número de firmas que saldrán de esta aprobación (lote).
	Firmas int
	// Documentos son los nombres de los documentos del lote, si se conocen.
	Documentos []string
	// Aplicacion es el texto con el que se presenta quien pide la firma.
	Aplicacion string
	// Origen es el origen comprobado del solicitante (cabecera Origin real).
	Origen string
	// Destinos son las URL u orígenes que recibirán el documento, la firma o
	// decidirán qué se firma (servlets, pre/post de lote, serverUrl).
	Destinos []string
	// OrigenNoVerificable indica que no se sabe qué web pidió la operación
	// (afirma:// directo): solo se conocen los destinos que puso la web.
	OrigenNoVerificable bool
	// SesionNoIdentificada indica que la sede abrió el canal afirma://websocket
	// sin idsession (sedes antiguas): se admite por compatibilidad, pero
	// nada liga los mensajes a ese lanzamiento y el diálogo lo avisa.
	SesionNoIdentificada bool
}

type claveSesionNoIdentificada struct{}

// ConSesionNoIdentificada marca el contexto de las operaciones que llegan por
// un canal afirma://websocket lanzado sin idsession.
func ConSesionNoIdentificada(ctx context.Context) context.Context {
	return context.WithValue(ctx, claveSesionNoIdentificada{}, true)
}

// SesionNoIdentificada dice si la operación llega por un canal lanzado sin
// idsession (véase ConSesionNoIdentificada).
func SesionNoIdentificada(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	marca, _ := ctx.Value(claveSesionNoIdentificada{}).(bool)
	return marca
}

// ComponedorAprobacion convierte un detalle en el texto del diálogo.
type ComponedorAprobacion func(DetalleAprobacion) string

// OperacionAprobacionDeAccion traduce la acción de firma a la operación que
// se enseña.
func OperacionAprobacionDeAccion(accion string) OperacionAprobacion {
	switch strings.ToLower(strings.TrimSpace(accion)) {
	case "cosign":
		return AprobacionCofirma
	case "countersign":
		return AprobacionContrafirma
	default:
		return AprobacionFirma
	}
}

// NeutralizarTextoAprobacion prepara un texto que viene de fuera (nombre de
// documento, sujeto del certificado, finalidad, origen) para enseñarlo en una
// sola línea del diálogo: cambia por espacios los controles (Cc), los
// caracteres de formato (Cf: marcas bidi, aislamientos, de anchura cero) y
// los separadores de línea y párrafo; colapsa los blancos y corta en maximo
// runas. El escapado propio de cada herramienta de diálogo (marcado de
// zenity, kdialog...) se hace aparte, al pintar.
func NeutralizarTextoAprobacion(valor string, maximo int) string {
	valor = strings.TrimSpace(valor)
	if valor == "" || maximo <= 0 {
		return ""
	}
	out := make([]rune, 0, min(len(valor), maximo+1))
	truncado := false
	for _, r := range valor {
		if len(out) >= maximo {
			truncado = true
			break
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.In(r, unicode.Zl, unicode.Zp) || r == unicode.ReplacementChar {
			out = append(out, ' ')
			continue
		}
		out = append(out, r)
	}
	texto := strings.Join(strings.Fields(string(out)), " ")
	if truncado {
		texto += "…"
	}
	return texto
}

// OrigenesDestino reduce una lista de URL a sus orígenes (esquema://host),
// sin repetir y en el orden en que llegan. Lo que no se pueda analizar se
// enseña neutralizado, para que el usuario vea igualmente que hay un destino.
func OrigenesDestino(urls ...string) []string {
	vistos := make(map[string]struct{}, len(urls))
	out := make([]string, 0, len(urls))
	for _, raw := range urls {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		origen := raw
		if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.Host != "" {
			origen = strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
		}
		origen = NeutralizarTextoAprobacion(origen, MaxRunasAprobacionCampo)
		if origen == "" {
			continue
		}
		if _, ok := vistos[origen]; ok {
			continue
		}
		vistos[origen] = struct{}{}
		out = append(out, origen)
	}
	return out
}

// DetalleAprobacionFirma describe la firma de un SignCommand con el
// certificado elegido.
func DetalleAprobacionFirma(cmd SignCommand, cert string) DetalleAprobacion {
	return DetalleAprobacion{
		Operacion:   OperacionAprobacionDeAccion(string(cmd.Action)),
		Certificado: cert,
		Formato:     string(cmd.Format),
		Documento:   cmd.Document.Name,
		Tamano:      len(cmd.Document.Content),
		Firmas:      1,
		Aplicacion:  cmd.RequesterApplication,
		Origen:      cmd.RequesterOrigin,
	}
}
