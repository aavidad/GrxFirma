// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type canonicalizadorPruebaLocal struct {
	salida []byte
	err    error
}

func (c canonicalizadorPruebaLocal) Canonicalizar(domain.SolicitudRetoIdentidad) ([]byte, error) {
	return append([]byte(nil), c.salida...), c.err
}

type catalogoPruebaLocal struct {
	certificados []domain.CertificateRef
	err          error
	llamadas     int
}

func (c *catalogoPruebaLocal) List(context.Context) ([]domain.CertificateRef, error) {
	c.llamadas++
	return append([]domain.CertificateRef(nil), c.certificados...), c.err
}

type selectorPruebaLocal struct {
	seleccionado domain.CertificateRef
	err          error
	contexto     ports.ContextoSeleccionIdentidad
	candidatos   []domain.CertificateRef
}

func (s *selectorPruebaLocal) Seleccionar(
	_ context.Context,
	contexto ports.ContextoSeleccionIdentidad,
	certificados []domain.CertificateRef,
) (domain.CertificateRef, error) {
	s.contexto = contexto
	s.candidatos = append([]domain.CertificateRef(nil), certificados...)
	return s.seleccionado, s.err
}

type firmadorPruebaLocal struct {
	resultado SignResult
	err       error
	comando   SignCommand
	llamadas  int
}

func (f *firmadorPruebaLocal) Execute(_ context.Context, comando SignCommand) (SignResult, error) {
	f.llamadas++
	f.comando = comando
	return f.resultado, f.err
}

type relojPruebaLocal struct{ ahora time.Time }

func (r relojPruebaLocal) Now() time.Time { return r.ahora }

func TestGeneradorPruebaIdentidadLocalFirmaCanonConSeleccionPrivada(t *testing.T) {
	reto, ahora := retoPruebaIdentidadLocal()
	certificado := certificadoPruebaIdentidadLocal("certificado-1", ahora.Add(time.Hour))
	selector := &selectorPruebaLocal{seleccionado: certificado}
	firmador := &firmadorPruebaLocal{resultado: resultadoFirmaIdentidadLocal("SHA256withRSA")}
	generador := nuevoGeneradorPruebaLocal(t, reto, ahora,
		[]domain.CertificateRef{certificado}, selector, firmador)

	prueba, err := generador.Generar(context.Background(), reto)
	if err != nil {
		t.Fatalf("generar: %v", err)
	}
	if prueba.RetoID != reto.Solicitud.RetoID || prueba.AlgoritmoFirma != "sha256-rsa-pkcs1v15" ||
		string(prueba.Certificado) != "certificado-hoja" || len(prueba.Cadena) != 1 {
		t.Fatalf("prueba inesperada: %+v", prueba)
	}
	if selector.contexto.Origen != reto.Solicitud.Origen ||
		selector.contexto.Finalidad != reto.Solicitud.Finalidad || len(selector.candidatos) != 1 {
		t.Fatalf("contexto local incompleto: %+v", selector)
	}
	if firmador.comando.CertificateID != certificado.ID ||
		!bytes.Equal(firmador.comando.Document.Content, reto.ContenidoCanonico) ||
		firmador.comando.RequesterOrigin != reto.Solicitud.Origen ||
		firmador.comando.Format != domain.FormatCAdES {
		t.Fatalf("comando inseguro: %+v", firmador.comando)
	}
	prueba.Certificado[0] = 'X'
	if string(firmador.resultado.CertificateChainDER[0]) != "certificado-hoja" {
		t.Fatal("la prueba comparte memoria con el firmador")
	}
}

func TestGeneradorPruebaIdentidadLocalRechazaCanonAlteradoAntesDelCatalogo(t *testing.T) {
	reto, ahora := retoPruebaIdentidadLocal()
	catalogo := &catalogoPruebaLocal{}
	selector := &selectorPruebaLocal{}
	firmador := &firmadorPruebaLocal{}
	generador, _ := NuevoGeneradorPruebaIdentidadLocal(
		canonicalizadorPruebaLocal{salida: []byte("otro-canon")}, catalogo,
		selector, firmador, relojPruebaLocal{ahora},
	)
	_, err := generador.Generar(context.Background(), reto)
	if !errors.Is(err, ErrPruebaIdentidadLocalInvalida) || catalogo.llamadas != 0 || firmador.llamadas != 0 {
		t.Fatalf("alteración no cerrada: error=%v catálogo=%d firmas=%d", err, catalogo.llamadas, firmador.llamadas)
	}
}

