// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type VeriFactuQR struct {
	URL        string `json:"url"`
	NIF        string `json:"nif"`
	Number     string `json:"numserie"`
	Date       string `json:"fecha"`
	Amount     string `json:"importe"`
	Verifiable bool   `json:"verifiable"`
	Test       bool   `json:"test"`
}

var vfQRNIF = regexp.MustCompile(`^[A-Z0-9]{9}$`)
var vfQRAmount = regexp.MustCompile(`^-?[0-9]{1,12}(?:\.[0-9]{1,2})?$`)

// LeerQRVeriFactu acepta la URL leída por un escáner externo o pegada por el usuario.
// Esta operación no construye un cliente HTTP ni hace peticiones.
func LeerQRVeriFactu(raw string) (VeriFactuQR, error) {
	if len(raw) > 2048 || strings.TrimSpace(raw) != raw {
		return VeriFactuQR{}, vfError("qr_url")
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Opaque != "" || u.User != nil || u.Fragment != "" || strings.Contains(raw, "#") || u.RawPath != "" || u.ForceQuery {
		return VeriFactuQR{}, vfError("qr_url")
	}
	if u.Host != "www2.agenciatributaria.gob.es" && u.Host != "prewww2.aeat.es" {
		return VeriFactuQR{}, vfError("qr_url")
	}
	if u.Path != "/wlpl/TIKE-CONT/ValidarQR" && u.Path != "/wlpl/TIKE-CONT/ValidarQRNoVerifactu" {
		return VeriFactuQR{}, vfError("qr_url")
	}
	q, e := url.ParseQuery(u.RawQuery)
	if e != nil || len(q) != 4 {
		return VeriFactuQR{}, vfError("qr_params")
	}
	for _, key := range []string{"nif", "numserie", "fecha", "importe"} {
		if len(q[key]) != 1 || q.Get(key) == "" {
			return VeriFactuQR{}, vfError("qr_params")
		}
	}
	nif := q.Get("nif")
	if !vfQRNIF.MatchString(nif) || !IdentificadorFiscalEspanolValido(nif) {
		return VeriFactuQR{}, vfError("qr_params")
	}
	number := q.Get("numserie")
	if len(number) > 60 {
		return VeriFactuQR{}, vfError("qr_params")
	}
	for _, c := range number {
		if c < 32 || c > 126 {
			return VeriFactuQR{}, vfError("qr_params")
		}
	}
	date := q.Get("fecha")
	dt, e := time.Parse("02-01-2006", date)
	if e != nil || len(date) != 10 || dt.Format("02-01-2006") != date || dt.Year() < 2024 {
		return VeriFactuQR{}, vfError("qr_params")
	}
	amount := q.Get("importe")
	if !vfQRAmount.MatchString(amount) {
		return VeriFactuQR{}, vfError("qr_params")
	}
	// Se reconstruye la URL a partir de los cuatro datos ya validados.
	u.RawQuery = q.Encode()
	return VeriFactuQR{u.String(), nif, number, date, amount, u.Path == "/wlpl/TIKE-CONT/ValidarQR", u.Host == "prewww2.aeat.es"}, nil
}

// ConsultarQRVeriFactu requiere una acción explícita separada de lectura/validación.
// No utiliza cookies, certificados de cliente, proxy ni almacenes del usuario.
func ConsultarQRVeriFactu(ctx context.Context, raw string) (json.RawMessage, error) {
	qr, e := LeerQRVeriFactu(raw)
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 8 * time.Second, MaxResponseHeaderBytes: 16 * 1024, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return vfQuery(ctx, client, qr)
}
func vfQuery(ctx context.Context, client *http.Client, qr VeriFactuQR) (json.RawMessage, error) {
	u, _ := url.Parse(qr.URL)
	q := u.Query()
	q.Set("formato", "json")
	u.RawQuery = q.Encode()
	request, e := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if e != nil {
		return nil, vfError("qr_service")
	}
	response, e := client.Do(request)
	if e != nil {
		return nil, vfError("qr_service")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, vfError("qr_service")
	}
	body, e := io.ReadAll(io.LimitReader(response.Body, 256*1024+1))
	if e != nil || len(body) > 256*1024 || !json.Valid(body) {
		return nil, vfError("qr_service")
	}
	return json.RawMessage(body), nil
}
