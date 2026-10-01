// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Implementacion base de XAdES y PAdES en Go nativo.
// XAdES se genera como firma XMLDSig/XAdES-BES de tipo enveloping.
// PAdES usa una actualización incremental del PDF original mediante pdfsign.
package signer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	pdfsign "github.com/digitorus/pdfsign/sign"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/common/tsaclient"
	"grxfirma/internal/domain"
)

func (m *MotorFirmaGo) firmarXAdES(ctx context.Context, job domain.SignatureJob, clave *ClaveLocal, ahora time.Time) (domain.SignatureResult, error) {
	_ = ahora
	return commonsigner.NewXAdESBESDetached().Sign(ctx, job, clave.ToLocalSigningKey())
}

func (m *MotorFirmaGo) firmarXMLDSig(ctx context.Context, job domain.SignatureJob, clave *ClaveLocal, ahora time.Time) (domain.SignatureResult, error) {
	_ = ahora
	return commonsigner.NewXMLDSigDetached().Sign(ctx, job, clave.ToLocalSigningKey())
}

// firmarPAdES firma PDFs reales preservando el documento original. Una entrada
// inválida se rechaza: nunca se sustituye silenciosamente por un PDF nuevo.
func (m *MotorFirmaGo) firmarPAdES(ctx context.Context, job domain.SignatureJob, clave *ClaveLocal, ahora time.Time) (domain.SignatureResult, error) {
	_ = ahora
	if err := ctx.Err(); err != nil {
		return domain.SignatureResult{}, err
	}
	if clave == nil || clave.cert == nil || clave.priv == nil {
		return domain.SignatureResult{}, errors.New("PAdES-Basic: clave local incompleta")
	}
	if err := comprobarPDFCifrado(job.Document.Content, job.Options); err != nil {
		return domain.SignatureResult{}, fmt.Errorf("PAdES: %w", err)
	}
	if valorBoolOpcionPAdES(job.Options, "allowInvalidPDF", false) {
		return domain.SignatureResult{}, errors.New("PAdES-Basic: allowInvalidPDF ya no está soportado; un PDF inválido se rechaza para preservar siempre el documento original")
	}

	tsaURL := m.resolverTSAURL(job.Options)
	if solicitaPerfilT(job.Options) {
		tsa := m.resolverTSA(job.Options)
		if tsa == nil {
			return domain.SignatureResult{}, errors.New("PAdES-B-T requiere una TSA configurada")
		}
		if tsaURL == "" {
			return domain.SignatureResult{}, errors.New("PAdES-B-T requiere una TSA RFC 3161 configurada mediante URL; no se generará un PDF alternativo")
		}
	}

	var validation pdfsign.ValidationData
	if solicitaLTVPAdES(job.Options) {
		var err error
		validation, err = m.construirValidationDataPAdES(ctx, clave)
		if err != nil {
			return domain.SignatureResult{}, fmt.Errorf("PAdES-LTV: %w", err)
		}
	}
	tsaConfig := pdfsign.TSA{URL: tsaURL, Context: ctx}
	if client, ok := m.resolverTSA(job.Options).(*tsaclient.Client); ok && client != nil {
		tsaConfig.HTTPClient = client.HTTPClient
	}
	resultado, err := firmarPAdESConPdfsign(job.Document, clave, job.Options, validation, tsaConfig)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("PAdES-Basic: el PDF de entrada no es válido o no puede firmarse sin sustituir su contenido: %w", err)
	}
	return resultado, nil
}

func resolverSubFilterPAdES(options map[string]string) string {
	for _, clave := range []string{"subfilter", "pdfsubfilter"} {
		valor := strings.TrimSpace(valorOpcion(options, clave))
		switch valor {
		case "adbe.pkcs7.detached", "/adbe.pkcs7.detached", "adobe":
			return "adbe.pkcs7.detached"
		case "ETSI.CAdES.detached", "/ETSI.CAdES.detached", "etsi", "":
		}
	}
	return "ETSI.CAdES.detached"
}
