// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package verificacionlocal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"time"

	"grxfirma/internal/adapters/outbound/common/revocationclient"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// Configuracion fija las fuentes del evaluador. Solo CRL es local; las otras
// dos son extensiones remotas y, a nil, quedan desactivadas.
type Configuracion struct {
	// CRL es el directorio local de CRL. Sin él la revocación queda
	// no_comprobada salvo que la firma embeba evidencias autenticables.
	CRL *AlmacenCRL
	// RevocacionRemota es el punto de extensión para OCSP/CRL por red. Solo
	// se consulta si las fuentes locales no concluyen. Desactivado por defecto.
	RevocacionRemota ports.RevocationProvider
	// SelloRemoto es el punto de extensión para acreditar sellos cuya TSA no
	// está entre las anclas locales. Desactivado por defecto.
	SelloRemoto ports.ValidadorSelloTiempoRemoto
}

// Evaluador implementa ports.EvaluadorDictamenFirma con fuentes locales.
type Evaluador struct {
	cfg Configuracion
}

// Nuevo crea un evaluador. La configuración vacía es válida y autónoma:
// cadena contra las anclas recibidas y revocación solo con evidencia embebida.
func Nuevo(cfg Configuracion) *Evaluador {
	return &Evaluador{cfg: cfg}
}

var _ ports.EvaluadorDictamenFirma = (*Evaluador)(nil)

// Evaluar devuelve los aspectos del dictamen por firmante. El veredicto
// global lo compone el dominio.
func (e *Evaluador) Evaluar(ctx context.Context, entrada ports.EntradaDictamen) (domain.DictamenVerificacion, error) {
	if err := ctx.Err(); err != nil {
		return domain.DictamenVerificacion{}, err
	}
	referencia := entrada.Referencia
	if referencia.IsZero() {
		referencia = time.Now().UTC()
	}
	anclas, err := poolAnclas(entrada.Anclas)
	if err != nil {
		return domain.DictamenVerificacion{}, err
	}
	dictamen := domain.DictamenVerificacion{
		Integridad:      evaluarIntegridad(entrada.Resultado.Integrity),
		VinculoOriginal: evaluarVinculo(entrada),
		Extensiones: domain.ExtensionesDictamen{
			RevocacionRemota:  estadoExtension(e.cfg.RevocacionRemota != nil),
			SelloTiempoRemoto: estadoExtension(e.cfg.SelloRemoto != nil),
		},
	}
	for _, material := range entrada.Resultado.Material.Firmas {
		if err := ctx.Err(); err != nil {
			return domain.DictamenVerificacion{}, err
		}
		firmante, ok := e.evaluarFirmante(ctx, material, anclas, referencia)
		if ok {
			dictamen.Firmantes = append(dictamen.Firmantes, firmante)
		}
	}
	return dictamen, nil
}

func estadoExtension(activa bool) string {
	if activa {
		return domain.ExtensionActiva
	}
	return domain.ExtensionDesactivada
}

// poolAnclas solo admite anclas X.509 explícitas. El almacén del sistema no
// cuenta como ancla local: si es lo único disponible, no hay anclas.
func poolAnclas(cadena domain.CertificateChain) (*x509.CertPool, error) {
	if len(cadena.DERCertificates) == 0 {
		return nil, nil
	}
	pool := x509.NewCertPool()
	for _, der := range cadena.DERCertificates {
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, errors.New("verificacionlocal: ancla de confianza X.509 no válida")
		}
		pool.AddCert(cert)
	}
	return pool, nil
}

func evaluarIntegridad(aspecto domain.VerificationAspect) domain.AspectoDictamen {
	switch aspecto.Status {
	case domain.VerificationStatusValid:
		return domain.AspectoDictamen{Estado: domain.IntegridadValida}
	case domain.VerificationStatusInvalid:
		return domain.AspectoDictamen{Estado: domain.IntegridadNoValida, Motivo: "firma_no_corresponde_al_contenido"}
	case domain.VerificationStatusWarning:
		return domain.AspectoDictamen{Estado: domain.IntegridadParcial, Motivo: "contenido_no_cubierto_por_la_firma"}
	default:
		return domain.AspectoDictamen{Estado: domain.IntegridadParcial, Motivo: "integridad_no_concluyente"}
	}
}

