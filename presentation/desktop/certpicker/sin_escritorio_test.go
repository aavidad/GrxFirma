// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certpicker

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/escritorio"
	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/pdffixture"
)

// Sin escritorio el editor del sello no se lanza: sería una ventana que nadie
// ve, esperando hasta el plazo de cinco minutos.
func TestSinEscritorioElEditorDelSelloNoSeLanza(t *testing.T) {
	t.Cleanup(escritorio.SustituirDetector(func() bool { return false }))
	previo := resolverEjecutableEditorSello
	lanzado := false
	resolverEjecutableEditorSello = func() (string, error) { lanzado = true; return "", errors.New("no debe llegar aquí") }
	t.Cleanup(func() { resolverEjecutableEditorSello = previo })

	_, err := EjecutarEditorSello(context.Background(), domain.Document{Content: pdffixture.Minimal()}, "Firmante")
	if !errors.Is(err, ErrEditorSelloNoDisponible) || !errors.Is(err, escritorio.ErrSinEscritorio) {
		t.Fatalf("editor sin escritorio = %v", err)
	}
	if lanzado {
		t.Fatal("sin escritorio se buscó el editor del sello")
	}
}

type selectorSelloSinEscritorio struct{}

func (selectorSelloSinEscritorio) ElegirSelloEnEditor(ctx context.Context, d domain.Document, n string) ([]byte, error) {
	return EjecutarEditorSello(ctx, d, n)
}

func (selectorSelloSinEscritorio) ElegirPosicionSello(context.Context) (string, string, error) {
	return "", "", escritorio.Comprobar("posicion_sello")
}

// Si la web pide elegir el sello y nadie puede elegirlo, la operación sale con
// error: no se coloca un sello por defecto ni se firma como si la persona
// hubiera decidido.
func TestSinEscritorioElSelloPedidoNoSeDecidePorLaPersona(t *testing.T) {
	t.Cleanup(escritorio.SustituirDetector(func() bool { return false }))
	opciones := map[string]string{"visibleSignature": "want"}
	got, err := ResolverSelloVisible(context.Background(), selectorSelloSinEscritorio{}, domain.FormatPAdES, opciones, domain.Document{Content: pdffixture.Minimal()}, "Firmante")
	if !errors.Is(err, escritorio.ErrSinEscritorio) || got != nil {
		t.Fatalf("sello sin escritorio: opciones=%v err=%v", got, err)
	}
}
