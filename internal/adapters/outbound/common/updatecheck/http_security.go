// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package updatecheck

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxBodyBytes int64 = 1 << 20
	maxRedirects       = 3
)

// validateEndpoint exige HTTPS para endpoints remotos. HTTP solo se admite
// para loopback, de modo que las pruebas y diagnósticos locales no relajen la
// política de producción.
func validateEndpoint(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("url de actualizaciones vacia")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("url de actualizaciones invalida: %w", err)
	}
	if u.Scheme == "" || u.Host == "" || u.Hostname() == "" {
		return nil, fmt.Errorf("url de actualizaciones invalida: %s", raw)
	}
	if u.User != nil {
		return nil, fmt.Errorf("url de actualizaciones con credenciales no permitida")
	}
	if u.Fragment != "" {
		return nil, fmt.Errorf("url de actualizaciones con fragmento no permitida")
	}

	switch strings.ToLower(u.Scheme) {
	case "https":
		return u, nil
	case "http":
		if isLoopbackHostname(u.Hostname()) {
			return u, nil
		}
		return nil, fmt.Errorf("url de actualizaciones remota debe usar HTTPS")
	default:
		return nil, fmt.Errorf("esquema de actualizaciones no permitido: %s", u.Scheme)
	}
}

func isLoopbackHostname(host string) bool {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func secureHTTPClient(base *http.Client, endpoint *url.URL, fallbackTimeout time.Duration) *http.Client {
	client := http.Client{}
	if base != nil {
		client = *base
	}
	if client.Timeout <= 0 {
		client.Timeout = fallbackTimeout
	}

	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects {
			return fmt.Errorf("demasiadas redirecciones de actualizaciones")
		}
		redirect, err := validateEndpoint(req.URL.String())
		if err != nil {
			return fmt.Errorf("redireccion de actualizaciones rechazada: %w", err)
		}
		if !sameOrigin(endpoint, redirect) {
			return fmt.Errorf("redireccion de actualizaciones a otro origen rechazada")
		}
		return nil
	}
	return &client
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(strings.TrimSuffix(a.Hostname(), "."), strings.TrimSuffix(b.Hostname(), ".")) &&
		effectivePort(a) == effectivePort(b)
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	if resp.ContentLength > maxBodyBytes {
		return nil, fmt.Errorf("respuesta de actualizaciones demasiado grande")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("leyendo respuesta: %w", err)
	}
	if int64(len(body)) > maxBodyBytes {
		return nil, fmt.Errorf("respuesta de actualizaciones demasiado grande")
	}
	return body, nil
}
