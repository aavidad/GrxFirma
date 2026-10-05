// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/application"
	"grxfirma/internal/ports"
)

func (m *Manejador) handleVeriFactu(ctx context.Context, action string, raw json.RawMessage) respuesta {
	var p struct {
		InputPath string `json:"inputPath"`
		URL       string `json:"url"`
		// ImageB64 lo usa WinUI para enviar la página del PDF ya rasterizada
		// con Windows.Data.Pdf, porque el motor de Windows no tiene Poppler.
		ImageB64 []byte `json:"imageB64"`
	}
	if e := json.Unmarshal(raw, &p); e != nil {
		return respuesta{Action: action, Error: m.t("verifactu.input")}
	}
	if e := ctx.Err(); e != nil {
		return respuesta{Action: action, Error: m.t("verifactu.limit")}
	}
	if action == "read_verifactu_qr" {
		qr, e := m.leerQRVeriFactu(ctx, p.URL, p.InputPath, p.ImageB64)
		if e != nil {
			return respuesta{
				Action:    action,
				ErrorCode: signer.CodigoErrorVeriFactu(e),
				Error:     signer.TraducirErrorVeriFactu(e, func(k string) string { return m.t(k) }),
			}
		}
		return respuesta{OK: true, Action: action, Data: qr}
	}
	if len(p.ImageB64) > 0 {
		return respuesta{Action: action, Error: m.t("verifactu.input")}
	}
	if action == "query_verifactu_qr" {
		data, e := signer.ConsultarQRVeriFactu(ctx, p.URL)
		if e != nil {
			return respuesta{Action: action, Error: signer.TraducirErrorVeriFactu(e, func(k string) string { return m.t(k) })}
		}
		return respuesta{OK: true, Action: action, Data: map[string]json.RawMessage{"response": data}}
	}
	// Carpetas admitidas, pero siempre tras resolver la zona protegida.
	canonical, e := filepath.EvalSymlinks(p.InputPath)
	if e != nil || strings.TrimSpace(p.InputPath) == "" || !filepath.IsAbs(p.InputPath) {
		return respuesta{Action: action, Error: m.t("verifactu.input")}
	}
	if e = rutaEnZonaProhibida(filepath.Clean(canonical)); e != nil {
		return respuesta{Action: action, Error: m.t("verifactu.input")}
	}
	if action == "detect_verifactu" {
		if e = validarRutaLectura(p.InputPath); e != nil {
			return respuesta{Action: action, Error: m.t("verifactu.input")}
		}
		data, e := securefile.ReadFileLimit(p.InputPath, signer.VeriFactuMaxXMLBytes)
		if e != nil {
			return respuesta{Action: action, Error: m.t("verifactu.input")}
		}
		return respuesta{OK: true, Action: action, Data: map[string]any{"inputPath": p.InputPath, "isVerifactu": signer.EsRegistroVeriFactu(data)}}
	}
	result, e := signer.ValidarRutaVeriFactu(ctx, p.InputPath)
	if e != nil {
		return respuesta{Action: action, Error: signer.TraducirErrorVeriFactu(e, func(k string) string { return m.t(k) })}
	}
	result.Localize(func(k string) string { return m.t(k) })
	return respuesta{OK: true, Action: action, Data: result}
}

// leerQRVeriFactu exige exactamente una fuente: la URL, una ruta local a PNG,
// JPEG o PDF, o la imagen ya rasterizada. Ninguna de ellas usa la red.
func (m *Manejador) leerQRVeriFactu(ctx context.Context, url, inputPath string, image []byte) (signer.VeriFactuQR, error) {
	fuentes := 0
	for _, presente := range []bool{url != "", inputPath != "", len(image) > 0} {
		if presente {
			fuentes++
		}
	}
	switch {
	case fuentes != 1:
		return signer.VeriFactuQR{}, signer.ErrorQRVeriFactuFuente()
	case url != "":
		return signer.LeerQRVeriFactu(url)
	case len(image) > 0:
		return signer.LeerQRVeriFactuImagen(ctx, image)
	}
	if strings.TrimSpace(inputPath) != inputPath || !filepath.IsAbs(inputPath) {
		return signer.VeriFactuQR{}, signer.ErrorQRVeriFactuFuente()
	}
	if e := validarRutaLectura(inputPath); e != nil {
		return signer.VeriFactuQR{}, signer.ErrorQRVeriFactuFuente()
	}
	var visor ports.VisualizadorPDF
	if m.Preview != nil {
		visor = visorPDFDesdePreview{m.Preview}
	}
	return signer.LeerQRVeriFactuFichero(ctx, inputPath, visor)
}

// visorPDFDesdePreview reutiliza el caso de uso de la vista previa del sello
// (pdftoppm con sus límites de tamaño, resolución y tiempo).
type visorPDFDesdePreview struct{ uc PdfPreviewUseCase }

func (v visorPDFDesdePreview) RenderizarPagina(ctx context.Context, ruta string, pagina int) (string, float64, float64, int, error) {
	r, e := v.uc.Ejecutar(ctx, application.PdfPreviewCommand{Ruta: ruta, Pagina: pagina})
	if e != nil {
		return "", 0, 0, 0, e
	}
	return r.DataB64, r.Ancho, r.Alto, r.TotalPaginas, nil
}
