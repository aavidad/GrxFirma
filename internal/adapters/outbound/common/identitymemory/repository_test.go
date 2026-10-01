// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package identitymemory

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

func TestRepositorioEntregaUnSoloGanadorConcurrente(t *testing.T) {
	repositorio := nuevoRepositorioPrueba(t, 8)
	reto := retoMemoriaPrueba("reto:concurrente")
	if err := repositorio.Registrar(context.Background(), reto); err != nil {
		t.Fatalf("registrar: %v", err)
	}
	var ganadores atomic.Int32
	var grupo sync.WaitGroup
	for range 32 {
		grupo.Add(1)
		go func() {
			defer grupo.Done()
			if _, err := repositorio.Reservar(context.Background(), reto.Solicitud.RetoID, reto.Solicitud.EmitidoEn); err == nil {
				ganadores.Add(1)
			} else if !errors.Is(err, ErrRetoNoDisponible) {
				t.Errorf("error inesperado: %v", err)
			}
		}()
	}
	grupo.Wait()
	if ganadores.Load() != 1 {
		t.Fatalf("ganadores: %d", ganadores.Load())
	}
}

func TestRepositorioCopiaEntradaYFinalizaUnaVez(t *testing.T) {
	repositorio := nuevoRepositorioPrueba(t, 2)
	reto := retoMemoriaPrueba("reto:1")
	if err := repositorio.Registrar(context.Background(), reto); err != nil {
		t.Fatalf("registrar: %v", err)
	}
	reto.Solicitud.Nonce[0] = 9
	reto.ContenidoCanonico[0] = 'X'
	reservado, err := repositorio.Reservar(context.Background(), "reto:1", reto.Solicitud.EmitidoEn)
	if err != nil {
		t.Fatalf("reservar: %v", err)
	}
	if reservado.Solicitud.Nonce[0] != 0 || string(reservado.ContenidoCanonico) != "canon" {
		t.Fatal("el repositorio compartió memoria mutable")
	}
	resultado := domain.ResultadoVerificacionIdentidad{Resultado: domain.ResultadoIdentidadIndeterminada,
		EvidenciaRef: "evidencia:1", RetoID: "reto:1", Solicitud: reservado.Solicitud}
	if err := repositorio.Finalizar(context.Background(), resultado); err != nil {
		t.Fatalf("finalizar: %v", err)
	}
	if err := repositorio.Finalizar(context.Background(), resultado); !errors.Is(err, ErrRetoNoDisponible) {
		t.Fatalf("segunda finalización admitida: %v", err)
	}
}

func TestRepositorioAplicaCapacidadYCaducidad(t *testing.T) {
	repositorio := nuevoRepositorioPrueba(t, 1)
	primero := retoMemoriaPrueba("reto:1")
	if err := repositorio.Registrar(context.Background(), primero); err != nil {
		t.Fatalf("registrar primero: %v", err)
	}
	segundo := retoMemoriaPrueba("reto:2")
	if err := repositorio.Registrar(context.Background(), segundo); !errors.Is(err, ErrCapacidadAgotada) {
		t.Fatalf("capacidad no aplicada: %v", err)
	}
	if _, err := repositorio.Reservar(context.Background(), "reto:1", primero.Solicitud.ExpiraEn); !errors.Is(err, ErrRetoNoDisponible) {
		t.Fatalf("reto caducado admitido: %v", err)
	}
}

func nuevoRepositorioPrueba(t *testing.T, maximo int) *Repositorio {
	t.Helper()
	repositorio, err := Nuevo(maximo, time.Minute)
	if err != nil {
		t.Fatalf("construir repositorio: %v", err)
	}
	return repositorio
}

func retoMemoriaPrueba(id string) domain.RetoIdentidad {
	emitido := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	return domain.RetoIdentidad{Solicitud: domain.SolicitudRetoIdentidad{
		Contrato: domain.VersionContratoIdentidadReforzada, RetoID: id,
		Audiencia: "urn:dipgra:identidad", ClienteRegistrado: "cliente",
		Finalidad: "Acreditar identidad", Operacion: "identidad.reforzar.v1",
		HuellaContextoTenant: "hmac:tenant", VinculoSesion: "hmac:sesion",
		Origen: "https://integrador.example", ConsentimientoID: "consentimiento",
		VersionConsentimiento: "1", PoliticaID: "politica", VersionPolitica: "1",
		Nonce: make([]byte, 32), EmitidoEn: emitido, ExpiraEn: emitido.Add(time.Minute),
	}, ContenidoCanonico: []byte("canon")}
}
