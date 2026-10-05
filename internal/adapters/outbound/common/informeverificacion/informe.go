// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package informeverificacion genera el informe de validación de una firma:
// un HTML autocontenido e imprimible (o guardable como PDF desde el navegador)
// que el usuario puede adjuntar a un expediente o entregar a un tercero.
package informeverificacion

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"strings"
	"time"
	"unicode"

	"grxfirma/internal/adapters/outbound/common/certutil"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/domain"
)

// Datos reúne lo que se incluye en el informe.
type Datos struct {
	NombreDocumento string
	Contenido       []byte
	Resultado       domain.VerificationResult
	Fecha           time.Time
	VersionApp      string
	// Idioma de la interfaz que pide el informe ("" = castellano).
	Idioma string
	// Zona en la que se escribe la fecha (nil = zona local del sistema).
	Zona *time.Location
}

type aspecto struct {
	Nombre  string
	Estado  string
	Clase   string
	Motivo  string
	Detalle []string
}

// firmante es lo que se lee de cada firmante: nombre, identificador,
// organización, emisor y fecha de la firma con su origen. El DN completo va
// a los detalles técnicos.
type firmante struct {
	Nombre, Identificador, Organizacion, Emisor, Huella, Fecha string
}

type vista struct {
	Idioma                                    string
	Titulo, Veredicto, ClaseVeredicto, Motivo string
	Documento, Huella, Tamano, Fecha, Version string
	Formato, Cobertura                        string
	Aspectos                                  []aspecto
	Firmantes                                 []firmante
	Advertencias, Errores, Detalles           []string
	Evidencias                                []domain.VerificationEvidence
	Rotulos                                   map[string]string
	Pie                                       string
}

// rotulosInforme son las claves de catálogo que usa la plantilla.
var rotulosInforme = []string{
	"report.section.document", "report.field.name", "report.field.hash",
	"report.field.size", "report.field.format", "report.field.coverage",
	"report.field.date", "report.section.checks", "report.section.signers",
	"report.signer.issuer", "report.signer.fingerprint", "report.section.errors",
	"report.signer.identifier", "report.signer.organization", "report.signer.signing_time",
	"report.section.warnings", "report.section.evidence", "report.section.details",
}

