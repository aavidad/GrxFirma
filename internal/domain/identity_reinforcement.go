// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	// VersionContratoIdentidadReforzada identifica el contrato genérico interoperable.
	VersionContratoIdentidadReforzada = "identidad-reforzada/v1"
	longitudMinimaNonceIdentidad      = 16
)

var (
	// ErrSolicitudIdentidadInvalida rechaza retos ambiguos o incompletos.
	ErrSolicitudIdentidadInvalida = errors.New("solicitud de identidad reforzada inválida")
	// ErrResultadoIdentidadInvalido rechaza dictámenes incompletos o contradictorios.
	ErrResultadoIdentidadInvalido = errors.New("resultado de identidad reforzada inválido")
	// ErrPruebaIdentidadInvalida rechaza firmas o cadenas ausentes y formatos libres.
	ErrPruebaIdentidadInvalida = errors.New("prueba de identidad reforzada inválida")
)

// ResultadoIdentidad conserva la incertidumbre como un estado de primera clase.
type ResultadoIdentidad string

const (
	ResultadoIdentidadAceptada      ResultadoIdentidad = "accepted"
	ResultadoIdentidadRechazada     ResultadoIdentidad = "rejected"
	ResultadoIdentidadIndeterminada ResultadoIdentidad = "indeterminate"
)

// SolicitudRetoIdentidad contiene únicamente contexto registrado y seudonimizado.
type SolicitudRetoIdentidad struct {
	Contrato, RetoID, Audiencia, ClienteRegistrado, Finalidad, Operacion string
	HuellaContextoTenant, VinculoSesion, Origen                          string
	ConsentimientoID, VersionConsentimiento                              string
	PoliticaID, VersionPolitica                                          string
	Nonce                                                                []byte
	EmitidoEn, ExpiraEn                                                  time.Time
}

// RetoIdentidad conserva los bytes exactos que deben verificarse una sola vez.
type RetoIdentidad struct {
	Solicitud         SolicitudRetoIdentidad
	ContenidoCanonico []byte
}

// PruebaIdentidad contiene el material presentado sin decisiones de autorización.
type PruebaIdentidad struct {
	RetoID, Formato, AlgoritmoFirma, AlgoritmoHuella string
	Firma, Certificado                               []byte
	Cadena                                           [][]byte
}

// Validar limita la prueba a perfiles explícitos y material acotado.
func (p PruebaIdentidad) Validar() error {
	if !textoContratoValido(p.RetoID) || p.Formato != "cades-detached" ||
		!algoritmoFirmaIdentidadValido(p.AlgoritmoFirma) || p.AlgoritmoHuella != "sha-256" ||
		len(p.Firma) == 0 || len(p.Firma) > 512*1024 || len(p.Certificado) == 0 ||
		len(p.Certificado) > 64*1024 || len(p.Cadena) > 8 {
		return ErrPruebaIdentidadInvalida
	}
	for _, certificado := range p.Cadena {
		if len(certificado) == 0 || len(certificado) > 64*1024 {
			return ErrPruebaIdentidadInvalida
		}
	}
	return nil
}

// Validar comprueba el contrato sin conocer usuarios, tenants ni sesiones reales.
func (s SolicitudRetoIdentidad) Validar() error {
	textos := []string{s.Contrato, s.RetoID, s.Audiencia, s.ClienteRegistrado, s.Finalidad,
		s.Operacion, s.HuellaContextoTenant, s.VinculoSesion, s.Origen,
		s.ConsentimientoID, s.VersionConsentimiento, s.PoliticaID, s.VersionPolitica}
	for _, texto := range textos {
		if !textoContratoValido(texto) {
			return ErrSolicitudIdentidadInvalida
		}
	}
	if s.Contrato != VersionContratoIdentidadReforzada || len(s.Nonce) < longitudMinimaNonceIdentidad ||
		len(s.Nonce) > 64 ||
		s.EmitidoEn.IsZero() || !s.ExpiraEn.After(s.EmitidoEn) {
		return ErrSolicitudIdentidadInvalida
	}
	return nil
}

// DictamenComprobacion expresa el resultado verificable de un control criptográfico.
type DictamenComprobacion struct {
	Estado, Fuente string
	ComprobadoEn   time.Time
}

// ResultadoVerificacionIdentidad reúne enlaces, identidad y controles acreditados.
type ResultadoVerificacionIdentidad struct {
	Resultado                                                          ResultadoIdentidad
	EvidenciaRef, RetoID                                               string
	Solicitud                                                          SolicitudRetoIdentidad
	NivelAseguramiento, MetodoAutenticacion                            string
	IdentidadAcreditada, HuellaCertificado                             string
	Integridad, Cadena, Vigencia, EKU, PoliticaCertificado, Revocacion DictamenComprobacion
}

// ValidarEstructura impide convertir omisiones del agente en una aceptación.
func (r ResultadoVerificacionIdentidad) ValidarEstructura() error {
	if r.Solicitud.Validar() != nil || !textoContratoValido(r.EvidenciaRef) ||
		!textoContratoValido(r.RetoID) || r.RetoID != r.Solicitud.RetoID {
		return ErrResultadoIdentidadInvalido
	}
	switch r.Resultado {
	case ResultadoIdentidadRechazada, ResultadoIdentidadIndeterminada:
		return nil
	case ResultadoIdentidadAceptada:
	default:
		return ErrResultadoIdentidadInvalido
	}
	if !textoContratoValido(r.NivelAseguramiento) || !textoContratoValido(r.MetodoAutenticacion) ||
		!textoContratoValido(r.IdentidadAcreditada) || !textoContratoValido(r.HuellaCertificado) {
		return ErrResultadoIdentidadInvalido
	}
	for _, dictamen := range []DictamenComprobacion{r.Integridad, r.Cadena, r.Vigencia, r.EKU, r.PoliticaCertificado, r.Revocacion} {
		if dictamen.Estado != "conforme" || !textoContratoValido(dictamen.Fuente) || dictamen.ComprobadoEn.IsZero() {
			return ErrResultadoIdentidadInvalido
		}
	}
	return nil
}

func textoContratoValido(texto string) bool {
	if texto == "" || texto != strings.TrimSpace(texto) || !utf8.ValidString(texto) || len(texto) > 512 {
		return false
	}
	for _, caracter := range texto {
		if unicode.IsControl(caracter) {
			return false
		}
	}
	return true
}

func algoritmoFirmaIdentidadValido(algoritmo string) bool {
	return algoritmo == "sha256-rsa-pkcs1v15" || algoritmo == "sha256-ecdsa"
}
