// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/common/tsaclient"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
)

// MotorFirmaGo implementa ports.SignerEngine usando adaptadores en Go puro.
// Mantiene CAdES-BES detached y anade XAdES-BES detached con C14N exclusiva
// y PAdES detached sin recurrir a exec.Command ni librerias C.
type MotorFirmaGo struct {
	reloj      ports.Clock
	tsa        ports.TimestampAuthority
	revocation ports.RevocationProvider
}

// NuevoMotorFirmaGo crea un MotorFirmaGo.
func NuevoMotorFirmaGo(reloj ports.Clock) *MotorFirmaGo {
	return &MotorFirmaGo{reloj: reloj}
}

// WithTimestampAuthority inyecta una TSA para soportar perfiles T cuando el
// trabajo de firma lo solicita explícitamente mediante Options.
func (m *MotorFirmaGo) WithTimestampAuthority(tsa ports.TimestampAuthority) *MotorFirmaGo {
	if m == nil {
		return nil
	}
	m.tsa = tsa
	return m
}

// WithRevocationProvider inyecta un proveedor de OCSP/CRL para perfiles LT/LTA.
func (m *MotorFirmaGo) WithRevocationProvider(provider ports.RevocationProvider) *MotorFirmaGo {
	if m == nil {
		return nil
	}
	m.revocation = provider
	return m
}

// Sign ejecuta la operacion de firma sobre el trabajo indicado.
// La clave debe ser de tipo *ClaveLocal; de lo contrario se devuelve error.
func (m *MotorFirmaGo) Sign(ctx context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.SignatureResult{}, err
	}

	if err := job.Validate(); err != nil {
		return domain.SignatureResult{}, fmt.Errorf("trabajo de firma invalido: %w", err)
	}

	clave, ok := key.(*ClaveLocal)
	if !ok {
		return domain.SignatureResult{}, errors.New("el motor de firma nativo solo acepta ClaveLocal")
	}

	ahora := time.Now().UTC()
	if m.reloj != nil {
		ahora = m.reloj.Now().UTC()
	}
	if err := clave.validateSigningIdentity(ahora); err != nil {
		return domain.SignatureResult{}, err
	}

	switch job.Format {
	case domain.FormatCAdES:
		return m.firmarCaDES(ctx, job, clave)
	case domain.FormatXAdES:
		if solicitaPerfilT(job.Options) {
			tsa := m.resolverTSA(job.Options)
			if tsa == nil {
				return domain.SignatureResult{}, errors.New("XAdES-T requiere una TSA configurada")
			}
			return commonsigner.NewSignerXAdEST(commonsigner.NewXAdESBESDetached(), tsa).Sign(ctx, job, clave.ToLocalSigningKey())
		}
		return m.firmarXAdES(ctx, job, clave, ahora)
	case domain.SignatureFormat("XMLdSig"):
		return m.firmarXMLDSig(ctx, job, clave, ahora)
	case commonsigner.FormatVeriFactu:
		return commonsigner.NewVeriFactuSigner().Sign(ctx, job, clave.ToLocalSigningKey())
	case domain.SignatureFormat("FacturaE"):
		return commonsigner.NewFacturaESigner().Sign(ctx, job, clave.ToLocalSigningKey())
	case domain.SignatureFormat("ASiC-XAdES"):
		return commonsigner.NewASiCXAdESSigner().Sign(ctx, job, clave.ToLocalSigningKey())
	case domain.SignatureFormat("PKCS1"):
		return commonsigner.NewPKCS1Signer().Sign(ctx, job, clave.ToLocalSigningKey())
	case domain.SignatureFormat("ASiC-CAdES"):
		return commonsigner.NewASiCCAdESSigner().Sign(ctx, job, clave.ToLocalSigningKey())
	case domain.SignatureFormat("ODF"):
		return commonsigner.NewODFDetached().Sign(ctx, job, clave.ToLocalSigningKey())
	case domain.SignatureFormat("OOXML"):
		return commonsigner.NewOOXMLDetached().Sign(ctx, job, clave.ToLocalSigningKey())
	case domain.FormatPAdES:
		return m.firmarPAdES(ctx, job, clave, ahora)
	default:
		return domain.SignatureResult{}, fmt.Errorf("formato no soportado: %s", job.Format)
	}
}

