// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package padesviewer

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"strings"

	"grxfirma/internal/ports"
)

var visorTmpl = template.Must(template.New("visor").Parse(`<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Visor PAdES – GrxFirma</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: system-ui, sans-serif; background: #1e1e2e; color: #cdd6f4; min-height: 100vh; display: flex; flex-direction: column; }
  header { background: #181825; padding: 1rem 2rem; display: flex; align-items: center; gap: 1rem; border-bottom: 1px solid #313244; }
  header h1 { font-size: 1.1rem; font-weight: 600; }
  .badge { background: #a6e3a1; color: #1e1e2e; font-size: .7rem; font-weight: 700; border-radius: 4px; padding: 2px 6px; }
  .badge.warn { background: #f38ba8; }
  main { flex: 1; padding: 2rem; display: grid; grid-template-columns: 1fr 320px; gap: 1.5rem; max-width: 1400px; margin: 0 auto; width: 100%; }
  .pdf-panel { background: #313244; border-radius: 8px; overflow: hidden; display: flex; align-items: center; justify-content: center; min-height: 600px; }
  .pdf-panel embed { width: 100%; height: 100%; min-height: 600px; }
  .info-panel { display: flex; flex-direction: column; gap: 1rem; }
  .card { background: #313244; border-radius: 8px; padding: 1.25rem; }
  .card h2 { font-size: .85rem; text-transform: uppercase; letter-spacing: .05em; color: #89b4fa; margin-bottom: .75rem; }
  .field { margin-bottom: .6rem; }
  .field label { font-size: .75rem; color: #a6adc8; display: block; }
  .field span { font-size: .875rem; word-break: break-all; }
  .valid-icon { font-size: 2rem; margin-bottom: .5rem; display: block; }
  .btn { display: inline-block; background: #89b4fa; color: #1e1e2e; border: none; border-radius: 6px; padding: .5rem 1rem; font-weight: 600; cursor: pointer; text-decoration: none; font-size: .85rem; }
  .btn:hover { background: #b4befe; }
  @media (max-width: 768px) { main { grid-template-columns: 1fr; } }
</style>
</head>
<body>
<header>
  <h1>Visor PAdES · GrxFirma</h1>
  {{if .Valido}}<span class="badge">✓ Firma válida</span>{{else}}<span class="badge warn">✗ Firma inválida</span>{{end}}
</header>
<main>
  <div class="pdf-panel">
    <embed src="data:application/pdf;base64,{{.PdfB64}}" type="application/pdf" width="100%" height="100%">
  </div>
  <aside class="info-panel">
    <div class="card">
      <h2>Estado de la firma</h2>
      <span class="valid-icon">{{if .Valido}}✅{{else}}❌{{end}}</span>
      {{if .Razon}}<div class="field"><label>Motivo</label><span>{{.Razon}}</span></div>{{end}}
    </div>
    <div class="card">
      <h2>Firmantes</h2>
      {{range .Firmantes}}
      <div class="field">
        <label>Sujeto</label><span>{{.Sujeto}}</span>
        <label>Emisor</label><span>{{.Emisor}}</span>
        {{if .FechaFirma}}<label>Fecha de firma</label><span>{{.FechaFirma}}</span>{{end}}
      </div>
      {{else}}
      <p style="font-size:.85rem;color:#a6adc8">Sin firmantes detectados.</p>
      {{end}}
    </div>
    <div class="card">
      <h2>Documento</h2>
      <div class="field"><label>Nombre</label><span>{{.NombreDocumento}}</span></div>
      {{if .GuardarURL}}
      <a class="btn" href="{{.GuardarURL}}">Guardar en disco</a>
      {{end}}
    </div>
  </aside>
</main>
</body>
</html>
`))

// DatosFirmante contiene los datos de un firmante para renderizar en el visor.
type DatosFirmante struct {
	Sujeto     string
	Emisor     string
	FechaFirma string
}

// DatosVisor contiene todos los datos necesarios para renderizar el visor PAdES.
type DatosVisor struct {
	PdfB64          string
	Valido          bool
	Razon           string
	Firmantes       []DatosFirmante
	NombreDocumento string
	GuardarURL      string
}

// renderVisor genera el HTML completo del visor PAdES.
func renderVisor(datos DatosVisor, loc ports.Localizador) (string, error) {
	var buf bytes.Buffer
	if err := visorTmpl.Execute(&buf, datos); err != nil {
		return "", fmt.Errorf("renderizando visor PAdES: %w", err)
	}
	return traducirVisor(buf.String(), datos, loc), nil
}

// renderVisorSimple genera el visor PAdES con un PDF ya en base64.
func renderVisorSimple(pdfData []byte, nombre string, guardarURL string, loc ports.Localizador) (string, error) {
	b64 := base64.StdEncoding.EncodeToString(pdfData)
	return renderVisor(DatosVisor{
		PdfB64:          b64,
		Valido:          true,
		NombreDocumento: nombre,
		GuardarURL:      guardarURL,
	}, loc)
}

func traducirVisor(html string, datos DatosVisor, loc ports.Localizador) string {
	t := func(id string) string {
		if loc == nil {
			return id
		}
		return loc.T(id)
	}
	html = strings.ReplaceAll(html, `<html lang="es">`, `<html lang="`+localeVisor(loc)+`">`)
	replacements := []string{
		"Visor PAdES – GrxFirma", t("Visor PAdES – GrxFirma"),
		"Visor PAdES · GrxFirma", t("Visor PAdES · GrxFirma"),
		"✓ Firma válida", t("✓ Firma válida"),
		"✗ Firma inválida", t("✗ Firma inválida"),
		"Estado de la firma", t("Estado de la firma"),
		"Motivo", t("Motivo"),
		"Firmantes", t("Firmantes"),
		"Sujeto", t("Sujeto"),
		"Emisor", t("Emisor"),
		"Fecha de firma", t("Fecha de firma"),
		"Sin firmantes detectados.", t("Sin firmantes detectados."),
		"Documento", t("Documento"),
		"Nombre", t("Nombre"),
		"Guardar en disco", t("Guardar en disco"),
	}
	return strings.NewReplacer(replacements...).Replace(html)
}

func localeVisor(loc ports.Localizador) string {
	type localeAware interface {
		Locale() string
	}
	if loc != nil {
		if l, ok := loc.(localeAware); ok {
			if v := strings.TrimSpace(strings.ToLower(l.Locale())); v != "" {
				return v
			}
		}
	}
	return "es"
}