// evaluarVinculo acredita que la firma cubre exactamente el original aportado:
// en CMS, porque el resumen verificado es el del original; en PAdES, porque
// el original es una revisión previa íntegramente cubierta por una firma.
func evaluarVinculo(entrada ports.EntradaDictamen) domain.AspectoDictamen {
	if entrada.Original == nil {
		return domain.AspectoDictamen{Estado: domain.VinculoNoAportado}
	}
	original := entrada.Original.Content
	material := entrada.Resultado.Material
	if len(material.CoberturasPDF) > 0 {
		firmado := entrada.Firmado.Content
		if len(original) == 0 || len(original) > len(firmado) || !bytes.Equal(firmado[:len(original)], original) {
			return domain.AspectoDictamen{Estado: domain.VinculoNoAcreditado, Motivo: "original_no_es_revision_previa", Fuente: "pades_revision"}
		}
		for _, cobertura := range material.CoberturasPDF {
			if cobertura.InicioHueco >= len(original) {
				return domain.AspectoDictamen{Estado: domain.VinculoAcreditado, Fuente: "pades_revision"}
			}
		}
		return domain.AspectoDictamen{Estado: domain.VinculoNoAcreditado, Motivo: "original_no_cubierto_por_la_firma", Fuente: "pades_revision"}
	}
	suma := sha256.Sum256(original)
	huella := hex.EncodeToString(suma[:])
	for _, verificada := range material.HuellasContenidoFirmado {
		if verificada == huella {
			return domain.AspectoDictamen{Estado: domain.VinculoAcreditado, Fuente: "resumen_contenido_firmado"}
		}
	}
	if len(material.HuellasContenidoFirmado) == 0 {
		return domain.AspectoDictamen{Estado: domain.VinculoNoAcreditado, Motivo: "formato_sin_comprobacion_de_vinculo"}
	}
	return domain.AspectoDictamen{Estado: domain.VinculoNoAcreditado, Motivo: "original_no_coincide", Fuente: "resumen_contenido_firmado"}
}

func (e *Evaluador) evaluarFirmante(ctx context.Context, material domain.MaterialFirma, anclas *x509.CertPool, referencia time.Time) (domain.DictamenFirmante, bool) {
	cert, err := x509.ParseCertificate(material.CertificadoDER)
	if err != nil {
		return domain.DictamenFirmante{}, false
	}
	suma := sha256.Sum256(cert.Raw)
	firmante := domain.DictamenFirmante{
		CertificadoHuellaSHA256: hex.EncodeToString(suma[:]),
		Serie:                   cert.SerialNumber.String(),
		Asunto:                  cert.Subject.String(),
		Emisor:                  cert.Issuer.String(),
		Certificado:             evaluarCertificado(cert, referencia),
	}
	embebidos := parsearEmbebidos(material.CertificadosEmbebidosDER, cert)
	cadena, aspectoCadena := construirCadena(cert, embebidos, anclas, referencia, nil)
	firmante.Cadena = aspectoCadena
	if cadena == nil {
		firmante.Revocacion = domain.AspectoDictamen{Estado: domain.RevocacionNoComprobada, Motivo: "cadena_no_construida"}
	} else {
		firmante.Revocacion = e.evaluarRevocacion(ctx, cadena, material, referencia)
	}
	firmante.SelloTiempo = e.evaluarSellos(ctx, material, anclas, referencia)
	return firmante, true
}

func parsearEmbebidos(der [][]byte, firmante *x509.Certificate) []*x509.Certificate {
	out := make([]*x509.Certificate, 0, len(der))
	for _, d := range der {
		cert, err := x509.ParseCertificate(d)
		if err != nil || bytes.Equal(cert.Raw, firmante.Raw) {
			continue
		}
		out = append(out, cert)
	}
	return out
}

func evaluarCertificado(cert *x509.Certificate, referencia time.Time) domain.AspectoDictamen {
	switch {
	case referencia.Before(cert.NotBefore):
		return domain.AspectoDictamen{Estado: domain.CertificadoNoVigente, Motivo: "aun_no_valido", Fecha: cert.NotBefore.UTC()}
	case referencia.After(cert.NotAfter):
		return domain.AspectoDictamen{Estado: domain.CertificadoNoVigente, Motivo: "caducado", Fecha: cert.NotAfter.UTC()}
	case cert.KeyUsage != 0 && cert.KeyUsage&(x509.KeyUsageDigitalSignature|x509.KeyUsageContentCommitment) == 0:
		return domain.AspectoDictamen{Estado: domain.CertificadoNoPermitido, Motivo: "sin_uso_de_firma"}
	default:
		return domain.AspectoDictamen{Estado: domain.CertificadoVigente, Fecha: cert.NotAfter.UTC()}
	}
}

