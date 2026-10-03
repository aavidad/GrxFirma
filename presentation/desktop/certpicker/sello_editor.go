// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certpicker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/digitorus/pdf"
	"grxfirma/internal/domain"
)

const MaxResultadoEditorSello = 32 * 1024

// ErrEditorSelloNoDisponible permite conservar el diálogo de seis posiciones.
var ErrEditorSelloNoDisponible = errors.New("el editor visual del sello no está disponible")

type SelectorEditorSello interface {
	ElegirSelloEnEditor(context.Context, domain.Document, string) ([]byte, error)
}

type rectEditorSello struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

type placementEditorSello struct {
	Page     int             `json:"page"`
	Rect     rectEditorSello `json:"rect"`
	Rotation int             `json:"rotation"`
}

type resultadoEditorSello struct {
	Action                string                 `json:"action"`
	VisibleSealPlacements []placementEditorSello `json:"visibleSealPlacements,omitempty"`
	Appearance            *appearanceEditorSello `json:"appearance,omitempty"`
}

type appearanceEditorSello struct {
	Logo           string `json:"logo"`
	OpacityPercent *int   `json:"opacityPercent"`
}

// ValidarResultadoEditorSello trata la respuesta del proceso hijo como entrada no fiable.
// Devuelve un JSON canónico de colocaciones o los errores de decisión del usuario.
func ValidarResultadoEditorSello(raw []byte, paginas int) (string, error) {
	placements, _, err := validarResultadoEditorSello(raw, paginas)
	return placements, err
}

func validarResultadoEditorSello(raw []byte, paginas int) (string, map[string]string, error) {
	if len(raw) == 0 || len(raw) > MaxResultadoEditorSello {
		return "", nil, errors.New(tp("portal.seal.error.result"))
	}
	if err := rechazarClavesDuplicadasEditor(raw); err != nil {
		return "", nil, errors.New(tp("portal.seal.error.result"))
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var result resultadoEditorSello
	if err := dec.Decode(&result); err != nil {
		return "", nil, errors.New(tp("portal.seal.error.result"))
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return "", nil, errors.New(tp("portal.seal.error.result"))
	}
	switch result.Action {
	case "cancel":
		if len(result.VisibleSealPlacements) != 0 || result.Appearance != nil {
			return "", nil, errors.New(tp("portal.seal.error.result"))
		}
		return "", nil, ErrSeleccionCancelada
	case "without":
		if len(result.VisibleSealPlacements) != 0 || result.Appearance != nil {
			return "", nil, errors.New(tp("portal.seal.error.result"))
		}
		return "", nil, ErrSinSelloVisible
	case "place":
	default:
		return "", nil, errors.New(tp("portal.seal.error.result"))
	}
	if paginas < 1 || len(result.VisibleSealPlacements) < 1 || len(result.VisibleSealPlacements) > paginas || len(result.VisibleSealPlacements) > 128 {
		return "", nil, errors.New(tp("portal.seal.error.placement"))
	}
	seen := make(map[int]bool, len(result.VisibleSealPlacements))
	for _, p := range result.VisibleSealPlacements {
		if p.Page < 1 || p.Page > paginas || seen[p.Page] || p.Rotation < 0 || p.Rotation > 359 {
			return "", nil, errors.New(tp("portal.seal.error.placement"))
		}
		seen[p.Page] = true
		r := p.Rect
		finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
		if !finite(r.X) || !finite(r.Y) || !finite(r.W) || !finite(r.H) ||
			r.X < 0 || r.Y < 0 || r.W < 0.01 || r.H < 0.01 || r.X+r.W > 1 || r.Y+r.H > 1 {
			return "", nil, errors.New(tp("portal.seal.error.placement"))
		}
	}
	canonical, err := json.Marshal(result.VisibleSealPlacements)
	if err != nil {
		return "", nil, err
	}
	appearance := make(map[string]string)
	if result.Appearance != nil {
		a := result.Appearance
		if a.OpacityPercent == nil || *a.OpacityPercent < 0 || *a.OpacityPercent > 100 ||
			(a.Logo != "text" && a.Logo != "institutional") {
			return "", nil, errors.New(tp("portal.seal.error.result"))
		}
		if a.Logo == "institutional" {
			appearance["visibleSealLogo"] = "institucional"
			appearance["visibleSealLogoOpacityPercent"] = fmt.Sprint(*a.OpacityPercent)
		}
	}
	return string(canonical), appearance, nil
}

func rechazarClavesDuplicadasEditor(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	var visitar func(int) error
	visitar = func(depth int) error {
		if depth > 16 {
			return errors.New("depth")
		}
		token, err := dec.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := make(map[string]bool)
			for dec.More() {
				keyToken, err := dec.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok || seen[key] {
					return errors.New("duplicate")
				}
				seen[key] = true
				if err := visitar(depth + 1); err != nil {
					return err
				}
			}
			_, err := dec.Token()
			return err
		case '[':
			for dec.More() {
				if err := visitar(depth + 1); err != nil {
					return err
				}
			}
			_, err := dec.Token()
			return err
		default:
			return errors.New("unexpected delimiter")
		}
	}
	if err := visitar(0); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing data")
	}
	return nil
}

func paginasPDFEditor(documento domain.Document) (int, error) {
	if len(documento.Content) == 0 || len(documento.Content) > 100*1024*1024 {
		return 0, ErrEditorSelloNoDisponible
	}
	r, err := pdf.NewReader(bytes.NewReader(documento.Content), int64(len(documento.Content)))
	if err != nil || r.NumPage() < 1 || r.NumPage() > 1000000 {
		return 0, ErrEditorSelloNoDisponible
	}
	return r.NumPage(), nil
}
