// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package intermediatehttp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/common/limits"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// Transporte implementa ports.ResultTransport con net/http.
type Transporte struct {
	client          *http.Client
	maxPayloadBytes int64
}

const (
	maxEndpointBytes          = 4096
	maxRedirects              = 5
	maxControlResponseBytes   = 4096
	maxConfiguredPayloadBytes = 1 << 30
)

// New construye el transporte HTTP. Si client es nil se crea uno aplicando
// HTTPTimeoutSec de los limites configurados via variables de entorno GRXFIRMA_*.
// Si client ya tiene un Timeout configurado (distinto de cero), se respeta sin modificar.
func New(client *http.Client) *Transporte {
	return newWithLimits(client, limits.FromEnv(limits.Default()))
}

func newWithLimits(client *http.Client, configured limits.Limits) *Transporte {
	defaults := limits.Default()
	if configured.HTTPTimeoutSec <= 0 {
		configured.HTTPTimeoutSec = defaults.HTTPTimeoutSec
	}
	if configured.MaxPayloadBytes <= 0 || configured.MaxPayloadBytes > maxConfiguredPayloadBytes {
		configured.MaxPayloadBytes = defaults.MaxPayloadBytes
	}

	if client == nil {
		client = &http.Client{}
	} else {
		clone := *client
		client = &clone
	}
	if client.Timeout <= 0 {
		client.Timeout = time.Duration(configured.HTTPTimeoutSec) * time.Second
	}
	client.CheckRedirect = secureRedirectPolicy(client.CheckRedirect)

	return &Transporte{
		client:          client,
		maxPayloadBytes: configured.MaxPayloadBytes,
	}
}

// Upload sube un resultado ya serializado al endpoint configurado en la sesion.
func (t *Transporte) Upload(ctx context.Context, session domain.ExchangeSession, data []byte) error {
	if err := t.validateSession(session); err != nil {
		return err
	}
	if err := t.checkPayload(data); err != nil {
		return err
	}
	valores := url.Values{}
	valores.Set("op", "put")
	valores.Set("v", "1_0")
	valores.Set("id", session.RequestID)
	valores.Set("dat", string(data))
	return t.sendOK(ctx, session.UploadEndpoint, valores)
}

// Retrieve descarga la peticion pendiente del endpoint configurado en la sesion.
func (t *Transporte) Retrieve(ctx context.Context, session domain.ExchangeSession) ([]byte, error) {
	if err := t.validateSession(session); err != nil {
		return nil, err
	}
	endpoint, err := parseEndpoint(session.RetrieveEndpoint)
	if err != nil {
		return nil, fmt.Errorf("retrieve endpoint no valido: %w", err)
	}
	query := endpoint.Query()
	query.Set("op", "get")
	query.Set("v", "1_0")
	query.Set("id", session.RequestID)
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear la peticion de retrieve: %w", err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fallo en retrieve: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = readBounded(resp.Body, maxControlResponseBytes)
		return nil, fmt.Errorf("retrieve devolvio estado %d", resp.StatusCode)
	}
	body, err := readBounded(resp.Body, t.maxPayloadBytes)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer la respuesta de retrieve: %w", err)
	}
	return body, nil
}

// SendWait envia el marcador de espera activa al servidor intermedio.
func (t *Transporte) SendWait(ctx context.Context, session domain.ExchangeSession) error {
	if err := t.validateSession(session); err != nil {
		return err
	}
	valores := url.Values{}
	valores.Set("op", "put")
	valores.Set("v", "1_0")
	valores.Set("id", session.RequestID)
	valores.Set("dat", "#WAIT")
	return t.sendOK(ctx, session.UploadEndpoint, valores)
}

// Cancel informa al servidor de que la operacion ha sido cancelada.
func (t *Transporte) Cancel(ctx context.Context, session domain.ExchangeSession) error {
	if err := t.validateSession(session); err != nil {
		return err
	}
	valores := url.Values{}
	valores.Set("op", "cancel")
	valores.Set("v", "1_0")
	valores.Set("id", session.RequestID)
	return t.sendOK(ctx, session.UploadEndpoint, valores)
}

