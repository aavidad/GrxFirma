// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certpicker

import (
	"context"
	"errors"
	"strings"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/avisos"
)

// ErrSinSelloVisible indica que el usuario prefiere firmar sin sello visible.
var ErrSinSelloVisible = errors.New("el usuario ha elegido firmar sin sello visible")

// ErrPosicionSelloNoDisponible lo devuelven los envoltorios de selector cuando
// la interfaz que envuelven no permite situar la firma.
var ErrPosicionSelloNoDisponible = errors.New("esta interfaz no permite situar la firma visible")

// SelectorPosicionSello lo implementan las interfaces que permiten al usuario
// situar la firma visible cuando la web pide "visibleSignature=want".
// pagina es "1" (primera) o "-1" (última).
type SelectorPosicionSello interface {
	ElegirPosicionSello(ctx context.Context) (posicion, pagina string, err error)
}

// ResolverSelloVisible pregunta al usuario dónde colocar la firma visible si
// la web lo ha pedido y la firma es PAdES. Si la interfaz no permite elegir o
// el usuario prefiere no poner sello, se firma sin sello visible y se le
// explica el motivo.
func ResolverSelloVisible(ctx context.Context, selector any, formato domain.SignatureFormat, opciones map[string]string) (map[string]string, error) {
	if formato != domain.FormatPAdES || !domain.SolicitaElegirSello(opciones) {
		return opciones, nil
	}
	sinSello := func(motivo string) map[string]string {
		out := make(map[string]string, len(opciones))
		for k, v := range opciones {
			if !strings.EqualFold(k, "visibleSignature") {
				out[k] = v
			}
		}
		avisos.Registrar("Firma sin sello visible", motivo)
		return out
	}
	sinSelector := func() map[string]string {
		return sinSello("la web pidió que eligiera dónde colocar la firma visible, pero esta interfaz no permite elegirlo; el PDF se firma sin sello visible")
	}
	sel, ok := selector.(SelectorPosicionSello)
	if !ok {
		return sinSelector(), nil
	}
	posicion, pagina, err := sel.ElegirPosicionSello(ctx)
	if errors.Is(err, ErrPosicionSelloNoDisponible) {
		return sinSelector(), nil
	}
	if errors.Is(err, ErrSinSelloVisible) {
		return sinSello("ha elegido firmar sin sello visible; la firma es igual de válida"), nil
	}
	if err != nil {
		return nil, err
	}
	if !domain.PosicionSelloValida(posicion) || (pagina != "1" && pagina != "-1") {
		return nil, errors.New("la posición elegida para el sello no es válida")
	}
	out := make(map[string]string, len(opciones)+2)
	for k, v := range opciones {
		out[k] = v
	}
	out[domain.OpcionPosicionSello] = posicion
	out[domain.OpcionPaginaSello] = pagina
	return out, nil
}
