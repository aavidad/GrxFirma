// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

// B-06: un servidor de larga duración renueva la hoja TLS antes de que caduque.
func TestRecargaHojaTLS_RenuevaAntesDeCaducar(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()
	r, err := newLocalLeafReloader(dirA)
	if err != nil {
		t.Fatalf("newLocalLeafReloader: %v", err)
	}
	primera, err := r.getCertificate(nil)
	if err != nil {
		t.Fatalf("getCertificate: %v", err)
	}
	if primera.Leaf == nil {
		t.Fatal("hoja sin Leaf")
	}

	llamadas := 0
	r.ensure = func() (string, string, error) {
		llamadas++
		cert, key, _, _, err := EnsureBrowserCompatibleLocalhostCertificate(dirB, defaultCertPrefix)
		return cert, key, err
	}

	// Lejos de caducar: se sirve la misma hoja sin tocar el disco.
	r.now = func() time.Time { return primera.Leaf.NotAfter.Add(-2 * localServerRenewBefore) }
	misma, _ := r.getCertificate(nil)
	if llamadas != 0 || !bytes.Equal(misma.Certificate[0], primera.Certificate[0]) {
		t.Fatalf("recarga innecesaria: llamadas=%d", llamadas)
	}

	// Dentro del margen de renovación: se pide hoja nueva y se sirve.
	r.now = func() time.Time { return primera.Leaf.NotAfter.Add(-time.Hour) }
	nueva, err := r.getCertificate(nil)
	if err != nil {
		t.Fatalf("getCertificate tras renovar: %v", err)
	}
	if llamadas != 1 || bytes.Equal(nueva.Certificate[0], primera.Certificate[0]) {
		t.Fatalf("no se recargó la hoja: llamadas=%d", llamadas)
	}

	// Si la renovación falla se sigue sirviendo la hoja actual.
	r.ensure = func() (string, string, error) { return "", "", errors.New("disco lleno") }
	r.now = func() time.Time { return nueva.Leaf.NotAfter.Add(-time.Hour) }
	sigue, err := r.getCertificate(nil)
	if err != nil || !bytes.Equal(sigue.Certificate[0], nueva.Certificate[0]) {
		t.Fatalf("fallo de renovación debe conservar la hoja: %v", err)
	}
}