func (t *Transporte) sendOK(ctx context.Context, endpoint string, valores url.Values) error {
	parsedEndpoint, err := parseEndpoint(endpoint)
	if err != nil {
		return fmt.Errorf("endpoint HTTP no valido: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, parsedEndpoint.String(), bytes.NewBufferString(valores.Encode()))
	if err != nil {
		return fmt.Errorf("no se pudo crear la peticion HTTP: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("fallo HTTP: %w", err)
	}
	defer resp.Body.Close()

	body, err := readBounded(resp.Body, maxControlResponseBytes)
	if err != nil {
		return fmt.Errorf("no se pudo leer la respuesta HTTP: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("el servidor devolvio estado %d", resp.StatusCode)
	}
	if strings.TrimSpace(string(body)) != "OK" {
		return errors.New("el servidor no devolvio la confirmacion esperada")
	}
	return nil
}

func (t *Transporte) validateSession(session domain.ExchangeSession) error {
	if t == nil || t.client == nil || t.maxPayloadBytes <= 0 {
		return errors.New("transporte HTTP no configurado")
	}
	if err := session.Validate(); err != nil {
		return err
	}
	if err := validateSessionValue("identificador de solicitud", session.RequestID, 1024); err != nil {
		return err
	}
	if err := validateSessionValue("clave de sesion", session.SessionKey, 4096); err != nil {
		return err
	}
	if _, err := parseEndpoint(session.UploadEndpoint); err != nil {
		return fmt.Errorf("upload endpoint no valido: %w", err)
	}
	if _, err := parseEndpoint(session.RetrieveEndpoint); err != nil {
		return fmt.Errorf("retrieve endpoint no valido: %w", err)
	}
	return nil
}

func (t *Transporte) checkPayload(data []byte) error {
	if int64(len(data)) > t.maxPayloadBytes {
		return &limits.ErrPayloadExcedido{Tamaño: int64(len(data)), Maximo: t.maxPayloadBytes}
	}
	return nil
}

func validateSessionValue(label, value string, maximum int) error {
	if len(value) > maximum {
		return fmt.Errorf("%s demasiado largo", label)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%s contiene caracteres de control", label)
		}
	}
	return nil
}

func parseEndpoint(raw string) (*url.URL, error) {
	if raw != strings.TrimSpace(raw) || raw == "" {
		return nil, errors.New("URL vacia o con espacios exteriores")
	}
	if len(raw) > maxEndpointBytes {
		return nil, errors.New("URL demasiado larga")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if !parsed.IsAbs() || parsed.Host == "" || parsed.Opaque != "" {
		return nil, errors.New("se requiere una URL absoluta")
	}
	if parsed.User != nil {
		return nil, errors.New("no se admiten credenciales en la URL")
	}
	if parsed.Fragment != "" {
		return nil, errors.New("no se admiten fragmentos en la URL")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
	case "http":
		if !isLoopbackHost(parsed.Hostname()) {
			return nil, errors.New("HTTP sin TLS solo se admite en loopback")
		}
	default:
		return nil, errors.New("solo se admiten los esquemas HTTPS y HTTP local")
	}
	return parsed, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func secureRedirectPolicy(previous func(*http.Request, []*http.Request) error) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return errors.New("demasiadas redirecciones HTTP")
		}
		if _, err := parseEndpoint(req.URL.String()); err != nil {
			return fmt.Errorf("redireccion no segura: %w", err)
		}
		if len(via) > 0 && !sameOrigin(via[0].URL, req.URL) {
			return errors.New("redireccion HTTP a otro origen rechazada")
		}
		if previous != nil {
			return previous(req, via)
		}
		return nil
	}
}

func sameOrigin(left, right *url.URL) bool {
	if left == nil || right == nil {
		return false
	}
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Hostname(), right.Hostname()) &&
		effectivePort(left) == effectivePort(right)
}

func effectivePort(endpoint *url.URL) string {
	if port := endpoint.Port(); port != "" {
		return port
	}
	if strings.EqualFold(endpoint.Scheme, "https") {
		return "443"
	}
	if strings.EqualFold(endpoint.Scheme, "http") {
		return "80"
	}
	return ""
}

func readBounded(reader io.Reader, maximum int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, &limits.ErrPayloadExcedido{Tamaño: int64(len(data)), Maximo: maximum}
	}
	return data, nil
}

var _ ports.ResultTransport = (*Transporte)(nil)
