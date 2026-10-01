// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import "strings"

// Posición del sello visible elegida por el usuario cuando la web pide
// "visibleSignature=want" (AutoFirma Java deja al usuario situar la firma).
// La aplicación la guarda en OpcionPosicionSello y el motor calcula el
// rectángulo con el tamaño real de la página elegida.
const (
	OpcionPosicionSello = "grxfirma.posicionSello"
	// OpcionPaginaSello: "1" primera página, "-1" última.
	OpcionPaginaSello = "grxfirma.paginaSello"
)

// PosicionesSello son las posiciones que se ofrecen al usuario.
var PosicionesSello = []string{
	"superior-izquierda", "superior-centro", "superior-derecha",
	"inferior-izquierda", "inferior-centro", "inferior-derecha",
}

// PosicionSelloValida indica si la posición es una de las ofrecidas.
func PosicionSelloValida(p string) bool {
	for _, v := range PosicionesSello {
		if v == p {
			return true
		}
	}
	return false
}

// SolicitaElegirSello indica si la web pide que el usuario sitúe la firma
// visible y todavía no se ha elegido la posición.
func SolicitaElegirSello(opciones map[string]string) bool {
	for k, v := range opciones {
		if strings.EqualFold(k, "visibleSignature") && strings.EqualFold(strings.TrimSpace(v), "want") {
			return strings.TrimSpace(opciones[OpcionPosicionSello]) == ""
		}
	}
	return false
}
