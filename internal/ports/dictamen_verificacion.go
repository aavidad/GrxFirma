// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ports

import (
	"context"
	"crypto"
	"time"

	"grxfirma/internal/domain"
)

// EntradaDictamen reúne lo que necesita una evaluación autónoma: el
// documento firmado, el original aportado (si lo hay), el resultado técnico
// del verificador de formato y las anclas locales que rigen la confianza.
type EntradaDictamen struct {
	Firmado    domain.Document
	Original   *domain.Document
	Resultado  domain.VerificationResult
	Anclas     domain.CertificateChain
	Referencia time.Time
}

// EvaluadorDictamenFirma completa un resultado técnico con cadena,
// revocación, sello de tiempo y vínculo con el original evaluados con
// fuentes locales. Devuelve los aspectos por firmante; el veredicto global lo
// compone domain.DictamenVerificacion.Componer. Solo devuelve error ante
// cancelación o entrada inutilizable, nunca para expresar incertidumbre.
type EvaluadorDictamenFirma interface {
	Evaluar(ctx context.Context, entrada EntradaDictamen) (domain.DictamenVerificacion, error)
}

// ResultadoSelloRemoto es la respuesta de un validador externo de sellos.
type ResultadoSelloRemoto struct {
	// Valido solo es true si el servicio acreditó el sello y su TSA.
	Valido bool
	Fecha  time.Time
}

// ValidadorSelloTiempoRemoto es un punto de extensión opcional para
// acreditar sellos cuya TSA no está entre las anclas locales. No hay ningún
// adaptador activo por defecto: la verificación es autónoma.
type ValidadorSelloTiempoRemoto interface {
	ValidarSello(ctx context.Context, token []byte, resumen []byte, algoritmo crypto.Hash) (ResultadoSelloRemoto, error)
}
