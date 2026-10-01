// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"strconv"
	"strings"
)

const (
	maxSealRasterEdge   = 4096
	maxSealRasterPixels = 8 * 1024 * 1024
	maxSealImageEdge    = 8192
	maxSealImagePixels  = 16 * 1024 * 1024
)

// dimensionesRasterSello limita la resolución, no el rectángulo físico del PDF.
// La escala se calcula antes de multiplicar para no desbordar con valores finitos enormes.
func dimensionesRasterSello(widthPt, heightPt float64, minWidth, minHeight int) (int, int, error) {
	if !numeroFinitoSello(widthPt) || !numeroFinitoSello(heightPt) || widthPt <= 0 || heightPt <= 0 {
		return 0, 0, fmt.Errorf("el ancho y alto del sello deben ser números finitos mayores que cero")
	}
	scale := math.Min(3, math.Min(float64(maxSealRasterEdge)/widthPt, float64(maxSealRasterEdge)/heightPt))
	w := math.Max(float64(minWidth), widthPt*scale)
	h := math.Max(float64(minHeight), heightPt*scale)
	if w*h > maxSealRasterPixels {
		factor := math.Sqrt(float64(maxSealRasterPixels) / (w * h))
		w, h = w*factor, h*factor
	}
	width := maxInt(1, int(math.Floor(w)))
	height := maxInt(1, int(math.Floor(h)))
	if err := validarPixelesSello(width, height, maxSealRasterEdge, maxSealRasterPixels); err != nil {
		return 0, 0, err
	}
	return width, height, nil
}

func numeroFinitoSello(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func validarGeometriaSello(options map[string]string, x, y, w, h float64) error {
	for _, key := range []string{"visibleSealRectX", "visibleSealRectY", "visibleSealRectW", "visibleSealRectH"} {
		if raw := strings.TrimSpace(valorOpcion(options, key)); raw != "" {
			if value, err := strconv.ParseFloat(raw, 64); err != nil || !numeroFinitoSello(value) {
				return fmt.Errorf("la geometría del sello contiene un número no válido en %s", key)
			}
		}
	}
	if !numeroFinitoSello(x) || !numeroFinitoSello(y) || !numeroFinitoSello(w) || !numeroFinitoSello(h) ||
		!numeroFinitoSello(x+w) || !numeroFinitoSello(y+h) || w <= 0 || h <= 0 {
		return fmt.Errorf("la posición del sello debe ser finita y su ancho y alto mayores que cero")
	}
	return nil
}

func validarPixelesSello(width, height, maxEdge, maxPixels int) error {
	// División en vez de multiplicación para no desbordar int con dimensiones de cabecera.
	if width <= 0 || height <= 0 || width > maxEdge || height > maxEdge || width > maxPixels/height {
		return fmt.Errorf("la imagen del sello excede las dimensiones permitidas; reduzca su resolución")
	}
	return nil
}

// decodificarImagenSello inspecciona la cabecera antes de reservar el mapa de píxeles.
func decodificarImagenSello(raw []byte) (image.Image, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer la imagen del sello")
	}
	if err := validarPixelesSello(config.Width, config.Height, maxSealImageEdge, maxSealImagePixels); err != nil {
		return nil, err
	}
	decoded, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("no se pudo decodificar la imagen del sello")
	}
	return decoded, nil
}
