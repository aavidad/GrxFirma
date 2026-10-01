// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestVinculacionPIDFrontend_ConexionPreviaEsperaPublicacion(t *testing.T) {
	t.Parallel()

	var binding vinculacionPIDFrontend
	if err := binding.preparar(); err != nil {
		t.Fatalf("preparar: %v", err)
	}

	resultado := make(chan error, 1)
	go func() {
		resultado <- binding.autorizar(context.Background(), 4321)
	}()

	select {
	case err := <-resultado:
		t.Fatalf("la conexion no debio resolverse antes de publicar: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	if err := binding.publicar(4321); err != nil {
		t.Fatalf("publicar: %v", err)
	}
	select {
	case err := <-resultado:
		if err != nil {
			t.Fatalf("autorizar tras publicar: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("la conexion no se desbloqueo tras publicar el PID")
	}
}

func TestVinculacionPIDFrontend_RechazaPIDDistintoYAdmiteReconexiones(t *testing.T) {
	t.Parallel()

	var binding vinculacionPIDFrontend
	if err := binding.preparar(); err != nil {
		t.Fatalf("preparar: %v", err)
	}
	if err := binding.publicar(1234); err != nil {
		t.Fatalf("publicar: %v", err)
	}
	if err := binding.autorizar(context.Background(), 9876); !errors.Is(err, errPeerNoAutorizado) {
		t.Fatalf("PID incorrecto: error=%v", err)
	}
	for i := 0; i < 2; i++ {
		if err := binding.autorizar(context.Background(), 1234); err != nil {
			t.Fatalf("reconexion %d rechazada: %v", i, err)
		}
	}
}

func TestVinculacionPIDFrontend_DenegacionDesbloqueaFailClosed(t *testing.T) {
	t.Parallel()

	var binding vinculacionPIDFrontend
	if err := binding.preparar(); err != nil {
		t.Fatalf("preparar: %v", err)
	}
	resultado := make(chan error, 1)
	go func() {
		resultado <- binding.autorizar(context.Background(), 4321)
	}()
	binding.denegar()

	select {
	case err := <-resultado:
		if !errors.Is(err, errPeerNoAutorizado) {
			t.Fatalf("error tras denegar=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("la denegacion no desbloqueo la conexion pendiente")
	}
	if err := binding.publicar(4321); !errors.Is(err, errVinculacionPIDNoPreparada) {
		t.Fatalf("publicacion posterior a deny=%v", err)
	}
}
