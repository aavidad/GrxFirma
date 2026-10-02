// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

var errSeleccionIdentidadCancelada = errors.New("selección local de certificado cancelada")

type funcionSeleccionIdentidad func(context.Context, string, string, string, string, []string) (int, error)

type localizadorSeleccionIdentidad interface {
	T(id string, argumentos ...any) string
}

type selectorCertificadoIdentidadSistema struct {
	seleccionar funcionSeleccionIdentidad
	localizador localizadorSeleccionIdentidad
}

func nuevoSelectorCertificadoIdentidadSistema(
	localizador localizadorSeleccionIdentidad,
) *selectorCertificadoIdentidadSistema {
	return &selectorCertificadoIdentidadSistema{
		seleccionar: seleccionarIndiceCertificadoSistema,
		localizador: localizador,
	}
}

// Seleccionar muestra el catálogo sólo en una ventana del equipo y devuelve una copia.
func (s *selectorCertificadoIdentidadSistema) Seleccionar(
	ctx context.Context,
	contexto ports.ContextoSeleccionIdentidad,
	certificados []domain.CertificateRef,
) (domain.CertificateRef, error) {
	if ctx == nil || s == nil || s.seleccionar == nil || len(certificados) == 0 {
		return domain.CertificateRef{}, errors.New("no hay certificados locales seleccionables")
	}
	opciones := make([]string, len(certificados))
	for indice, certificado := range certificados {
		opciones[indice] = s.etiquetaCertificado(indice, certificado)
	}
	titulo := s.texto("identity.local.selector.title", "GrxFirma — Identidad reforzada")
	prompt := s.texto(
		"identity.local.selector.prompt",
		"Origen: %s\nFinalidad: %s\nOperación: %s\n\nSeleccione el certificado que desea usar.",
		sanitizarTextoSelector(contexto.Origen, 256),
		sanitizarTextoSelector(contexto.Finalidad, 256),
		sanitizarTextoSelector(contexto.Operacion, 256),
	)
	usar := s.texto("identity.local.selector.use", "Usar certificado")
	cancelar := s.texto("identity.local.selector.cancel", "Cancelar")
	indice, err := s.seleccionar(ctx, titulo, prompt, usar, cancelar, opciones)
	if err != nil {
		return domain.CertificateRef{}, err
	}
	if indice < 0 || indice >= len(certificados) {
		return domain.CertificateRef{}, errors.New("selección local de certificado inválida")
	}
	return certificados[indice], nil
}

func (s *selectorCertificadoIdentidadSistema) etiquetaCertificado(
	indice int,
	certificado domain.CertificateRef,
) string {
	caducidad := s.texto("identity.local.selector.no_expiry", "sin caducidad disponible")
	if !certificado.NotAfter.IsZero() {
		caducidad = certificado.NotAfter.UTC().Format(time.DateOnly)
	}
	return s.texto(
		"identity.local.selector.certificate",
		"%d · %s · Emisor: %s · Válido hasta: %s · Huella: %s",
		indice+1,
		sanitizarTextoSelector(certificado.Subject, 180),
		sanitizarTextoSelector(certificado.Issuer, 180),
		caducidad,
		sanitizarTextoSelector(certificado.Fingerprint, 128),
	)
}

func (s *selectorCertificadoIdentidadSistema) texto(
	clave string,
	reserva string,
	argumentos ...any,
) string {
	if s != nil && s.localizador != nil {
		traducido := s.localizador.T(clave, argumentos...)
		if traducido != clave {
			return traducido
		}
	}
	return fmt.Sprintf(reserva, argumentos...)
}

func sanitizarTextoSelector(valor string, maximo int) string {
	runas := make([]rune, 0, min(len(valor), maximo))
	for _, caracter := range strings.TrimSpace(valor) {
		if len(runas) >= maximo {
			break
		}
		if unicode.IsControl(caracter) {
			runas = append(runas, ' ')
			continue
		}
		runas = append(runas, caracter)
	}
	return strings.Join(strings.Fields(string(runas)), " ")
}

var _ ports.SelectorCertificadoIdentidad = (*selectorCertificadoIdentidadSistema)(nil)
