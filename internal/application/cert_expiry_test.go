// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"strings"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

func TestAvisoCaducidadCertificado(t *testing.T) {
	ahora := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	casos := []struct {
		caduca   time.Time
		contiene string
	}{
		{time.Time{}, ""},
		{ahora.AddDate(0, 0, 31), ""},
		{ahora.AddDate(0, 0, 30).Add(time.Hour), "30 días"},
		{ahora.AddDate(0, 0, 12).Add(time.Hour), "12 días"},
		{ahora.Add(30 * time.Hour), "mañana"},
		{ahora.Add(2 * time.Hour), "hoy"},
		{ahora.AddDate(0, 0, -3), "caducó"},
	}
	for _, c := range casos {
		aviso := AvisoCaducidadCertificado(domain.CertificateRef{NotAfter: c.caduca}, ahora)
		if c.contiene == "" && aviso != "" || !strings.Contains(aviso, c.contiene) {
			t.Errorf("caduca=%v: aviso=%q, se esperaba %q", c.caduca, aviso, c.contiene)
		}
	}
}
