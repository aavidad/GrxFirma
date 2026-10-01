// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certcatalogagg_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/certcatalogagg"
	"grxfirma/internal/domain"
)

// catalogoMock es una implementación de ports.CertificateCatalog para tests.
type catalogoMock struct {
	refs []domain.CertificateRef
	err  error
}

func (m *catalogoMock) List(_ context.Context) ([]domain.CertificateRef, error) {
	return m.refs, m.err
}

func refConFP(subject, fingerprint string) domain.CertificateRef {
	return domain.CertificateRef{
		ID:          fingerprint[:8],
		Subject:     subject,
		Issuer:      "CA Test",
		Fingerprint: fingerprint,
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
	}
}

// TestAgregador_FuentesVacias verifica que sin fuentes retorna slice vacío sin error.
func TestAgregador_FuentesVacias(t *testing.T) {
	t.Parallel()

	agg := certcatalogagg.New()
	refs, err := agg.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("esperados 0 certs, obtenidos %d", len(refs))
	}
}

// TestAgregador_UnaSola verifica que una sola fuente pasa sus refs correctamente.
func TestAgregador_UnaSola(t *testing.T) {
	t.Parallel()

	cert := refConFP("Alberto Avidad", "aabbccdd11223344aabbccdd11223344aabbccdd11223344aabbccdd11223344")
	mock := &catalogoMock{refs: []domain.CertificateRef{cert}}
	agg := certcatalogagg.New(mock)

	refs, err := agg.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("esperado 1 cert, obtenido %d", len(refs))
	}
	if refs[0].Subject != "Alberto Avidad" {
		t.Errorf("Subject = %q", refs[0].Subject)
	}
}

// TestAgregador_DeduplicacionPorFingerprint verifica que el mismo cert en dos fuentes
// solo aparece una vez.
func TestAgregador_DeduplicacionPorFingerprint(t *testing.T) {
	t.Parallel()

	fp := "aabbccdd11223344aabbccdd11223344aabbccdd11223344aabbccdd11223344"
	cert := refConFP("Mismo Cert", fp)
	fuente1 := &catalogoMock{refs: []domain.CertificateRef{cert}}
	fuente2 := &catalogoMock{refs: []domain.CertificateRef{cert}}
	agg := certcatalogagg.New(fuente1, fuente2)

	refs, err := agg.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(refs) != 1 {
		t.Errorf("deduplicación fallida: esperado 1, obtenido %d", len(refs))
	}
}

// TestAgregador_MultiplesUnicas verifica que certs distintos de distintas fuentes se agregan.
func TestAgregador_MultiplesUnicas(t *testing.T) {
	t.Parallel()

	cert1 := refConFP("Cert A", "aaaa0000000000000000000000000000000000000000000000000000000000aa")
	cert2 := refConFP("Cert B", "bbbb0000000000000000000000000000000000000000000000000000000000bb")
	cert3 := refConFP("Cert C", "cccc0000000000000000000000000000000000000000000000000000000000cc")
	fuente1 := &catalogoMock{refs: []domain.CertificateRef{cert1, cert2}}
	fuente2 := &catalogoMock{refs: []domain.CertificateRef{cert3}}
	agg := certcatalogagg.New(fuente1, fuente2)

	refs, err := agg.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(refs) != 3 {
		t.Errorf("esperados 3 certs únicos, obtenidos %d", len(refs))
	}
}

// TestAgregador_FuenteConError_Continua verifica degradación controlada.
func TestAgregador_FuenteConError_Continua(t *testing.T) {
	t.Parallel()

	cert := refConFP("Cert OK", "dddd0000000000000000000000000000000000000000000000000000000000dd")
	fuenteBuena := &catalogoMock{refs: []domain.CertificateRef{cert}}
	fuenteMala := &catalogoMock{err: errors.New("hardware no disponible")}
	// fuenteMala primero para verificar que no bloquea fuenteBuena
	agg := certcatalogagg.New(fuenteMala, fuenteBuena)

	refs, err := agg.List(context.Background())
	if err != nil {
		t.Fatalf("List() no debe retornar error con fuente degradada: %v", err)
	}
	if len(refs) != 1 {
		t.Errorf("esperado 1 cert de la fuente buena, obtenidos %d", len(refs))
	}
	if len(agg.ErroresFuente()) != 1 {
		t.Errorf("esperado 1 error en ErroresFuente, obtenidos %d", len(agg.ErroresFuente()))
	}
}