// construirCadena valida la ruta hasta las anclas locales. Distingue la
// ausencia de ruta (indeterminada, como NO_CERTIFICATE_CHAIN_FOUND de ETSI EN
// 319 102-1) de una ruta defectuosa (no válida).
func construirCadena(hoja *x509.Certificate, intermedios []*x509.Certificate, anclas *x509.CertPool, momento time.Time, usos []x509.ExtKeyUsage) ([]*x509.Certificate, domain.AspectoDictamen) {
	if anclas == nil {
		return nil, domain.AspectoDictamen{Estado: domain.CadenaNoComprobada, Motivo: "sin_anclas_locales"}
	}
	pool := x509.NewCertPool()
	for _, cert := range intermedios {
		pool.AddCert(cert)
	}
	if len(usos) == 0 {
		usos = []x509.ExtKeyUsage{x509.ExtKeyUsageAny}
	}
	cadenas, err := hoja.Verify(x509.VerifyOptions{
		Roots:         anclas,
		Intermediates: pool,
		CurrentTime:   momento,
		KeyUsages:     usos,
	})
	if err != nil {
		var autoridad x509.UnknownAuthorityError
		if errors.As(err, &autoridad) {
			return nil, domain.AspectoDictamen{Estado: domain.CadenaNoComprobada, Motivo: "sin_cadena_hasta_ancla", Fuente: "anclas_locales"}
		}
		var invalido x509.CertificateInvalidError
		if errors.As(err, &invalido) && invalido.Reason == x509.Expired {
			return nil, domain.AspectoDictamen{Estado: domain.CadenaNoValida, Motivo: "certificado_de_cadena_no_vigente", Fuente: "anclas_locales"}
		}
		return nil, domain.AspectoDictamen{Estado: domain.CadenaNoValida, Motivo: "cadena_invalida", Fuente: "anclas_locales"}
	}
	if len(cadenas) == 0 || len(cadenas[0]) == 0 {
		return nil, domain.AspectoDictamen{Estado: domain.CadenaNoComprobada, Motivo: "sin_cadena_hasta_ancla", Fuente: "anclas_locales"}
	}
	return cadenas[0], domain.AspectoDictamen{Estado: domain.CadenaValida, Fuente: "anclas_locales"}
}

// estadoRevocacion resume la evidencia de un certificado concreto.
type estadoRevocacion struct {
	estado string
	motivo string
	fuente string
	fecha  time.Time
}

// evaluarRevocacion comprueba cada certificado de la cadena salvo el ancla.
// Cualquier revocación autenticada prevalece; «vigente» exige evidencia
// autenticada, fresca y aplicable para todos ellos.
func (e *Evaluador) evaluarRevocacion(ctx context.Context, cadena []*x509.Certificate, material domain.MaterialFirma, referencia time.Time) domain.AspectoDictamen {
	if len(cadena) < 2 {
		return domain.AspectoDictamen{Estado: domain.RevocacionNoComprobada, Motivo: "ancla_es_el_firmante"}
	}
	var pendiente *estadoRevocacion
	fuenteHoja := ""
	for i := 0; i < len(cadena)-1; i++ {
		resultado := e.revocacionDe(ctx, cadena[i], cadena[i+1], material, referencia)
		switch resultado.estado {
		case domain.RevocacionRevocado:
			motivo := "certificado_revocado"
			if i > 0 {
				motivo = "intermedio_revocado"
			}
			return domain.AspectoDictamen{Estado: domain.RevocacionRevocado, Motivo: motivo, Fuente: resultado.fuente, Fecha: resultado.fecha}
		case domain.RevocacionVigente:
			if i == 0 {
				fuenteHoja = resultado.fuente
			}
		default:
			if pendiente == nil {
				copia := resultado
				if i > 0 {
					copia.motivo = "intermedio_" + copia.motivo
				}
				pendiente = &copia
			}
		}
	}
	if pendiente != nil {
		return domain.AspectoDictamen{Estado: domain.RevocacionNoComprobada, Motivo: pendiente.motivo, Fuente: pendiente.fuente}
	}
	return domain.AspectoDictamen{Estado: domain.RevocacionVigente, Fuente: fuenteHoja, Fecha: referencia}
}

