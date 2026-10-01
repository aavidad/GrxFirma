// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package triphase

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"

	"grxfirma/internal/domain"
)

// Firma trifásica contra el servicio de firma de @firma (SignatureService /
// triphaseSignService), como los clientes CAdEStri/XAdEStri/PAdEStri de
// AutoFirma Java y como la usa FIRe con certificado local:
//
//	pre:  op=pre&cop=sign&format=CAdES&algo=...&cert=...&doc=...&params=...
//	      → datos trifásicos (XML en Base64 URL-safe) con los PRE a firmar
//	firma local PKCS#1 de cada PRE (PK1); la clave nunca sale del equipo
//	post: op=post&...&session=<datos firmados>&doc=...
//	      → "OK NEWID=<firma en Base64 URL-safe>"
//
// "doc" es el propio documento o un identificador que el servidor resuelve.

// SolicitudFirmaServidor describe una firma trifásica contra el servidor.
type SolicitudFirmaServidor struct {
	ServerURL string
	// FormatoLegacy es el formato pedido por la web (CAdEStri, XAdEStri...).
	FormatoLegacy string
	Accion        domain.SignatureAction
	Algoritmo     string
	Datos         []byte
	ExtraParams   map[string]string
}

// EsFormatoTrifasico indica si el formato pedido exige servidor trifásico.
func EsFormatoTrifasico(formato string) bool {
	f := strings.ToLower(strings.TrimSpace(formato))
	return strings.HasSuffix(f, "tri") || strings.HasSuffix(f, "-tri") || f == "adobe pdf triphase"
}

// formatoServidorTrifasico traduce el formato "tri" al que espera el servidor.
func formatoServidorTrifasico(formato string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(formato)) {
	case "cadestri":
		return "CAdES", nil
	case "xadestri":
		return "XAdES", nil
	case "padestri", "adobe pdf triphase":
		return "PAdES", nil
	case "facturaetri":
		return "FacturaE", nil
	case "cades-asic-s-tri":
		return "CAdES-ASiC-S", nil
	case "xades-asic-s-tri":
		return "XAdES-ASiC-S", nil
	case "nonetri":
		return "NONE", nil
	default:
		return "", fmt.Errorf("formato trifásico no soportado: %s", formato)
	}
}