func (m *MotorFirmaGo) firmarCaDES(ctx context.Context, job domain.SignatureJob, clave *ClaveLocal) (domain.SignatureResult, error) {
	base := commonsigner.NewCAdESBESDetached()
	if solicitaPerfilLTA(job.Options) {
		tsa := m.resolverTSA(job.Options)
		if tsa == nil {
			return domain.SignatureResult{}, errors.New("CAdES-LTA requiere una TSA configurada")
		}
		if m.revocation == nil {
			return domain.SignatureResult{}, errors.New("CAdES-LTA requiere un proveedor de revocación configurado")
		}
		return commonsigner.NewSignerCAdESLTA(
			commonsigner.NewSignerCAdESLT(
				commonsigner.NewSignerCAdEST(base, tsa),
				m.revocation,
			),
			tsa,
		).Sign(ctx, job, clave.ToLocalSigningKey())
	}
	if solicitaPerfilLT(job.Options) {
		tsa := m.resolverTSA(job.Options)
		if tsa == nil {
			return domain.SignatureResult{}, errors.New("CAdES-LT requiere una TSA configurada")
		}
		if m.revocation == nil {
			return domain.SignatureResult{}, errors.New("CAdES-LT requiere un proveedor de revocación configurado")
		}
		return commonsigner.NewSignerCAdESLT(
			commonsigner.NewSignerCAdEST(base, tsa),
			m.revocation,
		).Sign(ctx, job, clave.ToLocalSigningKey())
	}
	if solicitaPerfilT(job.Options) {
		tsa := m.resolverTSA(job.Options)
		if tsa == nil {
			return domain.SignatureResult{}, errors.New("CAdES-T requiere una TSA configurada")
		}
		return commonsigner.NewSignerCAdEST(base, tsa).Sign(ctx, job, clave.ToLocalSigningKey())
	}
	return base.Sign(ctx, job, clave.ToLocalSigningKey())
}

func solicitaPerfilT(options map[string]string) bool {
	if len(options) == 0 {
		return false
	}
	for _, clave := range []string{"nivel", "level", "profile", "baseline"} {
		valor := strings.ToLower(strings.TrimSpace(valorOpcion(options, clave)))
		switch valor {
		case "t", "b-t", "baseline-t", "xades-t", "pades-t", "cades-t", "timestamp":
			return true
		case "lt", "b-lt", "baseline-lt", "cades-lt", "lta", "b-lta", "baseline-lta", "cades-lta":
			return true
		case "baseline":
			if solicitaTSA(options) {
				return true
			}
		}
	}
	return false
}

func solicitaPerfilLT(options map[string]string) bool {
	for _, clave := range []string{"nivel", "level", "profile", "baseline"} {
		valor := strings.ToLower(strings.TrimSpace(valorOpcion(options, clave)))
		switch valor {
		case "lt", "b-lt", "baseline-lt", "cades-lt":
			return true
		}
	}
	return false
}

func solicitaPerfilLTA(options map[string]string) bool {
	for _, clave := range []string{"nivel", "level", "profile", "baseline"} {
		valor := strings.ToLower(strings.TrimSpace(valorOpcion(options, clave)))
		switch valor {
		case "lta", "b-lta", "baseline-lta", "cades-lta":
			return true
		}
	}
	return false
}

func (m *MotorFirmaGo) resolverTSA(options map[string]string) ports.TimestampAuthority {
	if tsaURL := strings.TrimSpace(valorOpcion(options, "tsaURL")); tsaURL != "" {
		if m != nil {
			if configured, ok := m.tsa.(*tsaclient.Client); ok && configured != nil {
				client := *configured
				client.URL = tsaURL
				return &client
			}
		}
		return tsaclient.New(tsaURL)
	}
	if m == nil {
		return nil
	}
	return m.tsa
}

// resolverTSAURL retorna la URL RFC 3161 utilizable por la ruta pdfsign:
// la opción tsaURL si viene, o la URL del cliente TSA inyectado en el motor.
// Retorna vacío si la TSA configurada no expone URL (implementaciones de la
// interfaz sin cliente HTTP), en cuyo caso la ruta pdfsign no puede sellar.
func (m *MotorFirmaGo) resolverTSAURL(options map[string]string) string {
	if tsaURL := strings.TrimSpace(valorOpcion(options, "tsaURL")); tsaURL != "" {
		return tsaURL
	}
	if m == nil {
		return ""
	}
	if c, ok := m.tsa.(*tsaclient.Client); ok && c != nil {
		return c.URL
	}
	return ""
}

func valorOpcion(options map[string]string, clave string) string {
	for k, v := range options {
		if strings.EqualFold(strings.TrimSpace(k), clave) {
			return v
		}
	}
	return ""
}

func solicitaTSA(options map[string]string) bool {
	for k, v := range options {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(k)), "tsa") && strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

// Comprobar en tiempo de compilacion que MotorFirmaGo implementa ports.SignerEngine.
var _ ports.SignerEngine = (*MotorFirmaGo)(nil)
