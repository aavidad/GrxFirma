// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package identityverifier valida pruebas CAdES y confianza X.509 del contrato v1.
package identityverifier

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

var (
	// ErrConfiguracionInvalida impide verificar sin una política completa.
	ErrConfiguracionInvalida = errors.New("configuración del verificador de identidad inválida")
	// ErrVerificacionIndeterminada indica que falta evidencia para decidir.
	ErrVerificacionIndeterminada = errors.New("verificación de identidad indeterminada")
)

// Configuracion fija EKU, OID de política y nivel que puede acreditar el conector.
type Configuracion struct {
	EKUPermitidos       []x509.ExtKeyUsage
	OIDPoliticas        []asn1.ObjectIdentifier
	NivelAseguramiento  string
	MetodoAutenticacion string
	FuenteDictamen      string
}

// Verificador compone motores criptográficos y fuentes de confianza sustituibles.
type Verificador struct {
	configuracion Configuracion
	anclas        ports.TrustAnchorProvider
	revocacion    ports.ComprobadorRevocacionIdentidad
	acreditador   ports.AcreditadorCertificadoIdentidad
	evidencias    ports.RegistroEvidenciaIdentidad
	reloj         ports.Clock
	cades         *signer.CAdESVerifier
}

// Nuevo valida y copia toda la política antes de admitir pruebas.
func Nuevo(configuracion Configuracion, anclas ports.TrustAnchorProvider, revocacion ports.ComprobadorRevocacionIdentidad, acreditador ports.AcreditadorCertificadoIdentidad, evidencias ports.RegistroEvidenciaIdentidad, reloj ports.Clock) (*Verificador, error) {
	if !configuracionValida(configuracion) || anclas == nil || revocacion == nil || acreditador == nil || evidencias == nil || reloj == nil {
		return nil, ErrConfiguracionInvalida
	}
	configuracion.EKUPermitidos = append([]x509.ExtKeyUsage(nil), configuracion.EKUPermitidos...)
	configuracion.OIDPoliticas = copiarOID(configuracion.OIDPoliticas)
	return &Verificador{configuracion: configuracion, anclas: anclas, revocacion: revocacion,
		acreditador: acreditador, evidencias: evidencias, reloj: reloj,
		cades: signer.NewCAdESVerifierWithChecker(nil)}, nil
}

