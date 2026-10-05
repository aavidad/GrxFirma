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

type vista struct {
	Idioma                                    string
	Titulo, Veredicto, ClaseVeredicto, Motivo string
	Documento, Huella, Tamano, Fecha, Version string
	Formato, Cobertura                        string
	Aspectos                                  []aspecto
	Firmantes                                 []domain.VerificationSignerSummary
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
		Tamano:       loc.T("report.size_bytes", len(d.Contenido)),
		Fecha:        loc.FechaHora(d.Fecha, d.Zona, true),
		Version:      limpiar(d.VersionApp),
		Formato:      limpiar(r.Format),
		Cobertura:    limpiar(textoCobertura(loc, r.Coverage)),
		Firmantes:    limpiarFirmantes(r.SignerSummaries),
		Advertencias: limpiarLista(traducirLista(tr, r.Warnings)),
		Errores:      limpiarLista(traducirLista(tr, r.Errors)),
		Detalles:     limpiarLista(traducirLista(tr, r.Details)),
		Evidencias:   limpiarEvidencias(r.Evidence),
		Rotulos:      rotulos,
		Pie:          loc.T("report.footer", pie),
		Aspectos: []aspecto{
			nuevoAspecto(loc, tr, "report.aspect.integrity", r.Integrity),
			nuevoAspecto(loc, tr, "report.aspect.certificate", r.Certificate),
			nuevoAspecto(loc, tr, "report.aspect.trust", r.Trust),
		},
	}
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

func nuevoAspecto(loc *localizador.Localizador, tr func(string) string, clave string, a domain.VerificationAspect) aspecto {
	out := aspecto{Nombre: loc.T(clave), Motivo: limpiar(tr(a.Reason)), Detalle: limpiarLista(traducirLista(tr, a.Details))}
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

func limpiarFirmantes(firmantes []domain.VerificationSignerSummary) []domain.VerificationSignerSummary {
	if len(firmantes) == 0 {
		return nil
	}
	out := make([]domain.VerificationSignerSummary, 0, len(firmantes))
	for _, f := range firmantes {
		out = append(out, domain.VerificationSignerSummary{
			ID: limpiar(f.ID), Subject: limpiar(f.Subject), Issuer: limpiar(f.Issuer), Fingerprint: limpiar(f.Fingerprint),
		})
	}
	return out
}

func limpiarEvidencias(evidencias []domain.VerificationEvidence) []domain.VerificationEvidence {
	if len(evidencias) == 0 {
		return nil
	}
	out := make([]domain.VerificationEvidence, 0, len(evidencias))
	for _, e := range evidencias {
		out = append(out, domain.VerificationEvidence{Type: limpiar(e.Type), Summary: limpiar(e.Summary)})
	}
	return out
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
{{range .Firmantes}}<tr><th>{{.Subject}}</th><td>{{index $.Rotulos "report.signer.issuer"}}: {{.Issuer}}{{if .Fingerprint}}<br>{{index $.Rotulos "report.signer.fingerprint"}}: <code>{{.Fingerprint}}</code>{{end}}</td></tr>
{{end}}</table>{{end}}
{{if .Errores}}<h2>{{index .Rotulos "report.section.errors"}}</h2><ul>{{range .Errores}}<li>{{.}}</li>{{end}}</ul>{{end}}
{{if .Advertencias}}<h2>{{index .Rotulos "report.section.warnings"}}</h2><ul>{{range .Advertencias}}<li>{{.}}</li>{{end}}</ul>{{end}}
{{if .Evidencias}}<h2>{{index .Rotulos "report.section.evidence"}}</h2><ul>{{range .Evidencias}}<li><strong>{{.Type}}</strong>{{if .Summary}}: {{.Summary}}{{end}}</li>{{end}}</ul>{{end}}
{{if .Detalles}}<h2>{{index .Rotulos "report.section.details"}}</h2><ul>{{range .Detalles}}<li>{{.}}</li>{{end}}</ul>{{end}}
<footer>{{.Pie}}</footer>
</body></html>
`))
