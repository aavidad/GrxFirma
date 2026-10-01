// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package updatecheck comprueba si hay una versión más reciente publicada en
// GitHub Releases. Nunca descarga ni instala nada: solo devuelve información
// para que cada interfaz pueda avisar al usuario y abrir el destino oficial.
package updatecheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// EnvHabilitar habilita la comprobación de versión cuando vale "1".
// Por defecto (sin la variable) la aplicación no contacta con GitHub.
const EnvHabilitar = "GRXFIRMA_CHECK_UPDATES"

const (
	OfficialRepositoryURL = "https://github.com/aavidad/GrxFirma"
	OfficialReleasesURL   = OfficialRepositoryURL + "/releases"
	OfficialLatestAPIURL  = "https://api.github.com/repos/aavidad/GrxFirma/releases/latest"
	defaultClientTimeout  = 5 * time.Second
)

// Habilitado indica si el operador activó la comprobación de actualizaciones.
func Habilitado() bool {
	return os.Getenv(EnvHabilitar) == "1"
}

// Resultado describe la comparación entre la versión en ejecución y la última
// publicada.
type Resultado struct {
	VersionActual string `json:"version_actual"`
	UltimaVersion string `json:"ultima_version"`
	HayNueva      bool   `json:"hay_nueva"`
	Comparable    bool   `json:"comparable"`
	URL           string `json:"url"`
	Estado        string `json:"estado"`
	Mensaje       string `json:"mensaje,omitempty"`
	Titulo        string `json:"titulo,omitempty"`
}

const (
	EstadoPublicada        = "publicada"
	EstadoSinPublicaciones = "sin_publicaciones"
)

type HTTPStatusError struct{ StatusCode int }

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("consultando releases: HTTP %d", e.StatusCode)
}

// Client consulta la última release publicada.
type Client struct {
	URL  string
	HTTP *http.Client
}

// FailureCode reduce un fallo de transporte a un código apto para interfaz y
// diagnóstico. Nunca devuelve la URL del proxy, credenciales ni texto remoto.
func FailureCode(err error) string {
	if err == nil {
		return "NETWORK_OR_PUBLICATION_UNAVAILABLE"
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "TIMEOUT"
	case errors.Is(err, context.Canceled):
		return "CANCELLED"
	}
	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) {
		return fmt.Sprintf("HTTP %d", statusErr.StatusCode)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "TIMEOUT"
	}
	message := err.Error()
	if index := strings.Index(message, "HTTP "); index >= 0 {
		status := message[index+len("HTTP "):]
		if len(status) >= 3 {
			status = status[:3]
			if status[0] >= '4' && status[0] <= '5' &&
				status[1] >= '0' && status[1] <= '9' &&
				status[2] >= '0' && status[2] <= '9' {
				return "HTTP " + status
			}
		}
	}
	if strings.Contains(strings.ToLower(message), "proxy") {
		return "PROXY_UNAVAILABLE"
	}
	return "NETWORK_OR_PUBLICATION_UNAVAILABLE"
}

// MessageKey selecciona un texto de catálogo sin exponer detalles de red.
func MessageKey(err error) string {
	code := FailureCode(err)
	if strings.HasPrefix(code, "HTTP 5") {
		return "El servicio de versiones de GitHub no está disponible ahora. Vuelva a intentarlo más tarde."
	}
	switch code {
	case "TIMEOUT":
		return "La consulta de versiones ha tardado demasiado. Vuelva a intentarlo más tarde."
	case "HTTP 403":
		return "GitHub ha limitado temporalmente la consulta de versiones. Vuelva a intentarlo más tarde."
	case "PROXY_UNAVAILABLE":
		return "No se pudo conectar mediante el proxy. Revise su configuración y vuelva a intentarlo."
	case "NETWORK_OR_PUBLICATION_UNAVAILABLE":
		return "No se pudo conectar para comprobar versiones. Revise su conexión a Internet y vuelva a intentarlo."
	default:
		return "No se pudo comprobar la versión. Vuelva a intentarlo más tarde."
	}
}

// ErrorCode es estable y cumple el formato del contrato IPC.
func ErrorCode(err error) string {
	code := FailureCode(err)
	if strings.HasPrefix(code, "HTTP 5") {
		return "update_service_unavailable"
	}
	switch code {
	case "TIMEOUT":
		return "update_timeout"
	case "HTTP 403":
		return "update_rate_limited"
	case "PROXY_UNAVAILABLE":
		return "update_proxy_unavailable"
	case "NETWORK_OR_PUBLICATION_UNAVAILABLE":
		return "update_network_unavailable"
	case "CANCELLED":
		return "update_cancelled"
	default:
		return "update_check_failed"
	}
}

// New crea un cliente con la URL oficial del repositorio y timeout corto.
func New() *Client {
	return NewWithHTTPClient(nil)
}

// NewWithHTTPClient conserva el transporte seguro compartido por el producto
// (incluido el proxy configurado), pero fija un timeout corto propio para que
// el aviso informativo nunca retrase las operaciones locales.
func NewWithHTTPClient(base *http.Client) *Client {
	httpClient := &http.Client{Timeout: defaultClientTimeout}
	if base != nil {
		*httpClient = *base
		httpClient.Timeout = defaultClientTimeout
	}
	return &Client{
		URL:  OfficialLatestAPIURL,
		HTTP: httpClient,
	}
}