// Verificar devuelve aceptación solo cuando todos los controles son conformes.
func (v *Verificador) Verificar(ctx context.Context, reto domain.RetoIdentidad, prueba domain.PruebaIdentidad) (domain.ResultadoVerificacionIdentidad, error) {
	resultado := resultadoBase(reto)
	if ctx == nil || ctx.Err() != nil || reto.Solicitud.Validar() != nil || prueba.Validar() != nil ||
		prueba.RetoID != reto.Solicitud.RetoID || len(reto.ContenidoCanonico) == 0 {
		return v.registrar(ctx, reto, prueba, resultado, domain.ResultadoIdentidadRechazada, nil)
	}
	certificado, cadenaPresentada, err := parsearCertificados(prueba)
	if err != nil || !algoritmoCompatible(certificado, prueba.AlgoritmoFirma) {
		resultado.Integridad = dictamen("no_conforme", v.configuracion.FuenteDictamen, v.reloj.Now())
		return v.registrar(ctx, reto, prueba, resultado, domain.ResultadoIdentidadRechazada, nil)
	}
	anclas, err := v.anclas.Anchors(ctx)
	if err != nil || anclas.IsEmpty() {
		return v.registrar(ctx, reto, prueba, resultado, domain.ResultadoIdentidadIndeterminada, ErrVerificacionIndeterminada)
	}
	verificacion, firmantes, err := v.cades.VerifyDetachedCMSWithAnchors(ctx, prueba.Firma, reto.ContenidoCanonico, anclas)
	if err != nil || !firmanteCoincide(firmantes, certificado) || verificacion.Integrity.Status != domain.VerificationStatusValid {
		resultado.Integridad = dictamen("no_conforme", v.configuracion.FuenteDictamen, v.reloj.Now())
		return v.registrar(ctx, reto, prueba, resultado, domain.ResultadoIdentidadRechazada, nil)
	}
	resultado.Integridad = dictamen("conforme", v.configuracion.FuenteDictamen, v.reloj.Now())
	cadenaVerificada, err := verificarCadena(certificado, cadenaPresentada, anclas, v.reloj.Now())
	if err != nil || verificacion.Trust.Status == domain.VerificationStatusInvalid {
		resultado.Cadena = dictamen("no_conforme", v.configuracion.FuenteDictamen, v.reloj.Now())
		return v.registrar(ctx, reto, prueba, resultado, domain.ResultadoIdentidadRechazada, nil)
	}
	if verificacion.Trust.Status != domain.VerificationStatusValid {
		return v.registrar(ctx, reto, prueba, resultado, domain.ResultadoIdentidadIndeterminada, ErrVerificacionIndeterminada)
	}
	resultado.Cadena = dictamen("conforme", v.configuracion.FuenteDictamen, v.reloj.Now())
	ahora := v.reloj.Now().UTC()
	if ahora.Before(certificado.NotBefore) || ahora.After(certificado.NotAfter) || certificado.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		resultado.Vigencia = dictamen("no_conforme", v.configuracion.FuenteDictamen, ahora)
		return v.registrar(ctx, reto, prueba, resultado, domain.ResultadoIdentidadRechazada, nil)
	}
	resultado.Vigencia = dictamen("conforme", v.configuracion.FuenteDictamen, ahora)
	if !ekuPermitido(certificado.ExtKeyUsage, v.configuracion.EKUPermitidos) {
		resultado.EKU = dictamen("no_conforme", v.configuracion.FuenteDictamen, ahora)
		return v.registrar(ctx, reto, prueba, resultado, domain.ResultadoIdentidadRechazada, nil)
	}
	resultado.EKU = dictamen("conforme", v.configuracion.FuenteDictamen, ahora)
	if !politicaPermitida(certificado.PolicyIdentifiers, v.configuracion.OIDPoliticas) {
		resultado.PoliticaCertificado = dictamen("no_conforme", v.configuracion.FuenteDictamen, ahora)
		return v.registrar(ctx, reto, prueba, resultado, domain.ResultadoIdentidadRechazada, nil)
	}
	resultado.PoliticaCertificado = dictamen("conforme", v.configuracion.FuenteDictamen, ahora)
	revocacion, err := comprobarCadenaRevocacion(ctx, v.revocacion, cadenaVerificada)
	if err != nil || revocacion.Estado == ports.EstadoRevocacionIndeterminada {
		resultado.Revocacion = dictamen("indeterminado", fuente(revocacion.Fuente, v.configuracion.FuenteDictamen), ahora)
		return v.registrar(ctx, reto, prueba, resultado, domain.ResultadoIdentidadIndeterminada, ErrVerificacionIndeterminada)
	}
	if revocacion.Estado == ports.EstadoRevocacionRevocada {
		resultado.Revocacion = dictamen("no_conforme", fuente(revocacion.Fuente, v.configuracion.FuenteDictamen), ahora)
		return v.registrar(ctx, reto, prueba, resultado, domain.ResultadoIdentidadRechazada, nil)
	}
	resultado.Revocacion = dictamen("conforme", fuente(revocacion.Fuente, v.configuracion.FuenteDictamen), revocacion.ComprobadoEn)
	identidad, err := v.acreditador.AcreditarCertificado(ctx, certificado)
	if err != nil {
		return v.registrar(ctx, reto, prueba, resultado, domain.ResultadoIdentidadIndeterminada, ErrVerificacionIndeterminada)
	}
	if !textoSeguro(identidad) {
		return v.registrar(ctx, reto, prueba, resultado, domain.ResultadoIdentidadRechazada, nil)
	}
	resultado.NivelAseguramiento = v.configuracion.NivelAseguramiento
	resultado.MetodoAutenticacion = v.configuracion.MetodoAutenticacion
	resultado.IdentidadAcreditada = identidad
	resultado.HuellaCertificado = huella(certificado.Raw)
	return v.registrar(ctx, reto, prueba, resultado, domain.ResultadoIdentidadAceptada, nil)
}

