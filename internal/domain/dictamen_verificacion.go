// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import "time"

// ContratoDictamenVerificacion versiona el dictamen explícito de verificación.
// Un consumidor debe rechazar cualquier otro valor en lugar de interpretarlo.
const ContratoDictamenVerificacion = "autofirmav2.dictamen-verificacion.v1"

// EstadoDictamen es el veredicto global trivalente. Solo EstadoDictamenValida
// acredita la firma; la incertidumbre nunca se colapsa en validez.
type EstadoDictamen string

const (
	EstadoDictamenValida        EstadoDictamen = "valida"
	EstadoDictamenNoValida      EstadoDictamen = "no_valida"
	EstadoDictamenIndeterminada EstadoDictamen = "indeterminada"
)

// MotivoDictamen es el catálogo cerrado que explica el veredicto global.
type MotivoDictamen string

const (
	MotivoDictamenVerificada MotivoDictamen = "verificada"

	// Defectos concluyentes: estado no_valida.
	MotivoDictamenIntegridadNoValida  MotivoDictamen = "integridad_no_valida"
	MotivoDictamenCertificadoNoValido MotivoDictamen = "certificado_no_valido"
	MotivoDictamenConfianzaNoValida   MotivoDictamen = "confianza_no_valida"

	// Comprobaciones necesarias que no concluyen: estado indeterminada.
	MotivoDictamenIntegridadParcial           MotivoDictamen = "integridad_parcial"
	MotivoDictamenFirmanteNoIdentificado      MotivoDictamen = "firmante_no_identificado"
	MotivoDictamenCertificadoNoAcreditado     MotivoDictamen = "certificado_no_acreditado"
	MotivoDictamenConfianzaNoAcreditada       MotivoDictamen = "confianza_no_acreditada"
	MotivoDictamenRevocacionNoAcreditada      MotivoDictamen = "revocacion_no_acreditada"
	MotivoDictamenSelloTiempoNoAcreditado     MotivoDictamen = "sello_tiempo_no_acreditado"
	MotivoDictamenVinculoOriginalNoAcreditado MotivoDictamen = "vinculo_original_no_acreditado"
)

// Estados de cada aspecto del dictamen. Se usan cadenas cerradas para que el
// contrato JSON sea autoexplicativo y estable.
const (
	// Integridad criptográfica del contenido firmado.
	IntegridadValida   = "valida"
	IntegridadParcial  = "parcial"
	IntegridadNoValida = "no_valida"

	// Cadena de certificación hasta las anclas locales configuradas.
	CadenaValida       = "valida"
	CadenaNoValida     = "no_valida"
	CadenaNoComprobada = "no_comprobada"

	// Vigencia temporal y uso de clave del certificado firmante.
	CertificadoVigente      = "vigente"
	CertificadoNoVigente    = "no_vigente"
	CertificadoNoPermitido  = "uso_no_permitido"
	CertificadoNoComprobado = "no_comprobado"

	// Revocación del certificado firmante y de sus intermedios.
	RevocacionVigente      = "vigente"
	RevocacionRevocado     = "revocado"
	RevocacionNoComprobada = "no_comprobada"

	// Sello de tiempo de la firma. No es obligatorio.
	SelloNoPresente   = "no_presente"
	SelloValido       = "valido"
	SelloNoValido     = "no_valido"
	SelloNoComprobado = "no_comprobado"

	// Vínculo entre la firma y el original aportado.
	VinculoAcreditado   = "acreditado"
	VinculoNoAcreditado = "no_acreditado"
	VinculoNoAportado   = "no_aportado"

	// Extensiones remotas opcionales.
	ExtensionDesactivada = "desactivada"
	ExtensionActiva      = "activa"
)

// AspectoDictamen describe una comprobación con su estado cerrado, un motivo
// técnico estable (snake_case, sin datos personales) y, si procede, la fuente
// de la evidencia y el instante que certifica.
type AspectoDictamen struct {
	Estado string
	Motivo string
	Fuente string
	Fecha  time.Time
}

// DictamenFirmante recoge la evaluación de un firmante concreto.
type DictamenFirmante struct {
	CertificadoHuellaSHA256 string
	Serie                   string
	Asunto                  string
	Emisor                  string
	Cadena                  AspectoDictamen
	Certificado             AspectoDictamen
	Revocacion              AspectoDictamen
	SelloTiempo             AspectoDictamen
}