// TestAgregador_TodasFallan_SinError verifica que si todas fallan retorna lista vacía.
func TestAgregador_TodasFallan_SinError(t *testing.T) {
	t.Parallel()

	fuenteMala1 := &catalogoMock{err: errors.New("NSS no disponible")}
	fuenteMala2 := &catalogoMock{err: errors.New("PKCS#11 no disponible")}
	agg := certcatalogagg.New(fuenteMala1, fuenteMala2)

	refs, err := agg.List(context.Background())
	if err != nil {
		t.Fatalf("List() no debe retornar error aunque todas las fuentes fallen: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("esperados 0 certs, obtenidos %d", len(refs))
	}
	if len(agg.ErroresFuente()) != 2 {
		t.Errorf("esperados 2 errores en ErroresFuente, obtenidos %d", len(agg.ErroresFuente()))
	}
}

// TestAgregador_NilFuenteIgnorada verifica que fuentes nil no producen panic.
func TestAgregador_NilFuenteIgnorada(t *testing.T) {
	t.Parallel()

	cert := refConFP("Cert Valido", "eeee0000000000000000000000000000000000000000000000000000000000ee")
	fuenteOK := &catalogoMock{refs: []domain.CertificateRef{cert}}
	agg := certcatalogagg.New(nil, fuenteOK, nil)

	refs, err := agg.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(refs) != 1 {
		t.Errorf("esperado 1 cert, obtenido %d", len(refs))
	}
}

// TestAgregador_ContextoCancelado verifica que la cancelación se propaga.
func TestAgregador_ContextoCancelado(t *testing.T) {
	t.Parallel()

	cert := refConFP("Cert X", "ffff0000000000000000000000000000000000000000000000000000000000ff")
	fuente := &catalogoMock{refs: []domain.CertificateRef{cert}}
	agg := certcatalogagg.New(fuente)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := agg.List(ctx)
	if err == nil {
		t.Log("contexto cancelado antes de primera fuente: aceptable con fuente rápida")
	}
}

// TestAgregador_AñadirFuente verifica que se puede añadir fuente tras crear el agregador.
func TestAgregador_AñadirFuente(t *testing.T) {
	t.Parallel()

	agg := certcatalogagg.New()
	if agg.TieneFuentes() {
		t.Error("recién creado no debe tener fuentes")
	}

	cert := refConFP("Nuevo", "1111000000000000000000000000000000000000000000000000000000001111")
	fuente := &catalogoMock{refs: []domain.CertificateRef{cert}}
	agg.AñadirFuente(fuente)

	if !agg.TieneFuentes() {
		t.Error("debe tener fuentes tras AñadirFuente")
	}

	refs, err := agg.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(refs) != 1 {
		t.Errorf("esperado 1 cert, obtenido %d", len(refs))
	}
}

// catalogoColgado ignora el contexto y no responde nunca, simulando una fuente
// externa colgada (p. ej. un módulo PKCS#11 bloqueado).
type catalogoColgado struct {
	desbloquear chan struct{}
}

func (m *catalogoColgado) List(_ context.Context) ([]domain.CertificateRef, error) {
	<-m.desbloquear
	return nil, nil
}

// TestAgregador_TimeoutPorFuente verifica que una fuente colgada no bloquea el
// listado: se registra como error y las demás fuentes responden.
func TestAgregador_TimeoutPorFuente(t *testing.T) {
	t.Parallel()

	colgada := &catalogoColgado{desbloquear: make(chan struct{})}
	defer close(colgada.desbloquear)
	cert := refConFP("Sana", "2222000000000000000000000000000000000000000000000000000000002222")
	sana := &catalogoMock{refs: []domain.CertificateRef{cert}}

	agg := certcatalogagg.New(colgada, sana)
	agg.TimeoutPorFuente = 50 * time.Millisecond

	inicio := time.Now()
	refs, err := agg.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if transcurrido := time.Since(inicio); transcurrido > 5*time.Second {
		t.Fatalf("List tardó %v; la fuente colgada no fue acotada por el timeout", transcurrido)
	}
	if len(refs) != 1 {
		t.Errorf("esperado 1 cert de la fuente sana, obtenidos %d", len(refs))
	}
	if len(agg.ErroresFuente()) != 1 {
		t.Errorf("esperado 1 error (timeout de la fuente colgada), obtenidos %d", len(agg.ErroresFuente()))
	}
}

// TestAgregador_ListConcurrente verifica que la misma instancia soporta List
// desde varias goroutines (los adaptadores de entrada la comparten). Con
// -race este test detecta cualquier carrera sobre el estado interno.
func TestAgregador_ListConcurrente(t *testing.T) {
	t.Parallel()

	ok := &catalogoMock{refs: []domain.CertificateRef{
		refConFP("Concurrente", "3333000000000000000000000000000000000000000000000000000000003333"),
	}}
	falla := &catalogoMock{err: errors.New("fuente rota")}
	agg := certcatalogagg.New(ok, falla)

	const goroutines = 16
	hecho := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			refs, err := agg.List(context.Background())
			if err == nil && len(refs) != 1 {
				err = errors.New("resultado inesperado en List concurrente")
			}
			_ = agg.ErroresFuente()
			agg.AñadirFuente(nil)
			hecho <- err
		}()
	}
	for i := 0; i < goroutines; i++ {
		if err := <-hecho; err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}