func (v *Verificador) registrar(ctx context.Context, reto domain.RetoIdentidad, prueba domain.PruebaIdentidad, resultado domain.ResultadoVerificacionIdentidad, estado domain.ResultadoIdentidad, causa error) (domain.ResultadoVerificacionIdentidad, error) {
	resultado.Resultado = estado
	evidencia := ports.EvidenciaIdentidad{Contrato: reto.Solicitud.Contrato, RetoID: reto.Solicitud.RetoID,
		PoliticaID: reto.Solicitud.PoliticaID, VersionPolitica: reto.Solicitud.VersionPolitica,
		ContenidoCanonico: append([]byte(nil), reto.ContenidoCanonico...),
		HuellaContenido:   huella(reto.ContenidoCanonico), HuellaFirma: huella(prueba.Firma),
		HuellaCertificado: huella(prueba.Certificado), Resultado: estado,
		Integridad: resultado.Integridad, Cadena: resultado.Cadena, Vigencia: resultado.Vigencia,
		EKU: resultado.EKU, Politica: resultado.PoliticaCertificado, Revocacion: resultado.Revocacion,
		ComprobadoEn: v.reloj.Now().UTC()}
	referencia, err := v.evidencias.RegistrarIdentidad(ctx, evidencia)
	if err != nil || strings.TrimSpace(referencia) == "" {
		resultado.Resultado = domain.ResultadoIdentidadIndeterminada
		resultado.EvidenciaRef = reto.Solicitud.RetoID
		return resultado, ErrVerificacionIndeterminada
	}
	resultado.EvidenciaRef = referencia
	return resultado, causa
}

func resultadoBase(reto domain.RetoIdentidad) domain.ResultadoVerificacionIdentidad {
	return domain.ResultadoVerificacionIdentidad{RetoID: reto.Solicitud.RetoID, Solicitud: reto.Solicitud,
		Integridad: dictamen("indeterminado", "pendiente", time.Time{}), Cadena: dictamen("indeterminado", "pendiente", time.Time{}),
		Vigencia: dictamen("indeterminado", "pendiente", time.Time{}), EKU: dictamen("indeterminado", "pendiente", time.Time{}),
		PoliticaCertificado: dictamen("indeterminado", "pendiente", time.Time{}), Revocacion: dictamen("indeterminado", "pendiente", time.Time{})}
}

func dictamen(estado, origen string, instante time.Time) domain.DictamenComprobacion {
	return domain.DictamenComprobacion{Estado: estado, Fuente: origen, ComprobadoEn: instante.UTC()}
}

func parsearCertificados(prueba domain.PruebaIdentidad) (*x509.Certificate, []*x509.Certificate, error) {
	certificado, err := x509.ParseCertificate(prueba.Certificado)
	if err != nil {
		return nil, nil, err
	}
	cadena := make([]*x509.Certificate, 0, len(prueba.Cadena))
	for _, der := range prueba.Cadena {
		actual, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, nil, err
		}
		cadena = append(cadena, actual)
	}
	return certificado, cadena, nil
}

func algoritmoCompatible(certificado *x509.Certificate, algoritmo string) bool {
	switch certificado.PublicKey.(type) {
	case *rsa.PublicKey:
		return algoritmo == "sha256-rsa-pkcs1v15"
	case *ecdsa.PublicKey:
		return algoritmo == "sha256-ecdsa"
	default:
		return false
	}
}

func firmanteCoincide(firmantes []domain.CertificateRef, certificado *x509.Certificate) bool {
	if len(firmantes) != 1 || certificado == nil {
		return false
	}
	esperada := strings.ToLower(strings.ReplaceAll(firmantes[0].Fingerprint, ":", ""))
	return esperada == huella(certificado.Raw)
}