// ExtensionesDictamen declara qué fuentes remotas opcionales intervinieron.
// Por defecto ambas están desactivadas: el dictamen es autónomo.
type ExtensionesDictamen struct {
	RevocacionRemota  string
	SelloTiempoRemoto string
}

// DictamenVerificacion es el resultado explícito y completo de una
// verificación autónoma. Los aspectos agregados resumen a todos los
// firmantes; el veredicto global se obtiene con Componer.
type DictamenVerificacion struct {
	Contrato     string
	Estado       EstadoDictamen
	Motivo       MotivoDictamen
	Formato      string
	ComprobadoEn time.Time

	Integridad      AspectoDictamen
	Cadena          AspectoDictamen
	Certificado     AspectoDictamen
	Revocacion      AspectoDictamen
	SelloTiempo     AspectoDictamen
	VinculoOriginal AspectoDictamen

	HuellaFirmadoSHA256     string
	HuellaOriginalSHA256    string
	CertificadoHuellaSHA256 string

	Firmantes   []DictamenFirmante
	Extensiones ExtensionesDictamen
}

// Componer agrega los aspectos de los firmantes y deriva el veredicto global
// con precedencia fija: primero los defectos concluyentes, después cualquier
// comprobación no concluida. Es una función pura y determinista.
func (d DictamenVerificacion) Componer() DictamenVerificacion {
	d.Contrato = ContratoDictamenVerificacion
	d.Cadena = agregarAspecto(d.Firmantes, func(f DictamenFirmante) AspectoDictamen { return f.Cadena },
		[]string{CadenaNoValida, CadenaNoComprobada, CadenaValida}, CadenaNoComprobada, "sin_firmantes")
	d.Certificado = agregarAspecto(d.Firmantes, func(f DictamenFirmante) AspectoDictamen { return f.Certificado },
		[]string{CertificadoNoPermitido, CertificadoNoVigente, CertificadoNoComprobado, CertificadoVigente}, CertificadoNoComprobado, "sin_firmantes")
	d.Revocacion = agregarAspecto(d.Firmantes, func(f DictamenFirmante) AspectoDictamen { return f.Revocacion },
		[]string{RevocacionRevocado, RevocacionNoComprobada, RevocacionVigente}, RevocacionNoComprobada, "sin_firmantes")
	d.SelloTiempo = agregarAspecto(d.Firmantes, func(f DictamenFirmante) AspectoDictamen { return f.SelloTiempo },
		[]string{SelloNoValido, SelloNoComprobado, SelloNoPresente, SelloValido}, SelloNoPresente, "sin_firmantes")
	d.CertificadoHuellaSHA256 = ""
	if len(d.Firmantes) == 1 {
		d.CertificadoHuellaSHA256 = d.Firmantes[0].CertificadoHuellaSHA256
	}
	if d.Extensiones.RevocacionRemota == "" {
		d.Extensiones.RevocacionRemota = ExtensionDesactivada
	}
	if d.Extensiones.SelloTiempoRemoto == "" {
		d.Extensiones.SelloTiempoRemoto = ExtensionDesactivada
	}
	d.Estado, d.Motivo = d.veredicto()
	return d
}

func (d DictamenVerificacion) veredicto() (EstadoDictamen, MotivoDictamen) {
	switch {
	case d.Integridad.Estado == IntegridadNoValida:
		return EstadoDictamenNoValida, MotivoDictamenIntegridadNoValida
	case d.Certificado.Estado == CertificadoNoVigente || d.Certificado.Estado == CertificadoNoPermitido ||
		d.Revocacion.Estado == RevocacionRevocado:
		return EstadoDictamenNoValida, MotivoDictamenCertificadoNoValido
	case d.Cadena.Estado == CadenaNoValida:
		return EstadoDictamenNoValida, MotivoDictamenConfianzaNoValida
	case d.Integridad.Estado != IntegridadValida:
		return EstadoDictamenIndeterminada, MotivoDictamenIntegridadParcial
	case len(d.Firmantes) == 0:
		return EstadoDictamenIndeterminada, MotivoDictamenFirmanteNoIdentificado
	case d.Certificado.Estado != CertificadoVigente:
		return EstadoDictamenIndeterminada, MotivoDictamenCertificadoNoAcreditado
	case d.Cadena.Estado != CadenaValida:
		return EstadoDictamenIndeterminada, MotivoDictamenConfianzaNoAcreditada
	case d.Revocacion.Estado != RevocacionVigente:
		return EstadoDictamenIndeterminada, MotivoDictamenRevocacionNoAcreditada
	case d.SelloTiempo.Estado == SelloNoValido:
		// Un sello presente pero falso o ajeno a la firma impide concluir. Un
		// sello ausente o no comprobable no bloquea: no es obligatorio y la
		// validez se evalúa en el instante de la comprobación.
		return EstadoDictamenIndeterminada, MotivoDictamenSelloTiempoNoAcreditado
	case d.VinculoOriginal.Estado == VinculoNoAcreditado:
		return EstadoDictamenIndeterminada, MotivoDictamenVinculoOriginalNoAcreditado
	case d.VinculoOriginal.Estado != VinculoAcreditado && d.VinculoOriginal.Estado != VinculoNoAportado:
		return EstadoDictamenIndeterminada, MotivoDictamenVinculoOriginalNoAcreditado
	default:
		return EstadoDictamenValida, MotivoDictamenVerificada
	}
}

