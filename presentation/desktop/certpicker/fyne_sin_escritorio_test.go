// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package certpicker

import (
	"context"
	"errors"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/escritorio"
	"grxfirma/internal/domain"
)

// Sin escritorio el selector Fyne no abre ventana ni elige un certificado.
func TestSinEscritorioElSelectorFyneNoEligeNiEspera(t *testing.T) {
	t.Cleanup(escritorio.SustituirDetector(func() bool { return false }))
	certs := []domain.CertificateRef{{ID: "a", Subject: "CN=Persona", NotAfter: time.Now().Add(time.Hour)}}
	inicio := time.Now()
	resultado, err := New().Select(context.Background(), certs)
	if !errors.Is(err, escritorio.ErrSinEscritorio) || resultado.Certificado.ID != "" {
		t.Fatalf("Select sin escritorio: resultado=%+v err=%v", resultado, err)
	}
	if _, _, err := New().ElegirPosicionSello(context.Background()); !errors.Is(err, escritorio.ErrSinEscritorio) {
		t.Fatalf("ElegirPosicionSello sin escritorio = %v", err)
	}
	if time.Since(inicio) > time.Second {
		t.Fatal("el selector esperó sin escritorio")
	}
}