func TestGeneradorPruebaIdentidadLocalFiltraCaducadosYNoAceptaSeleccionInyectada(t *testing.T) {
	reto, ahora := retoPruebaIdentidadLocal()
	vigente := certificadoPruebaIdentidadLocal("vigente", ahora.Add(time.Hour))
	caducado := certificadoPruebaIdentidadLocal("caducado", ahora.Add(-time.Second))
	selector := &selectorPruebaLocal{seleccionado: caducado}
	firmador := &firmadorPruebaLocal{}
	generador := nuevoGeneradorPruebaLocal(t, reto, ahora,
		[]domain.CertificateRef{caducado, vigente}, selector, firmador)

	_, err := generador.Generar(context.Background(), reto)
	if !errors.Is(err, ErrPruebaIdentidadLocalNoDisponible) || len(selector.candidatos) != 1 ||
		selector.candidatos[0].ID != vigente.ID || firmador.llamadas != 0 {
		t.Fatalf("selección inyectada no bloqueada: error=%v candidatos=%+v", err, selector.candidatos)
	}
}

func TestGeneradorPruebaIdentidadLocalRechazaCaducidadYAlgoritmoLibre(t *testing.T) {
	reto, ahora := retoPruebaIdentidadLocal()
	certificado := certificadoPruebaIdentidadLocal("certificado-1", ahora.Add(time.Hour))
	selector := &selectorPruebaLocal{seleccionado: certificado}
	firmador := &firmadorPruebaLocal{resultado: resultadoFirmaIdentidadLocal("SHA512withRSA")}
	generador := nuevoGeneradorPruebaLocal(t, reto, ahora,
		[]domain.CertificateRef{certificado}, selector, firmador)

	_, err := generador.Generar(context.Background(), reto)
	if !errors.Is(err, ErrPruebaIdentidadLocalNoDisponible) {
		t.Fatalf("algoritmo libre admitido: %v", err)
	}
	reto.Solicitud.ExpiraEn = ahora
	_, err = generador.Generar(context.Background(), reto)
	if !errors.Is(err, ErrPruebaIdentidadLocalInvalida) {
		t.Fatalf("reto caducado admitido: %v", err)
	}
}

func nuevoGeneradorPruebaLocal(
	t *testing.T,
	reto domain.RetoIdentidad,
	ahora time.Time,
	certificados []domain.CertificateRef,
	selector *selectorPruebaLocal,
	firmador *firmadorPruebaLocal,
) *GeneradorPruebaIdentidadLocal {
	t.Helper()
	generador, err := NuevoGeneradorPruebaIdentidadLocal(
		canonicalizadorPruebaLocal{salida: reto.ContenidoCanonico},
		&catalogoPruebaLocal{certificados: certificados}, selector, firmador,
		relojPruebaLocal{ahora},
	)
	if err != nil {
		t.Fatalf("construir: %v", err)
	}
	return generador
}

func retoPruebaIdentidadLocal() (domain.RetoIdentidad, time.Time) {
	ahora := time.Date(2026, 8, 1, 17, 0, 0, 0, time.UTC)
	solicitud := solicitudIdentidadAplicacion()
	solicitud.EmitidoEn = ahora.Add(-time.Second)
	solicitud.ExpiraEn = ahora.Add(time.Minute)
	return domain.RetoIdentidad{Solicitud: solicitud, ContenidoCanonico: []byte(`{"canon":"opaco"}`)}, ahora
}

func certificadoPruebaIdentidadLocal(id string, caducidad time.Time) domain.CertificateRef {
	return domain.CertificateRef{ID: id, Subject: "CN=Persona de prueba", Issuer: "CN=AC de prueba",
		Fingerprint: "sha256-" + id, NotAfter: caducidad}
}

func resultadoFirmaIdentidadLocal(algoritmo string) SignResult {
	return SignResult{Result: domain.SignatureResult{Format: domain.FormatCAdES,
		Data: []byte("firma-cades"), Algorithm: algoritmo}, CertificateChainDER: [][]byte{
		[]byte("certificado-hoja"), []byte("certificado-emisor"),
	}}
}
