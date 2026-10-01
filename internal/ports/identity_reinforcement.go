// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ports

import (
	"context"
	"crypto/x509"
	"time"

	"grxfirma/internal/domain"
)

// EstadoRevocacionIdentidad diferencia evidencia conforme, revocada y ausente.
type EstadoRevocacionIdentidad string

const (
	EstadoRevocacionConforme      EstadoRevocacionIdentidad = "conforme"
	EstadoRevocacionRevocada      EstadoRevocacionIdentidad = "revocada"
	EstadoRevocacionIndeterminada EstadoRevocacionIdentidad = "indeterminada"
)

// ResultadoRevocacionIdentidad conserva fuente e instante de comprobación.
type ResultadoRevocacionIdentidad struct {
	Estado       EstadoRevocacionIdentidad
	Fuente       string
	ComprobadoEn time.Time
}

// CanonicalizadorIdentidad produce los bytes contractuales exactos y versionados.
type CanonicalizadorIdentidad interface {
	Canonicalizar(solicitud domain.SolicitudRetoIdentidad) ([]byte, error)
}

// ContextoSeleccionIdentidad contiene sólo la información que la interfaz
// local necesita mostrar antes de elegir un certificado.
type ContextoSeleccionIdentidad struct {
	Origen, Finalidad, Operacion string
}

// SelectorCertificadoIdentidad mantiene la elección dentro del equipo local;
// ningún portal recibe el catálogo ni puede imponer un certificado.
type SelectorCertificadoIdentidad interface {
	Seleccionar(ctx context.Context, contexto ContextoSeleccionIdentidad, certificados []domain.CertificateRef) (domain.CertificateRef, error)
}

// AutorizadorSolicitudIdentidad exige cliente, origen, audiencia y política registrados.
type AutorizadorSolicitudIdentidad interface {
	Autorizar(ctx context.Context, solicitud domain.SolicitudRetoIdentidad) error
}

// RepositorioRetosIdentidad persiste y consume retos sin envolver llamadas externas.
type RepositorioRetosIdentidad interface {
	Registrar(ctx context.Context, reto domain.RetoIdentidad) error
	Reservar(ctx context.Context, retoID string, ahora time.Time) (domain.RetoIdentidad, error)
	Finalizar(ctx context.Context, resultado domain.ResultadoVerificacionIdentidad) error
}

// VerificadorPruebaIdentidad valida firma, cadena, uso, política y revocación.
type VerificadorPruebaIdentidad interface {
	Verificar(ctx context.Context, reto domain.RetoIdentidad, prueba domain.PruebaIdentidad) (domain.ResultadoVerificacionIdentidad, error)
}

// ComprobadorRevocacionIdentidad consulta evidencia autenticada sin colapsar incertidumbre.
type ComprobadorRevocacionIdentidad interface {
	ComprobarIdentidad(ctx context.Context, certificado, emisor *x509.Certificate) (ResultadoRevocacionIdentidad, error)
}

// AcreditadorCertificadoIdentidad obtiene una identidad opaca desde una fuente gobernada.
type AcreditadorCertificadoIdentidad interface {
	AcreditarCertificado(ctx context.Context, certificado *x509.Certificate) (string, error)
}

// EvidenciaIdentidad contiene material mínimo para un registro durable protegido.
type EvidenciaIdentidad struct {
	Contrato, RetoID, PoliticaID, VersionPolitica           string
	ContenidoCanonico                                       []byte
	HuellaContenido, HuellaFirma, HuellaCertificado         string
	Resultado                                               domain.ResultadoIdentidad
	Integridad, Cadena, Vigencia, EKU, Politica, Revocacion domain.DictamenComprobacion
	ComprobadoEn                                            time.Time
}

// RegistroEvidenciaIdentidad devuelve una referencia opaca tras persistir evidencia.
type RegistroEvidenciaIdentidad interface {
	RegistrarIdentidad(ctx context.Context, evidencia EvidenciaIdentidad) (string, error)
}
