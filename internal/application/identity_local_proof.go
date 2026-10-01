// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/url"
	"strings"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

const (
	maximoCanonPruebaIdentidadLocal = 16 * 1024
	maximoCertificadosIdentidad     = 64
)

var (
	// ErrPruebaIdentidadLocalInvalida rechaza retos alterados, caducados o ambiguos.
	ErrPruebaIdentidadLocalInvalida = errors.New("solicitud local de identidad inválida")
	// ErrPruebaIdentidadLocalNoDisponible oculta fallos del almacén y del firmador.
	ErrPruebaIdentidadLocalNoDisponible = errors.New("prueba local de identidad no disponible")
)

// FirmadorPruebaIdentidad ejecuta la firma genérica después de la selección local.
type FirmadorPruebaIdentidad interface {
	Execute(ctx context.Context, comando SignCommand) (SignResult, error)
}

// GeneradorPruebaIdentidadLocal produce una prueba sin revelar el catálogo al portal.
type GeneradorPruebaIdentidadLocal struct {
	canonicalizador ports.CanonicalizadorIdentidad
	catalogo        ports.CertificateCatalog
	selector        ports.SelectorCertificadoIdentidad
	firmador        FirmadorPruebaIdentidad
	reloj           ports.Clock
}

// NuevoGeneradorPruebaIdentidadLocal exige todos los conectores antes de operar.
func NuevoGeneradorPruebaIdentidadLocal(
	canonicalizador ports.CanonicalizadorIdentidad,
	catalogo ports.CertificateCatalog,
	selector ports.SelectorCertificadoIdentidad,
	firmador FirmadorPruebaIdentidad,
	reloj ports.Clock,
) (*GeneradorPruebaIdentidadLocal, error) {
	if canonicalizador == nil || catalogo == nil || selector == nil || firmador == nil || reloj == nil {
		return nil, ErrPruebaIdentidadLocalNoDisponible
	}
	return &GeneradorPruebaIdentidadLocal{canonicalizador, catalogo, selector, firmador, reloj}, nil
}

// Generar revalida el canon, solicita selección y consentimiento y firma CAdES detached.
func (g *GeneradorPruebaIdentidadLocal) Generar(
	ctx context.Context,
	reto domain.RetoIdentidad,
) (domain.PruebaIdentidad, error) {
	if ctx == nil || !generadorPruebaIdentidadConfigurado(g) {
		return domain.PruebaIdentidad{}, ErrPruebaIdentidadLocalInvalida
	}
	ahora := g.reloj.Now().UTC()
	if !retoLocalValido(reto, ahora) {
		return domain.PruebaIdentidad{}, ErrPruebaIdentidadLocalInvalida
	}
	canonEsperado, err := g.canonicalizador.Canonicalizar(reto.Solicitud)
	if err != nil || len(canonEsperado) != len(reto.ContenidoCanonico) ||
		subtle.ConstantTimeCompare(canonEsperado, reto.ContenidoCanonico) != 1 {
		return domain.PruebaIdentidad{}, ErrPruebaIdentidadLocalInvalida
	}
	certificados, err := g.catalogo.List(ctx)
	if err != nil {
		return domain.PruebaIdentidad{}, ErrPruebaIdentidadLocalNoDisponible
	}
	candidatos := certificadosVigentes(certificados, ahora)
	seleccionado, err := g.selector.Seleccionar(ctx, ports.ContextoSeleccionIdentidad{
		Origen: reto.Solicitud.Origen, Finalidad: reto.Solicitud.Finalidad,
		Operacion: reto.Solicitud.Operacion,
	}, candidatos)
	if err != nil || !certificadoSeleccionadoValido(seleccionado, candidatos) {
		return domain.PruebaIdentidad{}, ErrPruebaIdentidadLocalNoDisponible
	}
	comando, err := NewSignCommand(
		"reto-identidad.json",
		append([]byte(nil), reto.ContenidoCanonico...),
		"application/json", "cades", "sign", seleccionado.ID, nil,
	)
	if err != nil {
		return domain.PruebaIdentidad{}, ErrPruebaIdentidadLocalInvalida
	}
	comando.RequesterApplication = contextoFirmaIdentidad(reto.Solicitud)
	comando.RequesterOrigin = reto.Solicitud.Origen
	firmado, err := g.firmador.Execute(ctx, comando)
	if err != nil {
		return domain.PruebaIdentidad{}, ErrPruebaIdentidadLocalNoDisponible
	}
	prueba, err := nuevaPruebaIdentidadLocal(reto.Solicitud.RetoID, firmado)
	if err != nil {
		return domain.PruebaIdentidad{}, err
	}
	return prueba, nil
}

