// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// SelectCertificateUseCase resuelve la seleccion de un certificado a partir del catalogo disponible.
// En esta fase, si existen varios candidatos, solicita confirmacion para usar el primero compatible.
type SelectCertificateUseCase struct {
	catalogo  ports.CertificateCatalog
	aprobador ports.UserApproval
	eventos   ports.EventPublisher
	reloj     ports.Clock
}

// NuevoSelectCertificateUseCase construye el caso de uso de seleccion de certificado.
func NuevoSelectCertificateUseCase(
	catalogo ports.CertificateCatalog,
	aprobador ports.UserApproval,
	eventos ports.EventPublisher,
	reloj ports.Clock,
) *SelectCertificateUseCase {
	return &SelectCertificateUseCase{
		catalogo:  catalogo,
		aprobador: aprobador,
		eventos:   eventos,
		reloj:     reloj,
	}
}

// Ejecutar filtra el catalogo y devuelve el certificado seleccionado.
func (uc *SelectCertificateUseCase) Ejecutar(ctx context.Context, cmd SelectCertificateCommand) (SelectCertificateResult, error) {
	certs, err := uc.catalogo.List(ctx)
	if err != nil {
		return SelectCertificateResult{}, fmt.Errorf("no se pudo obtener el catalogo de certificados: %w", err)
	}
	candidatos := uc.filtrar(certs, cmd)
	if len(candidatos) == 0 {
		return SelectCertificateResult{}, errors.New("no hay certificados compatibles con los filtros solicitados")
	}
	if len(candidatos) == 1 {
		return SelectCertificateResult{
			Selection: domain.CertificateSelection{
				Certificate: candidatos[0],
				Confirmed:   true,
			},
		}, nil
	}

	_ = uc.publicarEvento(ctx, candidatos)

	if uc.aprobador == nil {
		return SelectCertificateResult{}, errors.New("hay varios certificados compatibles y no hay mecanismo de aprobacion configurado")
	}

	elegido := candidatos[0]
	ok, err := uc.aprobador.Request(ctx, fmt.Sprintf(
		"Se han encontrado %d certificados compatibles. ¿Desea usar el primero (%s)?",
		len(candidatos), elegido.Subject,
	))
	if err != nil {
		return SelectCertificateResult{}, fmt.Errorf("no se pudo solicitar la aprobacion de la seleccion: %w", err)
	}
	if !ok {
		return SelectCertificateResult{}, errors.New("el usuario ha cancelado la seleccion del certificado")
	}

	return SelectCertificateResult{
		Selection: domain.CertificateSelection{
			Certificate: elegido,
			Confirmed:   true,
		},
	}, nil
}

// Execute expone el caso de uso con el nombre neutro esperado por los adaptadores.
func (uc *SelectCertificateUseCase) Execute(ctx context.Context, cmd SelectCertificateCommand) (SelectCertificateResult, error) {
	return uc.Ejecutar(ctx, cmd)
}

func (uc *SelectCertificateUseCase) filtrar(certs []domain.CertificateRef, cmd SelectCertificateCommand) []domain.CertificateRef {
	now := uc.now()
	sujetof := strings.ToLower(strings.TrimSpace(cmd.SubjectFilter))
	emisorf := strings.ToLower(strings.TrimSpace(cmd.IssuerFilter))

	out := make([]domain.CertificateRef, 0, len(certs))
	for _, cert := range certs {
		if sujetof != "" && !strings.Contains(strings.ToLower(cert.Subject), sujetof) {
			continue
		}
		if emisorf != "" && !strings.Contains(strings.ToLower(cert.Issuer), emisorf) {
			continue
		}
		if cmd.SoloNoCaducados && cert.IsExpired(now) {
			continue
		}
		out = append(out, cert)
	}
	return out
}

func (uc *SelectCertificateUseCase) now() time.Time {
	if uc != nil && uc.reloj != nil {
		return uc.reloj.Now()
	}
	return time.Now()
}

func (uc *SelectCertificateUseCase) publicarEvento(ctx context.Context, candidatos []domain.CertificateRef) error {
	if uc == nil || uc.eventos == nil {
		return nil
	}
	return uc.eventos.Publish(ctx, ports.Event{
		Type:    "seleccion_certificado_requerida",
		Payload: []byte(fmt.Sprintf("candidatos=%d", len(candidatos))),
	})
}
