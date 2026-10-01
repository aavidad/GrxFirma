// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"fmt"
	"math"
)

// cajaSelloGirado conserva x/y/w/h como tarjeta sin girar, en puntos PDF.
// El giro cambia únicamente la caja envolvente, alrededor del mismo centro.
// Si se conoce la página, se desplaza la caja completa para mantenerla dentro.
func cajaSelloGirado(x, y, w, h, grados, pageX, pageY, pageW, pageH float64) ([4]float64, error) {
	if !numeroFinitoSello(x) || !numeroFinitoSello(y) || !numeroFinitoSello(w) || !numeroFinitoSello(h) ||
		!numeroFinitoSello(grados) || !numeroFinitoSello(pageX) || !numeroFinitoSello(pageY) ||
		!numeroFinitoSello(pageW) || !numeroFinitoSello(pageH) || w <= 0 || h <= 0 {
		return [4]float64{}, fmt.Errorf("geometría o giro del sello no válido")
	}
	r := grados * math.Pi / 180
	c, s := math.Abs(math.Cos(r)), math.Abs(math.Sin(r))
	bw, bh := w*c+h*s, w*s+h*c
	if !numeroFinitoSello(bw) || !numeroFinitoSello(bh) {
		return [4]float64{}, fmt.Errorf("caja girada del sello no válida")
	}
	left, bottom := x+(w-bw)/2, y+(h-bh)/2
	if pageW > 0 && pageH > 0 {
		if bw > pageW+1e-6 || bh > pageH+1e-6 {
			return [4]float64{}, fmt.Errorf("el sello girado no cabe en la página; reduzca el tamaño de la tarjeta")
		}
		left = math.Max(pageX, math.Min(left, pageX+pageW-bw))
		bottom = math.Max(pageY, math.Min(bottom, pageY+pageH-bh))
	}
	return [4]float64{left, bottom, left + bw, bottom + bh}, nil
}