// agregarAspecto devuelve el peor estado según el orden indicado (de peor a
// mejor). Un estado desconocido se trata como vacio y se sustituye por el
// estado por defecto, nunca por uno favorable.
func agregarAspecto(firmantes []DictamenFirmante, aspecto func(DictamenFirmante) AspectoDictamen, orden []string, porDefecto, motivoVacio string) AspectoDictamen {
	if len(firmantes) == 0 {
		return AspectoDictamen{Estado: porDefecto, Motivo: motivoVacio}
	}
	rango := func(estado string) int {
		for i, candidato := range orden {
			if candidato == estado {
				return i
			}
		}
		return -1
	}
	peor := AspectoDictamen{}
	peorRango := len(orden)
	for _, firmante := range firmantes {
		actual := aspecto(firmante)
		r := rango(actual.Estado)
		if r < 0 {
			actual = AspectoDictamen{Estado: porDefecto, Motivo: "estado_desconocido"}
			r = rango(porDefecto)
		}
		if r < peorRango {
			peor, peorRango = actual, r
		}
	}
	return peor
}

// MaterialFirma conserva, fuera de cualquier serialización, el material que
// un verificador de formato halló para un firmante. Permite evaluar cadena,
// revocación y sello con fuentes locales sin volver a analizar el formato.
type MaterialFirma struct {
	CertificadoDER           []byte
	CertificadosEmbebidosDER [][]byte
	// ValorFirma son los bytes sobre cuyo resumen se emite el sello de
	// tiempo de la firma (SignerInfo.signature en CMS).
	ValorFirma []byte
	// SellosTiempoDER contiene los TimeStampToken RFC 3161 no firmados.
	SellosTiempoDER [][]byte
	// SelloNoEvaluable indica que el formato contiene un sello que este
	// verificador todavía no sabe evaluar (por ejemplo XAdES-T).
	SelloNoEvaluable bool
	// Evidencias de revocación embebidas en la firma (CAdES/PAdES LT).
	CRLsEmbebidasDER [][]byte
	OCSPEmbebidasDER [][]byte
}

// CoberturaPDF describe una firma PDF: los bytes [0, InicioHueco) preceden al
// hueco /Contents y están cubiertos por la firma.
type CoberturaPDF struct {
	InicioHueco int
	FinRevision int
}

// MaterialVerificacion agrupa el material de todos los firmantes y los datos
// de cobertura necesarios para acreditar el vínculo con un original.
type MaterialVerificacion struct {
	Firmas []MaterialFirma
	// HuellasContenidoFirmado son las SHA-256 (hex) de los contenidos cuyo
	// resumen se comprobó criptográficamente (encapsulado o externo en CMS).
	HuellasContenidoFirmado []string
	CoberturasPDF           []CoberturaPDF
}

// Anadir incorpora el material de otra verificación parcial (por ejemplo,
// cada firma de un PDF) sin compartir memoria con el origen.
func (m MaterialVerificacion) Anadir(otro MaterialVerificacion) MaterialVerificacion {
	m.Firmas = append(m.Firmas, otro.Firmas...)
	m.HuellasContenidoFirmado = append(m.HuellasContenidoFirmado, otro.HuellasContenidoFirmado...)
	m.CoberturasPDF = append(m.CoberturasPDF, otro.CoberturasPDF...)
	return m
}