// HTML genera el informe. Todos los valores se escapan con html/template: el
// documento verificado puede contener texto controlado por terceros.
func HTML(d Datos) ([]byte, error) {
	r := d.Resultado
	idioma := localizador.Idioma(d.Idioma)
	if idioma == "" {
		idioma = "es"
	}
	loc := localizador.Para(idioma)
	// Los motivos y avisos del motor son literales en castellano que el
	// catálogo traduce usando el propio literal como clave; lo que no está
	// catalogado (valores técnicos, nombres) se deja tal cual.
	tr := func(s string) string { return loc.T(s) }
	huella := sha256.Sum256(d.Contenido)
	rotulos := make(map[string]string, len(rotulosInforme))
	for _, clave := range rotulosInforme {
		rotulos[clave] = loc.T(clave)
	}
	pie := "GrxFirma"
	if version := limpiar(d.VersionApp); version != "" {
		pie += " " + version
	}
	// html/template escapa el marcado, pero no los caracteres de formato:
	// un U+202E en el CN o en un aviso invertiría el texto visible del
	// informe que se entrega a un tercero. Se limpian todas las cadenas.
	v := vista{
		Idioma:       loc.EtiquetaHTML(),
		Titulo:       loc.T("report.title"),
		Motivo:       limpiar(tr(r.Reason)),
		Documento:    limpiar(d.NombreDocumento),
		Huella:       hex.EncodeToString(huella[:]),
		Tamano:       textoTamano(loc, len(d.Contenido)),
		Fecha:        loc.FechaHora(d.Fecha, d.Zona, true),
		Version:      limpiar(d.VersionApp),
		Formato:      limpiar(r.Format),
		Cobertura:    limpiar(textoCobertura(loc, r.Coverage)),
		Firmantes:    firmantesLegibles(loc, d.Zona, r.SignerSummaries),
		Advertencias: limpiarLista(traducirLista(tr, r.Warnings)),
		Errores:      limpiarLista(traducirLista(tr, r.Errors)),
		Rotulos:      rotulos,
		Pie:          loc.T("report.footer", pie),
	}
	// Las comprobaciones solo muestran frases legibles; las evidencias en
	// bruto (clave=valor sin traducción, DN completos) pasan a «Detalles
	// técnicos», donde no se repiten.
	tecnicos := limpiarLista(TraducirDetalles(loc, r.Details))
	for _, a := range []struct {
		clave   string
		aspecto domain.VerificationAspect
	}{
		{"report.aspect.integrity", r.Integrity},
		{"report.aspect.certificate", r.Certificate},
		{"report.aspect.trust", r.Trust},
	} {
		legibles, enBruto := separarDetalles(loc, sinDetallesRepetidos(a.aspecto, r.Details).Details)
		a.aspecto.Details = legibles
		v.Aspectos = append(v.Aspectos, nuevoAspecto(loc, tr, a.clave, a.aspecto))
		tecnicos = anadirTecnicos(tecnicos, nombresDN(r), limpiarLista(enBruto)...)
	}
	evidencias, dnEvidencias := evidenciasLegibles(loc, r.Evidence)
	v.Evidencias = evidencias
	tecnicos = anadirTecnicos(tecnicos, nombresDN(r), dnEvidencias...)
	v.Detalles = anadirTecnicos(tecnicos, nombresDN(r), dnFirmantes(loc, r.SignerSummaries)...)
	switch {
	case r.Valid && len(r.Warnings) == 0:
		v.Veredicto, v.ClaseVeredicto = loc.T("report.verdict.valid"), "ok"
	case r.Valid:
		v.Veredicto, v.ClaseVeredicto = loc.T("report.verdict.valid_warnings"), "aviso"
	case r.Integrity.Status == domain.VerificationStatusInvalid:
		v.Veredicto, v.ClaseVeredicto = loc.T("report.verdict.invalid"), "error"
	case r.Integrity.Status == domain.VerificationStatusValid:
		// El documento no se ha alterado, pero no se ha podido acreditar la
		// vigencia o la confianza del certificado.
		v.Veredicto, v.ClaseVeredicto = loc.T("report.verdict.intact_unproven"), "aviso"
	default:
		v.Veredicto, v.ClaseVeredicto = loc.T("report.verdict.unproven"), "aviso"
	}
	var b bytes.Buffer
	if err := plantilla.Execute(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// sinDetallesRepetidos quita de un aspecto las evidencias que ya salen en
// «Detalles técnicos»: el informe no las repite dos veces.
func sinDetallesRepetidos(a domain.VerificationAspect, generales []string) domain.VerificationAspect {
	if len(a.Details) == 0 || len(generales) == 0 {
		return a
	}
	ya := make(map[string]bool, len(generales))
	for _, d := range generales {
		ya[d] = true
	}
	propios := make([]string, 0, len(a.Details))
	for _, d := range a.Details {
		if !ya[d] {
			propios = append(propios, d)
		}
	}
	a.Details = propios
	return a
}

func nuevoAspecto(loc *localizador.Localizador, tr func(string) string, clave string, a domain.VerificationAspect) aspecto {
	out := aspecto{Nombre: loc.T(clave), Motivo: limpiar(tr(a.Reason)), Detalle: limpiarLista(a.Details)}
	switch a.Status {
	case domain.VerificationStatusValid:
		out.Estado, out.Clase = loc.T("report.status.valid"), "ok"
	case domain.VerificationStatusInvalid:
		out.Estado, out.Clase = loc.T("report.status.invalid"), "error"
	case domain.VerificationStatusWarning:
		out.Estado, out.Clase = loc.T("report.status.warning"), "aviso"
	default:
		out.Estado, out.Clase = loc.T("report.status.unknown"), "nd"
	}
	return out
}

func traducirLista(tr func(string) string, valores []string) []string {
	if len(valores) == 0 {
		return nil
	}
	out := make([]string, len(valores))
	for i, v := range valores {
		out[i] = tr(v)
	}
	return out
}

// limpiar quita los caracteres de control y de formato Unicode (Cc y Cf:
// marcas bidireccionales, espacios de anchura cero, BOM) de un texto que
// puede venir del documento verificado o de su certificado.
func limpiar(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s))
}