// FirmarConServidor ejecuta la firma trifásica y devuelve la firma final.
func (e *Executor) FirmarConServidor(ctx context.Context, req SolicitudFirmaServidor) ([]byte, error) {
	servidor, err := url.Parse(strings.TrimSpace(req.ServerURL))
	if err != nil || !strings.EqualFold(servidor.Scheme, "https") || servidor.Host == "" {
		return nil, errors.New("el servidor de firma trifásica debe ser una URL HTTPS válida")
	}
	formato, err := formatoServidorTrifasico(req.FormatoLegacy)
	if err != nil {
		return nil, err
	}
	if len(req.Datos) == 0 {
		return nil, errors.New("la firma trifásica necesita el documento o su identificador")
	}
	key, ok := signingKeyFromContext(ctx).(legacyBatchSigningKey)
	if !ok {
		return nil, errors.New("clave de firma no disponible para la firma trifásica")
	}
	chain := key.CertificateChainDER()
	if len(chain) == 0 || len(chain[0]) == 0 {
		return nil, errors.New("el certificado de firma no está disponible")
	}
	algoritmo := strings.TrimSpace(req.Algoritmo)
	if algoritmo == "" {
		algoritmo = "SHA256withRSA"
	}
	cop := "sign"
	switch req.Accion {
	case domain.ActionCoSign:
		cop = "cosign"
	case domain.ActionCounterSign:
		cop = "countersign"
	}
	certs := chain
	if strings.EqualFold(strings.TrimSpace(req.ExtraParams["includeOnlySignningCertificate"]), "true") {
		certs = chain[:1]
	}
	certParam := make([]string, 0, len(certs))
	for _, c := range certs {
		certParam = append(certParam, base64.URLEncoding.EncodeToString(c))
	}
	doc := base64.URLEncoding.EncodeToString(req.Datos)
	params := propiedadesTrifasicas(req.ExtraParams)

	comunes := [][2]string{
		{"cop", cop}, {"format", formato}, {"algo", algoritmo}, {"cert", strings.Join(certParam, ",")},
	}
	if params != "" {
		comunes = append(comunes, [2]string{"params", params})
	}

	preURL, err := buildLegacyBatchURL(servidor.String(), append([][2]string{{"op", "pre"}}, append(comunes, [2]string{"doc", doc})...), sinEscapar)
	if err != nil {
		return nil, err
	}
	pre, err := e.postFormularioTrifasico(ctx, preURL)
	if err != nil {
		trazaServidorTrifasico("pre", "transporte")
		return nil, fmt.Errorf("prefirma trifásica fallida: %w", err)
	}
	if err := errorServidorTrifasico(pre, "prefirma"); err != nil {
		trazaServidorTrifasico("pre", "rechazada")
		return nil, err
	}
	datosTrifasicos, err := decodeLegacyProtocolBase64(strings.TrimSpace(string(pre)))
	if err != nil {
		trazaServidorTrifasico("pre", "respuesta_no_base64")
		return nil, fmt.Errorf("respuesta de prefirma no válida: %w", err)
	}
	trazaServidorTrifasico("pre", "ok")
	firmados, err := signLegacyBatchTriphaseXML(datosTrifasicos, key, algoritmo, req.ExtraParams, nil)
	if err != nil {
		trazaServidorTrifasico("firma_local", "error")
		return nil, fmt.Errorf("firma local de la prefirma fallida: %w", err)
	}

	postURL, err := buildLegacyBatchURL(servidor.String(), append(append([][2]string{{"op", "post"}}, comunes...),
		[2]string{"session", base64.URLEncoding.EncodeToString(firmados)}, [2]string{"doc", doc}), sinEscapar)
	if err != nil {
		return nil, err
	}
	post, err := e.postFormularioTrifasico(ctx, postURL)
	if err != nil {
		trazaServidorTrifasico("post", "transporte")
		return nil, fmt.Errorf("postfirma trifásica fallida: %w", err)
	}
	if err := errorServidorTrifasico(post, "postfirma"); err != nil {
		trazaServidorTrifasico("post", "rechazada")
		return nil, err
	}
	respuesta := strings.TrimSpace(string(post))
	if !strings.HasPrefix(respuesta, "OK") {
		trazaServidorTrifasico("post", "respuesta_inesperada")
		return nil, fmt.Errorf("la firma trifásica no ha finalizado correctamente: %.120s", respuesta)
	}
	_, nuevo, _ := strings.Cut(respuesta, "NEWID=")
	firma, err := decodeLegacyProtocolBase64(strings.TrimSpace(nuevo))
	if err != nil || len(firma) == 0 {
		trazaServidorTrifasico("post", "sin_firma")
		return nil, errors.New("el servidor trifásico no devolvió una firma válida")
	}
	trazaServidorTrifasico("post", "ok")
	return firma, nil
}

// trazaServidorTrifasico registra la fase y su resultado con un vocabulario
// cerrado, sin datos del documento, del certificado ni del servidor.
func trazaServidorTrifasico(fase, resultado string) {
	slog.Info("triphase_server_event", "phase", fase, "outcome", resultado)
}

func sinEscapar(v string) string { return v }

// postFormularioTrifasico envía los parámetros en el cuerpo del POST como
// formulario, igual que AutoFirma Java: el servicio trifásico de @firma y de
// FIRe los lee del cuerpo y responde "ERR-1" si solo van en la URL.
func (e *Executor) postFormularioTrifasico(ctx context.Context, rawURL string) ([]byte, error) {
	_, base, formulario, err := splitLegacyBatchRequest(rawURL)
	if err != nil {
		return nil, err
	}
	body, status, err := e.doLegacyBatchPOST(ctx, base, strings.NewReader(formulario))
	if err != nil {
		return nil, err
	}
	if status < 200 || status > 299 {
		return nil, fmt.Errorf("HTTP %d: %.200s", status, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func errorServidorTrifasico(raw []byte, fase string) error {
	texto := strings.TrimSpace(string(raw))
	if strings.HasPrefix(texto, "ERR-") {
		return fmt.Errorf("el servidor trifásico rechazó la %s: %.200s", fase, texto)
	}
	return nil
}

// propiedadesTrifasicas serializa los parámetros extra como Properties de
// Java en Base64 URL-safe, sin los que ya viajan en la URL.
func propiedadesTrifasicas(extra map[string]string) string {
	claves := make([]string, 0, len(extra))
	for k := range extra {
		switch strings.ToLower(k) {
		case "serverurl", "documentid", "algorithm":
			continue
		}
		if strings.HasPrefix(k, "grxfirma.") {
			continue
		}
		claves = append(claves, k)
	}
	if len(claves) == 0 {
		return ""
	}
	sort.Strings(claves)
	var b strings.Builder
	for _, k := range claves {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(strings.ReplaceAll(extra[k], "\n", "\\n"))
		b.WriteByte('\n')
	}
	return base64.URLEncoding.EncodeToString([]byte(b.String()))
}