func verificarCadena(certificado *x509.Certificate, presentada []*x509.Certificate, anclas domain.CertificateChain, ahora time.Time) ([]*x509.Certificate, error) {
	raices := x509.NewCertPool()
	if anclas.UseSystemRoots {
		sistema, err := x509.SystemCertPool()
		if err != nil || sistema == nil {
			return nil, ErrVerificacionIndeterminada
		}
		raices = sistema.Clone()
	}
	for _, der := range anclas.DERCertificates {
		raiz, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, err
		}
		raices.AddCert(raiz)
	}
	intermedias := x509.NewCertPool()
	for _, actual := range presentada {
		intermedias.AddCert(actual)
	}
	cadenas, err := certificado.Verify(x509.VerifyOptions{Roots: raices, Intermediates: intermedias,
		CurrentTime: ahora.UTC(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
	if err != nil || len(cadenas) == 0 {
		return nil, err
	}
	return cadenas[0], nil
}

func comprobarCadenaRevocacion(ctx context.Context, comprobador ports.ComprobadorRevocacionIdentidad, cadena []*x509.Certificate) (ports.ResultadoRevocacionIdentidad, error) {
	if len(cadena) < 2 {
		return ports.ResultadoRevocacionIdentidad{Estado: ports.EstadoRevocacionIndeterminada}, ErrVerificacionIndeterminada
	}
	var ultimo ports.ResultadoRevocacionIdentidad
	for i := 0; i < len(cadena)-1; i++ {
		actual, err := comprobador.ComprobarIdentidad(ctx, cadena[i], cadena[i+1])
		if err != nil || actual.Estado != ports.EstadoRevocacionConforme {
			return actual, err
		}
		ultimo = actual
	}
	return ultimo, nil
}

func ekuPermitido(certificado, permitidos []x509.ExtKeyUsage) bool {
	for _, actual := range certificado {
		for _, permitido := range permitidos {
			if actual == permitido {
				return true
			}
		}
	}
	return false
}

func politicaPermitida(certificado, permitidas []asn1.ObjectIdentifier) bool {
	for _, actual := range certificado {
		for _, permitida := range permitidas {
			if actual.Equal(permitida) {
				return true
			}
		}
	}
	return false
}

func configuracionValida(configuracion Configuracion) bool {
	if len(configuracion.EKUPermitidos) == 0 || len(configuracion.OIDPoliticas) == 0 ||
		(configuracion.NivelAseguramiento != "mfa" && configuracion.NivelAseguramiento != "certificado") ||
		!textoSeguro(configuracion.MetodoAutenticacion) || !textoSeguro(configuracion.FuenteDictamen) {
		return false
	}
	for _, uso := range configuracion.EKUPermitidos {
		if uso == x509.ExtKeyUsageAny {
			return false
		}
	}
	for _, oid := range configuracion.OIDPoliticas {
		if len(oid) < 2 {
			return false
		}
	}
	return true
}

func textoSeguro(valor string) bool {
	return utf8.ValidString(valor) && valor == strings.TrimSpace(valor) && len(valor) <= 512 &&
		valor != "" && strings.IndexFunc(valor, unicode.IsControl) == -1
}

func copiarOID(origen []asn1.ObjectIdentifier) []asn1.ObjectIdentifier {
	destino := make([]asn1.ObjectIdentifier, len(origen))
	for i := range origen {
		destino[i] = append(asn1.ObjectIdentifier(nil), origen[i]...)
	}
	return destino
}

func huella(contenido []byte) string {
	suma := sha256.Sum256(contenido)
	return hex.EncodeToString(suma[:])
}

func fuente(obtenida, alternativa string) string {
	if strings.TrimSpace(obtenida) != "" {
		return obtenida
	}
	return alternativa
}

var _ ports.VerificadorPruebaIdentidad = (*Verificador)(nil)
