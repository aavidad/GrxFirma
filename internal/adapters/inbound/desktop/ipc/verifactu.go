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
)

func (m *Manejador) handleVeriFactu(ctx context.Context, action string, raw json.RawMessage) respuesta {
	var p struct {
		InputPath string `json:"inputPath"`
		URL       string `json:"url"`
	}
	if e := json.Unmarshal(raw, &p); e != nil {
		return respuesta{Action: action, Error: m.t("verifactu.input")}
	}
	if e := ctx.Err(); e != nil {
		return respuesta{Action: action, Error: m.t("verifactu.limit")}
	}
	if action == "read_verifactu_qr" {
		qr, e := signer.LeerQRVeriFactu(p.URL)
		if e != nil {
			return respuesta{Action: action, Error: signer.TraducirErrorVeriFactu(e, func(k string) string { return m.t(k) })}
		}
		return respuesta{OK: true, Action: action, Data: qr}
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