func generadorPruebaIdentidadConfigurado(g *GeneradorPruebaIdentidadLocal) bool {
	return g != nil && g.canonicalizador != nil && g.catalogo != nil &&
		g.selector != nil && g.firmador != nil && g.reloj != nil
}

func retoLocalValido(reto domain.RetoIdentidad, ahora time.Time) bool {
	return reto.Solicitud.Validar() == nil && len(reto.ContenidoCanonico) > 0 &&
		len(reto.ContenidoCanonico) <= maximoCanonPruebaIdentidadLocal &&
		origenIdentidadHTTPSValido(reto.Solicitud.Origen) &&
		ahora.Before(reto.Solicitud.ExpiraEn) &&
		!reto.Solicitud.EmitidoEn.After(ahora.Add(time.Minute))
}

func origenIdentidadHTTPSValido(origen string) bool {
	direccion, err := url.Parse(origen)
	return err == nil && direccion.Scheme == "https" && direccion.Host != "" &&
		direccion.Opaque == "" && direccion.User == nil && direccion.Path == "" &&
		direccion.RawPath == "" && direccion.RawQuery == "" && !direccion.ForceQuery &&
		direccion.Fragment == "" && origen == direccion.Scheme+"://"+direccion.Host
}

func certificadosVigentes(certificados []domain.CertificateRef, ahora time.Time) []domain.CertificateRef {
	resultado := make([]domain.CertificateRef, 0, len(certificados))
	for _, certificado := range certificados {
		if certificado.Validate() == nil && !certificado.NotAfter.IsZero() && ahora.Before(certificado.NotAfter) {
			resultado = append(resultado, certificado)
			if len(resultado) == maximoCertificadosIdentidad {
				break
			}
		}
	}
	return resultado
}

func certificadoSeleccionadoValido(seleccionado domain.CertificateRef, candidatos []domain.CertificateRef) bool {
	for _, candidato := range candidatos {
		if seleccionado.ID == candidato.ID && seleccionado.Fingerprint == candidato.Fingerprint {
			return true
		}
	}
	return false
}

func contextoFirmaIdentidad(solicitud domain.SolicitudRetoIdentidad) string {
	return strings.Join([]string{solicitud.Finalidad, solicitud.Operacion}, " · ")
}

func nuevaPruebaIdentidadLocal(retoID string, firmado SignResult) (domain.PruebaIdentidad, error) {
	if firmado.Result.Format != domain.FormatCAdES || len(firmado.CertificateChainDER) == 0 ||
		len(firmado.CertificateChainDER) > 9 {
		return domain.PruebaIdentidad{}, ErrPruebaIdentidadLocalNoDisponible
	}
	algoritmo := ""
	switch strings.ToUpper(strings.TrimSpace(firmado.Result.Algorithm)) {
	case "SHA256WITHRSA":
		algoritmo = "sha256-rsa-pkcs1v15"
	case "SHA256WITHECDSA":
		algoritmo = "sha256-ecdsa"
	default:
		return domain.PruebaIdentidad{}, ErrPruebaIdentidadLocalNoDisponible
	}
	prueba := domain.PruebaIdentidad{
		RetoID: retoID, Formato: "cades-detached", AlgoritmoFirma: algoritmo,
		AlgoritmoHuella: "sha-256", Firma: append([]byte(nil), firmado.Result.Data...),
		Certificado: append([]byte(nil), firmado.CertificateChainDER[0]...),
		Cadena:      copiarMatrizBytes(firmado.CertificateChainDER[1:]),
	}
	if prueba.Validar() != nil {
		return domain.PruebaIdentidad{}, ErrPruebaIdentidadLocalNoDisponible
	}
	return prueba, nil
}
