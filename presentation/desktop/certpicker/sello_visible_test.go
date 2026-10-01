// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certpicker

import (
	"context"
	"strings"
	"testing"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/avisos"
)

type selectorSello struct {
	pos, pag string
	err      error
}

func (s selectorSello) ElegirPosicionSello(context.Context) (string, string, error) {
	return s.pos, s.pag, s.err
}

func TestResolverSelloVisible(t *testing.T) {
	quiere := map[string]string{"visibleSignature": "want", "otra": "x"}
	out, err := ResolverSelloVisible(context.Background(), selectorSello{"inferior-derecha", "-1", nil}, domain.FormatPAdES, quiere)
	if err != nil || out[domain.OpcionPosicionSello] != "inferior-derecha" || out[domain.OpcionPaginaSello] != "-1" || out["otra"] != "x" {
		t.Fatalf("out=%v err=%v", out, err)
	}
	if out, _ := ResolverSelloVisible(context.Background(), selectorSello{}, domain.FormatCAdES, quiere); out[domain.OpcionPosicionSello] != "" {
		t.Error("solo aplica a PAdES")
	}
	avisos.Reiniciar()
	defer avisos.Reiniciar()
	out, err = ResolverSelloVisible(context.Background(), selectorSello{err: ErrSinSelloVisible}, domain.FormatPAdES, quiere)
	if err != nil || out["visibleSignature"] != "" || !strings.Contains(avisos.Texto(), "sin sello visible") {
		t.Fatalf("sin sello: out=%v err=%v avisos=%q", out, err, avisos.Texto())
	}
	// Una interfaz sin selector firma sin sello y lo explica.
	if out, err := ResolverSelloVisible(context.Background(), struct{}{}, domain.FormatPAdES, quiere); err != nil || out["visibleSignature"] != "" {
		t.Fatalf("sin selector: out=%v err=%v", out, err)
	}
	if _, err := ResolverSelloVisible(context.Background(), selectorSello{"centro", "1", nil}, domain.FormatPAdES, quiere); err == nil {
		t.Error("una posición inválida debe rechazarse")
	}
}
