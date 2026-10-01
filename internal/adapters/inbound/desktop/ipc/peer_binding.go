// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const timeoutVinculacionPIDFrontend = 10 * time.Second

var (
	errVinculacionPIDNoPreparada = errors.New("la vinculacion del PID del frontend no esta pendiente")
	errPeerNoAutorizado          = errors.New("el proceso peer no esta autorizado")
)

type estadoVinculacionPID uint8

const (
	vinculacionSinConfigurar estadoVinculacionPID = iota
	vinculacionPendiente
	vinculacionPublicada
	vinculacionDenegada
)

// vinculacionPIDFrontend mantiene el gate de admision del proceso frontend.
// El estado pendiente se configura antes de abrir el listener y se resuelve
// justo despues de crear el proceso. Las conexiones que ganen esa carrera
// esperan de forma acotada y nunca se admiten con un PID cero o desconocido.
type vinculacionPIDFrontend struct {
	mu          sync.Mutex
	estado      estadoVinculacionPID
	pidEsperado uint32
	resuelta    chan struct{}
}

func (v *vinculacionPIDFrontend) preparar() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.estado != vinculacionSinConfigurar {
		return errors.New("la vinculacion del PID del frontend ya fue configurada")
	}
	v.estado = vinculacionPendiente
	v.resuelta = make(chan struct{})
	return nil
}

func (v *vinculacionPIDFrontend) publicar(pid uint32) error {
	if pid == 0 {
		return errors.New("el PID del frontend debe ser mayor que cero")
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	if v.estado != vinculacionPendiente {
		return errVinculacionPIDNoPreparada
	}
	v.pidEsperado = pid
	v.estado = vinculacionPublicada
	close(v.resuelta)
	return nil
}

func (v *vinculacionPIDFrontend) denegar() {
	v.mu.Lock()
	defer v.mu.Unlock()

	switch v.estado {
	case vinculacionPendiente:
		v.estado = vinculacionDenegada
		close(v.resuelta)
	case vinculacionPublicada:
		v.estado = vinculacionDenegada
		v.pidEsperado = 0
	}
}

func (v *vinculacionPIDFrontend) autorizar(
	ctx context.Context,
	pidPeer uint32,
) error {
	estado, esperado, resuelta := v.snapshot()
	if estado == vinculacionSinConfigurar {
		return nil
	}
	if estado == vinculacionPendiente {
		timer := time.NewTimer(timeoutVinculacionPIDFrontend)
		defer timer.Stop()
		select {
		case <-resuelta:
		case <-ctx.Done():
			return errors.Join(errPeerNoAutorizado, ctx.Err())
		case <-timer.C:
			return errors.Join(
				errPeerNoAutorizado,
				errors.New("tiempo de vinculacion del frontend agotado"),
			)
		}
		estado, esperado, _ = v.snapshot()
	}
	if estado != vinculacionPublicada || pidPeer == 0 || pidPeer != esperado {
		return fmt.Errorf(
			"%w: PID recibido=%d",
			errPeerNoAutorizado,
			pidPeer,
		)
	}
	return nil
}

func (v *vinculacionPIDFrontend) snapshot() (
	estadoVinculacionPID,
	uint32,
	<-chan struct{},
) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.estado, v.pidEsperado, v.resuelta
}
