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
	"fmt"
	"html/template"
	"strings"
	"time"
	"unicode"

	"grxfirma/internal/domain"
)

// Datos reúne lo que se incluye en el informe.
type Datos struct {
	NombreDocumento string
	Contenido       []byte
	Resultado       domain.VerificationResult
	Fecha           time.Time
	VersionApp      string
}

type aspecto struct {
	Nombre  string
	Estado  string
	Clase   string
	Motivo  string
	Detalle []string
}

type vista struct {
	Titulo, Veredicto, ClaseVeredicto, Motivo string
	Documento, Huella, Tamano, Fecha, Version string
	Formato, Cobertura                        string
	Aspectos                                  []aspecto
	Firmantes                                 []domain.VerificationSignerSummary
	Advertencias, Errores, Detalles           []string
	Evidencias                                []domain.VerificationEvidence
}

// HTML genera el informe. Todos los valores se escapan con html/template: el
// documento verificado puede contener texto controlado por terceros.
func HTML(d Datos) ([]byte, error) {
	r := d.Resultado
	huella := sha256.Sum256(d.Contenido)
	// html/template escapa el marcado, pero no los caracteres de formato:
	// un U+202E en el CN o en un aviso invertiría el texto visible del
	// informe que se entrega a un tercero. Se limpian todas las cadenas.
	v := vista{
		Titulo:       "Informe de validación de firma electrónica",
		Motivo:       limpiar(r.Reason),
		Documento:    limpiar(d.NombreDocumento),
		Huella:       hex.EncodeToString(huella[:]),
		Tamano:       fmt.Sprintf("%d bytes", len(d.Contenido)),
		Fecha:        d.Fecha.Local().Format("02/01/2006 15:04:05 MST"),
		Version:      limpiar(d.VersionApp),
		Formato:      limpiar(r.Format),
		Cobertura:    limpiar(textoCobertura(r.Coverage)),
		Firmantes:    limpiarFirmantes(r.SignerSummaries),
		Advertencias: limpiarLista(r.Warnings),
		Errores:      limpiarLista(r.Errors),
		Detalles:     limpiarLista(r.Details),
		Evidencias:   limpiarEvidencias(r.Evidence),
		Aspectos: []aspecto{
			nuevoAspecto("Integridad del documento", r.Integrity),
			nuevoAspecto("Vigencia y revocación del certificado", r.Certificate),
			nuevoAspecto("Confianza en el emisor", r.Trust),
		},
	}
	switch {
	case r.Valid && len(r.Warnings) == 0:
		v.Veredicto, v.ClaseVeredicto = "FIRMA VÁLIDA", "ok"
	case r.Valid:
		v.Veredicto, v.ClaseVeredicto = "FIRMA VÁLIDA CON ADVERTENCIAS", "aviso"
	case r.Integrity.Status == domain.VerificationStatusInvalid:
		v.Veredicto, v.ClaseVeredicto = "FIRMA NO VÁLIDA", "error"
	case r.Integrity.Status == domain.VerificationStatusValid:
		// El documento no se ha alterado, pero no se ha podido acreditar la
		// vigencia o la confianza del certificado.
		v.Veredicto, v.ClaseVeredicto = "FIRMA ÍNTEGRA · VALIDEZ DEL CERTIFICADO NO ACREDITADA", "aviso"
	default:
		v.Veredicto, v.ClaseVeredicto = "VALIDEZ NO ACREDITADA", "aviso"
	}
	var b bytes.Buffer
	if err := plantilla.Execute(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func nuevoAspecto(nombre string, a domain.VerificationAspect) aspecto {
	out := aspecto{Nombre: nombre, Motivo: limpiar(a.Reason), Detalle: limpiarLista(a.Details)}
	switch a.Status {
	case domain.VerificationStatusValid:
		out.Estado, out.Clase = "Correcto", "ok"
	case domain.VerificationStatusInvalid:
		out.Estado, out.Clase = "Incorrecto", "error"
	case domain.VerificationStatusWarning:
		out.Estado, out.Clase = "Con advertencias", "aviso"
	default:
		out.Estado, out.Clase = "No comprobado", "nd"
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

func textoCobertura(c string) string {
	switch c {
	case "full":
		return "Completa: se ha comprobado todo el contenido firmado"
	case "partial":
		return "Parcial: parte del contenido no se ha podido comprobar"
	case "", "unknown":
		return "No acreditada"
	default:
		return c
	}
}

var plantilla = template.Must(template.New("informe").Parse(`<!DOCTYPE html>
<html lang="es"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'">
<title>{{.Titulo}}</title>
<style>
body{font-family:system-ui,-apple-system,"Segoe UI",sans-serif;max-width:52rem;margin:2rem auto;padding:0 1rem;color:#1b1b1b;line-height:1.45}
h1{font-size:1.4rem;margin-bottom:.2rem}h2{font-size:1.1rem;margin-top:1.8rem;border-bottom:1px solid #ccc;padding-bottom:.2rem}
.veredicto{font-size:1.3rem;font-weight:700;padding:.8rem 1rem;border-radius:.4rem;margin:1rem 0}
.ok{background:#e3f4e6;color:#14532d}.error{background:#fde2e2;color:#7f1d1d}.aviso{background:#fff4d6;color:#713f12}.nd{background:#eee;color:#333}
table{border-collapse:collapse;width:100%}td,th{text-align:left;vertical-align:top;padding:.35rem .5rem;border-bottom:1px solid #e5e5e5}
th{width:34%;font-weight:600}code{font-size:.85rem;word-break:break-all}.estado{font-weight:700;padding:.1rem .45rem;border-radius:.3rem}
ul{margin:.3rem 0 .3rem 1.2rem;padding:0}footer{margin-top:2.5rem;font-size:.85rem;color:#555}
@media print{body{margin:0}.veredicto{border:1px solid #999}}
</style></head><body>
<h1>{{.Titulo}}</h1>
<div class="veredicto {{.ClaseVeredicto}}">{{.Veredicto}}</div>
{{if .Motivo}}<p>{{.Motivo}}</p>{{end}}
<h2>Documento verificado</h2>
<table>
<tr><th>Nombre</th><td>{{.Documento}}</td></tr>
<tr><th>Huella SHA-256</th><td><code>{{.Huella}}</code></td></tr>
<tr><th>Tamaño</th><td>{{.Tamano}}</td></tr>
<tr><th>Formato de firma</th><td>{{.Formato}}</td></tr>
<tr><th>Cobertura</th><td>{{.Cobertura}}</td></tr>
<tr><th>Fecha de la verificación</th><td>{{.Fecha}}</td></tr>
</table>
<h2>Comprobaciones</h2>
<table>
{{range .Aspectos}}<tr><th>{{.Nombre}}</th><td><span class="estado {{.Clase}}">{{.Estado}}</span>{{if .Motivo}}<br>{{.Motivo}}{{end}}{{if .Detalle}}<ul>{{range .Detalle}}<li>{{.}}</li>{{end}}</ul>{{end}}</td></tr>
{{end}}</table>
{{if .Firmantes}}<h2>Firmantes</h2>
<table>
{{range .Firmantes}}<tr><th>{{.Subject}}</th><td>Emisor: {{.Issuer}}{{if .Fingerprint}}<br>Huella: <code>{{.Fingerprint}}</code>{{end}}</td></tr>
{{end}}</table>{{end}}
{{if .Errores}}<h2>Errores</h2><ul>{{range .Errores}}<li>{{.}}</li>{{end}}</ul>{{end}}
{{if .Advertencias}}<h2>Advertencias</h2><ul>{{range .Advertencias}}<li>{{.}}</li>{{end}}</ul>{{end}}
{{if .Evidencias}}<h2>Evidencias</h2><ul>{{range .Evidencias}}<li><strong>{{.Type}}</strong>{{if .Summary}}: {{.Summary}}{{end}}</li>{{end}}</ul>{{end}}
{{if .Detalles}}<h2>Detalles técnicos</h2><ul>{{range .Detalles}}<li>{{.}}</li>{{end}}</ul>{{end}}
<footer>Generado por GrxFirma{{if .Version}} {{.Version}}{{end}}. Este informe describe el resultado de la verificación local en la fecha indicada; la huella SHA-256 identifica el fichero exacto verificado. No sustituye a un servicio de validación cualificado.</footer>
</body></html>
`))