// revocacionDe consulta, por orden, CRL locales, evidencias embebidas y, solo
// si está configurada, la extensión remota.
func (e *Evaluador) revocacionDe(ctx context.Context, cert, emisor *x509.Certificate, material domain.MaterialFirma, referencia time.Time) estadoRevocacion {
	motivo := "sin_evidencia_de_revocacion"
	if e.cfg.CRL == nil {
		motivo = "sin_crl_locales"
	}
	var vigente *estadoRevocacion
	anotar := func(r estadoRevocacion, fallo string) *estadoRevocacion {
		switch {
		case r.estado == domain.RevocacionRevocado:
			return &r
		case r.estado == domain.RevocacionVigente && vigente == nil:
			copia := r
			vigente = &copia
		case fallo != "":
			motivo = fallo
		}
		return nil
	}

	if e.cfg.CRL != nil {
		crls, err := e.cfg.CRL.CRLsDe(ctx, emisor)
		if err != nil {
			motivo = "crl_locales_no_legibles"
		} else if len(crls) == 0 {
			motivo = "sin_crl_del_emisor"
		}
		for _, crl := range crls {
			if revocado := anotar(evaluarCRL(crl.DER, crl.lista, cert, emisor, referencia, "crl_local")); revocado != nil {
				return *revocado
			}
		}
	}
	for _, der := range material.CRLsEmbebidasDER {
		lista, err := x509.ParseRevocationList(der)
		if err != nil {
			continue
		}
		if revocado := anotar(evaluarCRL(der, lista, cert, emisor, referencia, "crl_embebida")); revocado != nil {
			return *revocado
		}
	}
	for _, der := range material.OCSPEmbebidasDER {
		if revocado := anotar(evaluarOCSP(der, cert, emisor, referencia, "ocsp_embebida")); revocado != nil {
			return *revocado
		}
	}
	if vigente == nil && e.cfg.RevocacionRemota != nil {
		evidencia, err := e.cfg.RevocacionRemota.Fetch(ctx, cert, emisor)
		if err != nil {
			motivo = "revocacion_remota_no_disponible"
		}
		for _, der := range evidencia.CRLs {
			lista, err := x509.ParseRevocationList(der)
			if err != nil {
				continue
			}
			if revocado := anotar(evaluarCRL(der, lista, cert, emisor, referencia, "crl_remota")); revocado != nil {
				return *revocado
			}
		}
		for _, der := range evidencia.OCSPResponses {
			if revocado := anotar(evaluarOCSP(der, cert, emisor, referencia, "ocsp_remota")); revocado != nil {
				return *revocado
			}
		}
	}
	if vigente != nil {
		return *vigente
	}
	return estadoRevocacion{estado: domain.RevocacionNoComprobada, motivo: motivo}
}

func evaluarCRL(der []byte, lista *x509.RevocationList, cert, emisor *x509.Certificate, referencia time.Time, fuente string) (estadoRevocacion, string) {
	if aplicable, motivo := crlAplicable(lista, cert); !aplicable {
		return estadoRevocacion{}, motivo
	}
	resultado, err := revocationclient.ValidateCRLResponse(der, cert, emisor, referencia)
	if err != nil {
		if lista.NextUpdate.Before(referencia) && !lista.NextUpdate.IsZero() {
			return estadoRevocacion{}, "crl_caducada"
		}
		return estadoRevocacion{}, "crl_no_autenticada"
	}
	return mapearValidacion(resultado, fuente)
}

func evaluarOCSP(der []byte, cert, emisor *x509.Certificate, referencia time.Time, fuente string) (estadoRevocacion, string) {
	resultado, err := revocationclient.ValidateBasicOCSPResponse(der, cert, emisor, referencia)
	if err != nil {
		resultado, _, err = revocationclient.ValidateOCSPResponse(der, cert, emisor, referencia)
		if err != nil {
			return estadoRevocacion{}, "ocsp_no_autenticada"
		}
	}
	return mapearValidacion(resultado, fuente)
}

func mapearValidacion(resultado revocationclient.ValidationResult, fuente string) (estadoRevocacion, string) {
	switch resultado.Status {
	case revocationclient.CertificateStatusGood:
		return estadoRevocacion{estado: domain.RevocacionVigente, fuente: fuente}, ""
	case revocationclient.CertificateStatusRevoked:
		return estadoRevocacion{estado: domain.RevocacionRevocado, fuente: fuente, fecha: resultado.RevokedAt.UTC()}, ""
	default:
		return estadoRevocacion{}, "estado_de_revocacion_desconocido"
	}
}
