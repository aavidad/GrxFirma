// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"context"
	"errors"

	"grxfirma/internal/domain"
)

// AdaptiveProtector delega entre el sobre JSON nativo y el contenedor CMS
// interoperable sin exponer detalles de protocolo a la aplicación.
type AdaptiveProtector struct {
	JSON *EnvelopeProtector
	CMS  *CMSProtector
}

func NuevoAdaptiveProtector() *AdaptiveProtector {
	return &AdaptiveProtector{
		JSON: NuevoEnvelopeProtector(),
		CMS:  NuevoCMSProtector(),
	}
}

func (p *AdaptiveProtector) Protect(ctx context.Context, job domain.ProtectionJob, recipients []domain.ProtectionRecipient) (domain.ProtectedPayload, error) {
	contentType, err := requestedCMSContentType(job.Options)
	if err != nil {
		return domain.ProtectedPayload{}, err
	}
	switch {
	case contentType != cmsContentTypeNone:
		if p == nil || p.CMS == nil {
			return domain.ProtectedPayload{}, errors.New("protector CMS no configurado")
		}
		return p.CMS.Protect(ctx, job, recipients)
	default:
		if p == nil || p.JSON == nil {
			return domain.ProtectedPayload{}, errors.New("protector JSON no configurado")
		}
		return p.JSON.Protect(ctx, job, recipients)
	}
}

func (p *AdaptiveProtector) Unprotect(ctx context.Context, protected domain.Document, keys []domain.ProtectionKeyMaterial) (domain.UnprotectedPayload, error) {
	if looksLikeJSONEnvelope(protected.Content) {
		if p == nil || p.JSON == nil {
			return domain.UnprotectedPayload{}, errors.New("protector JSON no configurado")
		}
		return p.JSON.Unprotect(ctx, protected, keys)
	}
	if p == nil || p.CMS == nil {
		return domain.UnprotectedPayload{}, errors.New("protector CMS no configurado")
	}
	return p.CMS.Unprotect(ctx, protected, keys)
}