// Comprobar consulta la última release y la compara con versionActual.
// Si versionActual no es un semver reconocible (build de desarrollo),
// HayNueva queda en false y solo se informa de la última publicada.
func (c *Client) Comprobar(ctx context.Context, versionActual string) (Resultado, error) {
	endpoint, err := validateEndpoint(c.URL)
	if err != nil {
		return Resultado{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return Resultado{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "GrxFirma-UpdateCheck")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := secureHTTPClient(c.HTTP, endpoint, defaultClientTimeout).Do(req)
	if err != nil {
		return Resultado{}, fmt.Errorf("consultando releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound && isOfficialLatestEndpoint(endpoint) &&
		resp.Request != nil && isOfficialLatestEndpoint(resp.Request.URL) {
		return Resultado{VersionActual: versionActual, Estado: EstadoSinPublicaciones}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return Resultado{}, &HTTPStatusError{StatusCode: resp.StatusCode}
	}

	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	body, err := readResponseBody(resp)
	if err != nil {
		return Resultado{}, err
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return Resultado{}, fmt.Errorf("respuesta de releases no reconocida: %w", err)
	}
	if strings.TrimSpace(release.TagName) == "" {
		return Resultado{}, fmt.Errorf("la respuesta de releases no incluye tag_name")
	}
	releaseURL, err := releaseDestination(endpoint, release.HTMLURL)
	if err != nil {
		return Resultado{}, err
	}
	_, comparableActual := parseSemver(versionActual)
	_, comparableUltima := parseSemver(release.TagName)

	return Resultado{
		VersionActual: versionActual,
		UltimaVersion: release.TagName,
		HayNueva:      esMasNueva(versionActual, release.TagName),
		Comparable:    comparableActual && comparableUltima,
		URL:           releaseURL,
		Estado:        EstadoPublicada,
	}, nil
}

func releaseDestination(endpoint *url.URL, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if isOfficialLatestEndpoint(endpoint) {
			return OfficialReleasesURL, nil
		}
		return "", fmt.Errorf("la respuesta de releases no incluye html_url")
	}
	destination, err := validateEndpoint(raw)
	if err != nil {
		return "", fmt.Errorf("destino de release no permitido: %w", err)
	}
	if !isOfficialLatestEndpoint(endpoint) {
		return destination.String(), nil
	}
	escapedPath := destination.EscapedPath()
	tagPrefix := "/aavidad/GrxFirma/releases/tag/"
	if !strings.EqualFold(destination.Scheme, "https") ||
		!strings.EqualFold(strings.TrimSuffix(destination.Hostname(), "."), "github.com") ||
		destination.Port() != "" ||
		destination.RawQuery != "" ||
		destination.Fragment != "" ||
		!strings.HasPrefix(escapedPath, tagPrefix) ||
		len(escapedPath) == len(tagPrefix) {
		return "", fmt.Errorf("destino de release fuera del repositorio oficial")
	}
	return destination.String(), nil
}

func isOfficialLatestEndpoint(endpoint *url.URL) bool {
	if endpoint == nil {
		return false
	}
	return strings.EqualFold(endpoint.Scheme, "https") &&
		strings.EqualFold(strings.TrimSuffix(endpoint.Hostname(), "."), "api.github.com") &&
		endpoint.Port() == "" &&
		endpoint.EscapedPath() == "/repos/aavidad/GrxFirma/releases/latest" &&
		endpoint.RawQuery == "" &&
		endpoint.Fragment == ""
}

// esMasNueva compara dos versiones semver de forma laxa (prefijo v opcional y
// componente patch opcional). Si la actual no es parseable (p. ej. "dev" o un
// hash de git), retorna false: no se puede afirmar que haya una más nueva.
func esMasNueva(actual, ultima string) bool {
	va, okA := parseSemver(actual)
	vu, okU := parseSemver(ultima)
	if !okA || !okU {
		return false
	}
	for i := 0; i < 3; i++ {
		if vu.numbers[i] != va.numbers[i] {
			return vu.numbers[i] > va.numbers[i]
		}
	}
	// A igual versión numérica, la release estable sucede a cualquier
	// pre-release. No se intenta ordenar dos identificadores pre-release: el
	// endpoint /latest de GitHub no publica pre-releases.
	return va.prerelease && !vu.prerelease
}

type semver struct {
	numbers    [3]int
	prerelease bool
}

func parseSemver(s string) (semver, bool) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V"))
	prerelease := false
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		if i == len(s)-1 {
			return semver{}, false
		}
		prerelease = true
		s = s[:i]
	}
	partes := strings.Split(s, ".")
	if len(partes) < 2 || len(partes) > 3 {
		return semver{}, false
	}
	var v [3]int
	for i, p := range partes {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, false
		}
		v[i] = n
	}
	return semver{numbers: v, prerelease: prerelease}, true
}
