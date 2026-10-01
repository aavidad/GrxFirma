// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package identityjcs implementa el perfil canónico de identidad reforzada.
package identityjcs

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	canonicaljcs "github.com/gowebpki/jcs"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

const (
	// VersionFormato fija el perfil interoperable con los integradores.
	VersionFormato  = "rfc8785-jcs-v1"
	maximoPermitido = 16 * 1024
)

var (
	// ErrConfiguracionInvalida rechaza versiones o límites no gobernados.
	ErrConfiguracionInvalida = errors.New("configuración de canonicalización de identidad inválida")
	// ErrCanonicalizacion impide publicar bytes incompletos o excesivos.
	ErrCanonicalizacion = errors.New("no se pudo canonicalizar el reto de identidad")
)

// Configuracion declara versión y máximo de salida.
type Configuracion struct {
	VersionFormato string
	MaximoBytes    int
}

// Canonicalizador adapta el dominio al objeto JSON contractual RFC 8785.
type Canonicalizador struct{ maximoBytes int }

// ConfiguracionPredeterminada devuelve el único perfil admitido en v1.
func ConfiguracionPredeterminada() Configuracion {
	return Configuracion{VersionFormato: VersionFormato, MaximoBytes: maximoPermitido}
}

// Nuevo valida la configuración completa antes de procesar retos.
func Nuevo(configuracion Configuracion) (*Canonicalizador, error) {
	if configuracion.VersionFormato != VersionFormato || configuracion.MaximoBytes <= 0 || configuracion.MaximoBytes > maximoPermitido {
		return nil, ErrConfiguracionInvalida
	}
	return &Canonicalizador{maximoBytes: configuracion.MaximoBytes}, nil
}

// Canonicalizar devuelve una copia de los bytes UTF-8 JCS exactos.
func (c *Canonicalizador) Canonicalizar(solicitud domain.SolicitudRetoIdentidad) ([]byte, error) {
	if c == nil || c.maximoBytes <= 0 || solicitud.Validar() != nil {
		return nil, ErrCanonicalizacion
	}
	serializado, err := json.Marshal(nuevoContrato(solicitud))
	if err != nil {
		return nil, ErrCanonicalizacion
	}
	canon, err := canonicaljcs.Transform(serializado)
	if err != nil || len(canon) == 0 || len(canon) > c.maximoBytes {
		return nil, ErrCanonicalizacion
	}
	return append([]byte(nil), canon...), nil
}

type contratoFirmable struct {
	Contrato              string `json:"contract"`
	RetoID                string `json:"challengeId"`
	ClienteRegistrado     string `json:"registeredClient"`
	Finalidad             string `json:"purpose"`
	Operacion             string `json:"operation"`
	Audiencia             string `json:"audience"`
	HuellaContextoTenant  string `json:"tenantContextHash"`
	VinculoSesion         string `json:"sessionBinding"`
	Origen                string `json:"origin"`
	ConsentimientoID      string `json:"consentId"`
	VersionConsentimiento string `json:"consentVersion"`
	PoliticaID            string `json:"policyId"`
	VersionPolitica       string `json:"policyVersion"`
	Nonce                 string `json:"nonce"`
	EmitidoEn             string `json:"issuedAt"`
	ExpiraEn              string `json:"expiresAt"`
}

func nuevoContrato(s domain.SolicitudRetoIdentidad) contratoFirmable {
	return contratoFirmable{s.Contrato, s.RetoID, s.ClienteRegistrado, s.Finalidad, s.Operacion,
		s.Audiencia, s.HuellaContextoTenant, s.VinculoSesion, s.Origen,
		s.ConsentimientoID, s.VersionConsentimiento, s.PoliticaID, s.VersionPolitica,
		base64.RawURLEncoding.EncodeToString(s.Nonce), s.EmitidoEn.UTC().Format(time.RFC3339Nano),
		s.ExpiraEn.UTC().Format(time.RFC3339Nano)}
}

var _ ports.CanonicalizadorIdentidad = (*Canonicalizador)(nil)