func limpiarLista(valores []string) []string {
	if len(valores) == 0 {
		return nil
	}
	out := make([]string, 0, len(valores))
	for _, v := range valores {
		if v = limpiar(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// firmantesLegibles escribe cada firmante como lo lee una persona: el CN
// (o nombre y apellidos), su NIF o identificador, la organización, el
// emisor por su nombre y la fecha de la firma con su origen.
func firmantesLegibles(loc *localizador.Localizador, zona *time.Location, firmantes []domain.VerificationSignerSummary) []firmante {
	if len(firmantes) == 0 {
		return nil
	}
	out := make([]firmante, 0, len(firmantes))
	for _, f := range firmantes {
		sujeto := limpiar(f.Subject)
		titular := leerNombreDistinguido(sujeto)
		emisorDN := limpiar(f.Issuer)
		emisor := leerNombreDistinguido(emisorDN)
		nombreEmisor := emisor.legible(emisorDN)
		if emisor.organizacion != "" && emisor.organizacion != nombreEmisor {
			nombreEmisor += " (" + emisor.organizacion + ")"
		}
		out = append(out, firmante{
			Nombre:        limpiar(titular.legible(sujeto)),
			Identificador: limpiar(titular.identificador),
			Organizacion:  limpiar(titular.organizacion),
			Emisor:        limpiar(nombreEmisor),
			Huella:        limpiar(f.Fingerprint),
			Fecha:         fechaFirma(loc, zona, f),
		})
	}
	return out
}

// fechaFirma escribe la fecha de la firma y de dónde sale: el sello de
// tiempo o la hora que declaró quien firmó. Sin fecha fiable, nada.
func fechaFirma(loc *localizador.Localizador, zona *time.Location, f domain.VerificationSignerSummary) string {
	if f.SigningTime == "" {
		return ""
	}
	instante, err := time.Parse(time.RFC3339, f.SigningTime)
	if err != nil {
		return ""
	}
	fecha := loc.FechaHora(instante, zona, true)
	switch f.SigningTimeSource {
	case domain.SigningTimeSourceTimestamp:
		return loc.T("report.signer.time_from_timestamp", fecha)
	case domain.SigningTimeSourceSignedAttribute:
		return loc.T("report.signer.time_declared", fecha)
	default:
		return ""
	}
}

// dnFirmantes lleva el DN completo de titular y emisor a los detalles
// técnicos, salvo que ya figuren allí.
func dnFirmantes(loc *localizador.Localizador, firmantes []domain.VerificationSignerSummary) []string {
	var out []string
	for _, f := range firmantes {
		if dn := dnLegible(f.Subject); dn != "" {
			out = append(out, loc.T("report.technical.subject_dn", dn))
		}
		if dn := dnLegible(f.Issuer); dn != "" {
			out = append(out, loc.T("report.technical.issuer_dn", dn))
		}
	}
	return out
}

// nombresDN son los DN de titulares y emisores de la verificación.
func nombresDN(r domain.VerificationResult) []string {
	var out []string
	for _, f := range r.SignerSummaries {
		out = append(out, dnLegible(f.Subject), dnLegible(f.Issuer))
	}
	for _, e := range r.Evidence {
		if e.Type == "certificate.subject" || e.Type == "certificate.issuer" {
			out = append(out, dnLegible(e.Summary))
		}
	}
	return out
}

// dnLegible es el DN para leer: con nombre en los atributos que crypto/x509
// deja como OID y hexadecimal (nombre y apellidos de la FNMT, por ejemplo).
func dnLegible(dn string) string {
	return certutil.DNLegible(limpiar(dn))
}

// anadirTecnicos añade líneas a los detalles técnicos sin repetir una línea
// ni un DN que ya figure en otra.
func anadirTecnicos(lista, dns []string, nuevos ...string) []string {
	for _, nuevo := range nuevos {
		repetido := false
		for _, existente := range lista {
			if existente == nuevo {
				repetido = true
				break
			}
			for _, dn := range dns {
				if dn != "" && strings.Contains(nuevo, dn) && strings.Contains(existente, dn) {
					repetido = true
					break
				}
			}
			if repetido {
				break
			}
		}
		if !repetido {
			lista = append(lista, nuevo)
		}
	}
	return lista
}

// evidenciasLegibles traduce el tipo de cada evidencia. Las que solo
// repiten el DN del certificado (ya resumido en «Firmantes») pasan a los
// detalles técnicos.
func evidenciasLegibles(loc *localizador.Localizador, evidencias []domain.VerificationEvidence) ([]domain.VerificationEvidence, []string) {
	if len(evidencias) == 0 {
		return nil, nil
	}
	out := make([]domain.VerificationEvidence, 0, len(evidencias))
	var tecnicos []string
	for _, e := range evidencias {
		tipo, resumen := limpiar(e.Type), limpiar(e.Summary)
		switch tipo {
		case "certificate.subject":
			if resumen = dnLegible(resumen); resumen != "" {
				tecnicos = append(tecnicos, loc.T("report.technical.subject_dn", resumen))
			}
			continue
		case "certificate.issuer":
			if resumen = dnLegible(resumen); resumen != "" {
				tecnicos = append(tecnicos, loc.T("report.technical.issuer_dn", resumen))
			}
			continue
		}
		if clave := "report.evidence." + tipo; loc.T(clave) != clave {
			tipo = loc.T(clave)
		}
		out = append(out, domain.VerificationEvidence{Type: tipo, Summary: resumen})
	}
	if len(out) == 0 {
		out = nil
	}
	return out, tecnicos
}

// separarDetalles traduce los detalles de una comprobación y aparta los que
// siguen siendo claves técnicas (clave=valor sin entrada en el catálogo).
func separarDetalles(loc *localizador.Localizador, lineas []string) (legibles, enBruto []string) {
	for _, linea := range lineas {
		traducida := TraducirDetalle(loc, linea)
		if clave, _, ok := strings.Cut(linea, "="); ok && traducida == detalleConDNLegible(linea) &&
			clave != "" && !strings.ContainsAny(clave, " \t") {
			enBruto = append(enBruto, traducida)
			continue
		}
		legibles = append(legibles, traducida)
	}
	return legibles, enBruto
}

func textoCobertura(loc *localizador.Localizador, c string) string {
	switch c {
	case "full":
		return loc.T("report.coverage.full")
	case "partial":
		return loc.T("report.coverage.partial")
	case "", "unknown":
		return loc.T("report.coverage.unknown")
	default:
		return c
	}
}

// La hoja de estilo cabe en una pantalla de móvil (las huellas, OID y
// evidencias largas se parten en vez de desbordar, y las tablas se apilan
// por debajo de 30rem) y conserva el aspecto de documento al imprimir.
var plantilla = template.Must(template.New("informe").Parse(`<!DOCTYPE html>
<html lang="{{.Idioma}}"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'">
<title>{{.Titulo}}</title>
<style>
*{box-sizing:border-box}
body{font-family:system-ui,-apple-system,"Segoe UI",sans-serif;max-width:52rem;margin:2rem auto;padding:0 1rem;color:#1b1b1b;background:#fff;line-height:1.45;overflow-wrap:anywhere}
h1{font-size:1.4rem;margin-bottom:.2rem}h2{font-size:1.1rem;margin-top:1.8rem;border-bottom:1px solid #ccc;padding-bottom:.2rem}
.veredicto{font-size:1.3rem;font-weight:700;padding:.8rem 1rem;border-radius:.4rem;margin:1rem 0}
.ok{background:#e3f4e6;color:#14532d}.error{background:#fde2e2;color:#7f1d1d}.aviso{background:#fff4d6;color:#713f12}.nd{background:#eee;color:#333}
table{border-collapse:collapse;width:100%;table-layout:fixed}td,th{text-align:left;vertical-align:top;padding:.35rem .5rem;border-bottom:1px solid #e5e5e5;overflow-wrap:anywhere;word-break:break-word}
th{width:34%;font-weight:600}code{font-size:.85rem;word-break:break-all;overflow-wrap:anywhere}.estado{font-weight:700;padding:.1rem .45rem;border-radius:.3rem;display:inline-block}
ul{margin:.3rem 0 .3rem 1.2rem;padding:0}li{overflow-wrap:anywhere}footer{margin-top:2.5rem;font-size:.85rem;color:#555}
@media (max-width:30rem){body{margin:1rem auto;padding:0 .75rem}h1{font-size:1.2rem}.veredicto{font-size:1.1rem;padding:.6rem .75rem}th,td{display:block;width:auto}th{border-bottom:0;padding-bottom:0}td{padding-top:.1rem}}
@media print{body{margin:0;max-width:none}.veredicto{border:1px solid #999}th,td{display:table-cell}th{width:34%}}
</style></head><body>
<h1>{{.Titulo}}</h1>
<div class="veredicto {{.ClaseVeredicto}}">{{.Veredicto}}</div>
{{if .Motivo}}<p>{{.Motivo}}</p>{{end}}
<h2>{{index .Rotulos "report.section.document"}}</h2>
<table>
<tr><th>{{index .Rotulos "report.field.name"}}</th><td>{{.Documento}}</td></tr>
<tr><th>{{index .Rotulos "report.field.hash"}}</th><td><code>{{.Huella}}</code></td></tr>
<tr><th>{{index .Rotulos "report.field.size"}}</th><td>{{.Tamano}}</td></tr>
<tr><th>{{index .Rotulos "report.field.format"}}</th><td>{{.Formato}}</td></tr>
<tr><th>{{index .Rotulos "report.field.coverage"}}</th><td>{{.Cobertura}}</td></tr>
<tr><th>{{index .Rotulos "report.field.date"}}</th><td>{{.Fecha}}</td></tr>
</table>
<h2>{{index .Rotulos "report.section.checks"}}</h2>
<table>
{{range .Aspectos}}<tr><th>{{.Nombre}}</th><td><span class="estado {{.Clase}}">{{.Estado}}</span>{{if .Motivo}}<br>{{.Motivo}}{{end}}{{if .Detalle}}<ul>{{range .Detalle}}<li>{{.}}</li>{{end}}</ul>{{end}}</td></tr>
{{end}}</table>
{{if .Firmantes}}<h2>{{index .Rotulos "report.section.signers"}}</h2>
<table>
{{range .Firmantes}}<tr><th>{{.Nombre}}</th><td>{{if .Fecha}}{{index $.Rotulos "report.signer.signing_time"}}: {{.Fecha}}<br>{{end}}{{if .Identificador}}{{index $.Rotulos "report.signer.identifier"}}: {{.Identificador}}<br>{{end}}{{if .Organizacion}}{{index $.Rotulos "report.signer.organization"}}: {{.Organizacion}}<br>{{end}}{{index $.Rotulos "report.signer.issuer"}}: {{.Emisor}}{{if .Huella}}<br>{{index $.Rotulos "report.signer.fingerprint"}}: <code>{{.Huella}}</code>{{end}}</td></tr>
{{end}}</table>{{end}}
{{if .Errores}}<h2>{{index .Rotulos "report.section.errors"}}</h2><ul>{{range .Errores}}<li>{{.}}</li>{{end}}</ul>{{end}}
{{if .Advertencias}}<h2>{{index .Rotulos "report.section.warnings"}}</h2><ul>{{range .Advertencias}}<li>{{.}}</li>{{end}}</ul>{{end}}
{{if .Evidencias}}<h2>{{index .Rotulos "report.section.evidence"}}</h2><ul>{{range .Evidencias}}<li><strong>{{.Type}}</strong>{{if .Summary}}: {{.Summary}}{{end}}</li>{{end}}</ul>{{end}}
{{if .Detalles}}<h2>{{index .Rotulos "report.section.details"}}</h2><ul>{{range .Detalles}}<li>{{.}}</li>{{end}}</ul>{{end}}
<footer>{{.Pie}}</footer>
</body></html>
`))

// textoTamano escribe el tamaño con la forma singular o plural del catálogo
// («1 byte», «2 bytes»).
func textoTamano(loc *localizador.Localizador, n int) string {
	if n == 1 {
		return loc.T("report.size_byte_one", n)
	}
	return loc.T("report.size_bytes", n)
}
