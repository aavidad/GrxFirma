// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"fmt"
	"math"
	"time"

	"grxfirma/internal/domain"
)

// DiasAvisoCaducidad es la antelación con la que se avisa de que un
// certificado va a caducar, suficiente para renovarlo sin quedarse sin firma.
const DiasAvisoCaducidad = 30

// DiasHastaCaducidad devuelve los días completos que faltan para que caduque
// el certificado (negativo si ya caducó) y false si no se conoce la fecha.
func DiasHastaCaducidad(c domain.CertificateRef, ahora time.Time) (int, bool) {
	if c.NotAfter.IsZero() {
		return 0, false
	}
	return int(math.Floor(c.NotAfter.Sub(ahora).Hours() / 24)), true
}

// AvisoCaducidadCertificado explica al usuario que su certificado caduca
// pronto; devuelve "" si no procede avisar.
func AvisoCaducidadCertificado(c domain.CertificateRef, ahora time.Time) string {
	dias, ok := DiasHastaCaducidad(c, ahora)
	if !ok || dias > DiasAvisoCaducidad {
		return ""
	}
	fecha := c.NotAfter.Local().Format("02/01/2006")
	switch {
	case dias < 0:
		return fmt.Sprintf("El certificado caducó el %s: las firmas nuevas no serán válidas. Renuévelo con su emisor.", fecha)
	case dias == 0:
		return fmt.Sprintf("El certificado caduca hoy (%s). Renuévelo con su emisor para seguir firmando.", fecha)
	case dias == 1:
		return fmt.Sprintf("El certificado caduca mañana (%s). Renuévelo con su emisor para seguir firmando.", fecha)
	default:
		return fmt.Sprintf("El certificado caduca en %d días (%s). Renuévelo con su emisor antes de esa fecha para seguir firmando.", dias, fecha)
	}
}
