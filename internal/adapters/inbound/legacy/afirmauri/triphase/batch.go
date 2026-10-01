// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package triphase

import (
	"context"
	"sync"

	"grxfirma/internal/domain"
)

const defaultMaxConcurrency = 4

// BatchJob representa un único trabajo dentro del batch.
type BatchJob struct {
	Job     domain.SignatureJob
	Session domain.ExchangeSession
}

// BatchResult contiene el resultado de un job individual dentro del batch.
type BatchResult struct {
	Index  int
	Result domain.SignatureResult
	Err    error
}

// BatchExecutor ejecuta el protocolo trifásico para múltiples documentos en paralelo.
type BatchExecutor struct {
	executor *Executor // reusar el Executor de un único documento
	maxConc  int       // paralelismo máximo
}

// NewBatch construye un BatchExecutor con concurrencia por defecto (4).
func NewBatch(e *Executor) *BatchExecutor {
	return NewBatchWithConcurrency(e, defaultMaxConcurrency)
}

// NewBatchWithConcurrency construye un BatchExecutor con concurrencia máxima personalizada.
// Si maxConc es menor que 1 se usa 1.
func NewBatchWithConcurrency(e *Executor, maxConc int) *BatchExecutor {
	if maxConc < 1 {
		maxConc = 1
	}
	return &BatchExecutor{
		executor: e,
		maxConc:  maxConc,
	}
}

// Execute ejecuta todos los jobs en paralelo (hasta maxConc goroutines simultáneas).
// Retorna un slice de BatchResult en el mismo orden que los jobs de entrada.
// Un error en un job no cancela los otros (modo best-effort).
// Si el contexto está cancelado antes de lanzar un job, ese job devuelve ctx.Err().
func (b *BatchExecutor) Execute(ctx context.Context, jobs []BatchJob) []BatchResult {
	results := make([]BatchResult, len(jobs))

	if len(jobs) == 0 {
		return results
	}

	// Semáforo para limitar la concurrencia máxima.
	sem := make(chan struct{}, b.maxConc)

	var wg sync.WaitGroup
	wg.Add(len(jobs))

	for i, bj := range jobs {
		i, bj := i, bj // capturar variables de bucle

		// Adquirir el semáforo o retornar si el contexto está cancelado.
		select {
		case sem <- struct{}{}:
			// Semáforo adquirido, lanzar goroutine.
		case <-ctx.Done():
			// Contexto cancelado: registrar error para este y los restantes jobs.
			results[i] = BatchResult{Index: i, Err: ctx.Err()}
			wg.Done()
			// Cancelar los jobs restantes (desde i+1 en adelante) sin goroutine.
			for j := i + 1; j < len(jobs); j++ {
				results[j] = BatchResult{Index: j, Err: ctx.Err()}
				wg.Done()
			}
			// Esperar las goroutines ya lanzadas.
			wg.Wait()
			return results
		}

		go func() {
			defer func() {
				<-sem // liberar el semáforo
				wg.Done()
			}()

			// Antes de ejecutar comprobar si el contexto ya está cancelado.
			if err := ctx.Err(); err != nil {
				results[i] = BatchResult{Index: i, Err: err}
				return
			}

			res, err := b.executor.Execute(ctx, bj.Session, bj.Job)
			results[i] = BatchResult{Index: i, Result: res, Err: err}
		}()
	}

	wg.Wait()
	return results
}
