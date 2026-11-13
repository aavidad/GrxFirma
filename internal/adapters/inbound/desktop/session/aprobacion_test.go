// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package session_test

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"testing"

	"grxfirma/internal/adapters/inbound/common/aprobacion"
	"grxfirma/internal/adapters/inbound/desktop/session"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/presentation/desktop/certpicker"
)

type aprobadorPrueba struct{}

func (aprobadorPrueba) Request(context.Context, string) (bool, error) { return true, nil }

func componerAprobacionPrueba(d application.DetalleAprobacion) string {
	return aprobacion.Componer(localizador.Para("es"), d)
}

// aprobadorRegistro guarda el mensaje y el orden respecto al selector.
type aprobadorRegistro struct {
	mu       sync.Mutex
	aprobado bool
	mensajes []string
	eventos  *[]string
}

func (a *aprobadorRegistro) Request(_ context.Context, mensaje string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mensajes = append(a.mensajes, mensaje)
	if a.eventos != nil {
		*a.eventos = append(*a.eventos, "aprobacion")
	}
	return a.aprobado, nil
}

func orquestadorConAprobacion(eventos *[]string, ejecutado *bool, contexto *certpicker.ContextoSolicitud) *session.Orchestrator {
	return session.New(session.Config{
		ComponerAprobacion: componerAprobacionPrueba,
		CertCatalog: &mockCertCatalog{listFn: func(context.Context) ([]domain.CertificateRef, error) {
			c := certRef("c1")
			c.Subject = "CN=Ana Prueba"
			return []domain.CertificateRef{c}, nil
		}},
		KeyProvider: &mockKeyProvider{},
		CertSelector: &mockCertSelector{selectFn: func(ctx context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
			*eventos = append(*eventos, "selector")
			if c, ok := certpicker.ContextoSolicitudDe(ctx); ok && contexto != nil {
				*contexto = c
			}
			return certpicker.ResultadoSeleccion{Certificado: certs[0]}, nil
		}},
		TriphaseExec: &mockSimpleExecutor{executeFn: func(context.Context, domain.ExchangeSession, domain.SignatureJob) (domain.SignatureResult, error) {
			*ejecutado = true
			return domain.SignatureResult{}, nil
		}},
		Progress: &mockProgressProvider{},
		Notify:   &mockDesktopNotification{},
	})
}

// A-02: en afirma:// directo la aprobación llega después del selector y dice
// el formato, el tamaño del documento y el servidor que recibe el resultado.
func TestOrchestrator_AprobacionDespuesDelSelectorConDetalle(t *testing.T) {
	adaptador := afirmauri.New(nil)
	dat := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 3000)))
	solicitud, err := adaptador.Parse(context.Background(),
		"afirma://sign?id=1&format=PAdES&stservlet=https%3A%2F%2Fa.gob.es%2FStorageService&dat="+dat)
	if err != nil {
		t.Fatal(err)
	}
	var eventos []string
	var ejecutado bool
	var contexto certpicker.ContextoSolicitud
	aprobador := &aprobadorRegistro{aprobado: true, eventos: &eventos}
	if err := orquestadorConAprobacion(&eventos, &ejecutado, &contexto).HandleRequestConAprobacion(context.Background(), solicitud, aprobador); err != nil {
		t.Fatalf("HandleRequestConAprobacion: %v", err)
	}
	if len(aprobador.mensajes) != 1 || strings.Join(eventos, ",") != "selector,aprobacion" {
		t.Fatalf("orden = %v, mensajes = %d; se esperaba selector y después una aprobación", eventos, len(aprobador.mensajes))
	}
	mensaje := aprobador.mensajes[0]
	for _, esperado := range []string{"https://a.gob.es", "PAdES", "3 KB", "CN=Ana Prueba", localizador.Para("es").T("approval.origin.unverifiable")} {
		if !strings.Contains(mensaje, esperado) {
			t.Fatalf("el mensaje no contiene %q:\n%s", esperado, mensaje)
		}
	}
	if !contexto.OrigenEsDestino || contexto.Origen != "https://a.gob.es" {
		t.Fatalf("el selector debía mostrar el dominio como destino del resultado: %+v", contexto)
	}
	if !ejecutado {
		t.Fatal("tras aprobar debía ejecutarse la firma")
	}
}

func TestOrchestrator_RechazoDeAprobacionNoFirma(t *testing.T) {
	solicitud := solicitudSimpleValida()
	solicitud.SignCommand = &application.SignCommand{Document: mustDocument(t, "d.bin", []byte("d")), Format: domain.FormatCAdES, Action: domain.ActionSign}
	var eventos []string
	var ejecutado bool
	err := orquestadorConAprobacion(&eventos, &ejecutado, nil).HandleRequestConAprobacion(context.Background(), solicitud, &aprobadorRegistro{aprobado: false})
	if !errors.Is(err, session.ErrAprobacionCancelada) || ejecutado {
		t.Fatalf("err = %v, ejecutado = %v; se esperaba cancelación sin firmar", err, ejecutado)
	}
	// Sin aprobador se falla cerrado.
	if err := orquestadorConAprobacion(&eventos, &ejecutado, nil).HandleRequestConAprobacion(context.Background(), solicitud, nil); err == nil || ejecutado {
		t.Fatalf("sin aprobador: err = %v, ejecutado = %v", err, ejecutado)
	}
}

func TestOrchestrator_SelectCertNoPideAprobacion(t *testing.T) {
	solicitud := solicitudSimpleValida()
	solicitud.Operacion = afirmauri.OperacionSelectCert
	var eventos []string
	var ejecutado bool
	aprobador := &aprobadorRegistro{aprobado: false}
	_ = orquestadorConAprobacion(&eventos, &ejecutado, nil).HandleRequestConAprobacion(context.Background(), solicitud, aprobador)
	if len(aprobador.mensajes) != 0 {
		t.Fatalf("selectcert no firma y no debe pedir aprobación: %v", aprobador.mensajes)
	}
}
