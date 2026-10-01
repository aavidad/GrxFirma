// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certcatalogagg

import (
	"context"
	"sync"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// TimeoutPorFuentePorDefecto acota cuánto puede tardar una fuente individual
// en List antes de descartarla en esa pasada. Las fuentes son locales
// (certutil, keychain, directorio P12), así que superar este límite indica
// una fuente colgada, no una fuente lenta legítima.
const TimeoutPorFuentePorDefecto = 30 * time.Second

// Agregador combina múltiples fuentes de certificados en una vista única y deduplicada.
// Las fuentes se consultan en el orden en que se registraron.
// Si una fuente retorna error, el error se registra (ver ErroresFuente) y se continúa
// con las demás (degradación controlada).
//
// Es seguro para uso concurrente: la misma instancia se comparte entre los
// adaptadores de entrada (WebSocket, REST, service) que atienden peticiones
// en paralelo.
type Agregador struct {
	// TimeoutPorFuente acota cada llamada individual a una fuente.
	// Cero o negativo desactiva el límite. Debe fijarse antes del primer
	// List concurrente.
	TimeoutPorFuente time.Duration

	mu            sync.Mutex
	fuentes       []ports.CertificateCatalog
	erroresFuente []error
}

// New crea un Agregador con las fuentes indicadas.
// El orden de las fuentes determina la prioridad en caso de duplicados:
// la primera aparición de un fingerprint es la que se conserva.
func New(fuentes ...ports.CertificateCatalog) *Agregador {
	fs := make([]ports.CertificateCatalog, 0, len(fuentes))
	for _, f := range fuentes {
		if f != nil {
			fs = append(fs, f)
		}
	}
	return &Agregador{fuentes: fs, TimeoutPorFuente: TimeoutPorFuentePorDefecto}
}

// List agrega los certificados de todas las fuentes, deduplicando por fingerprint SHA-256.
// Los errores de fuentes individuales se recuperan con ErroresFuente y no bloquean
// el resultado de las fuentes que sí funcionan. Una fuente que exceda
// TimeoutPorFuente se registra como error y no retrasa a las demás.
func (a *Agregador) List(ctx context.Context) ([]domain.CertificateRef, error) {
	a.mu.Lock()
	fuentes := make([]ports.CertificateCatalog, len(a.fuentes))
	copy(fuentes, a.fuentes)
	timeout := a.TimeoutPorFuente
	a.mu.Unlock()

	seen := make(map[string]struct{})
	var resultado []domain.CertificateRef
	var errores []error

	for _, fuente := range fuentes {
		if ctx.Err() != nil {
			a.guardarErrores(errores)
			return resultado, ctx.Err()
		}

		refs, err := listarFuente(ctx, fuente, timeout)
		if err != nil {
			errores = append(errores, err)
			continue
		}

		for _, ref := range refs {
			fp := ref.Fingerprint
			if fp == "" {
				fp = ref.ID // fallback si no hay fingerprint completo
			}
			if _, dup := seen[fp]; dup {
				continue
			}
			seen[fp] = struct{}{}
			resultado = append(resultado, ref)
		}
	}

	a.guardarErrores(errores)
	return resultado, nil
}

// listarFuente consulta una fuente acotada por el timeout. La consulta corre
// en una goroutine propia para que una fuente que ignora el contexto y se
// queda colgada no bloquee al agregador; su resultado tardío se descarta.
func listarFuente(ctx context.Context, fuente ports.CertificateCatalog, timeout time.Duration) ([]domain.CertificateRef, error) {
	if timeout <= 0 {
		return fuente.List(ctx)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type respuesta struct {
		refs []domain.CertificateRef
		err  error
	}
	ch := make(chan respuesta, 1)
	go func() {
		refs, err := fuente.List(ctx)
		ch <- respuesta{refs: refs, err: err}
	}()

	select {
	case r := <-ch:
		return r.refs, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *Agregador) guardarErrores(errores []error) {
	a.mu.Lock()
	a.erroresFuente = errores
	a.mu.Unlock()
}

// ErroresFuente retorna los errores de fuentes individuales de la última
// llamada a List, para diagnóstico. Retorna una copia.
func (a *Agregador) ErroresFuente() []error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.erroresFuente) == 0 {
		return nil
	}
	out := make([]error, len(a.erroresFuente))
	copy(out, a.erroresFuente)
	return out
}

// AñadirFuente añade una fuente al agregador en tiempo de ejecución.
// Útil para registrar fuentes que solo están disponibles tras la inicialización
// (p. ej. PKCS#11 cuando se detecta hardware).
func (a *Agregador) AñadirFuente(f ports.CertificateCatalog) {
	if f == nil {
		return
	}
	a.mu.Lock()
	a.fuentes = append(a.fuentes, f)
	a.mu.Unlock()
}

// TieneFuentes retorna true si el agregador tiene al menos una fuente registrada.
func (a *Agregador) TieneFuentes() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.fuentes) > 0
}

var _ ports.CertificateCatalog = (*Agregador)(nil)
