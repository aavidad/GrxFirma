// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"grxfirma/internal/ports"
)

const (
	defaultTimeout = 3 * time.Second
)

type Config struct {
	Enabled        bool
	CurrentVersion string
	SourceURL      string
	Timeout        time.Duration
	Logger         *slog.Logger
	Notify         ports.DesktopNotification
}

type Result struct {
	Version string
	URL     string
}

type releasePayload struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Version string `json:"version"`
	URL     string `json:"url"`
}

type comparableVersion struct {
	Numbers    []int
	Prerelease bool
}

func Start(cfg Config) {
	if !cfg.Enabled {
		return
	}
	if strings.TrimSpace(cfg.SourceURL) == "" {
		return
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		result, ok, err := Check(ctx, cfg.CurrentVersion, cfg.SourceURL)
		if err != nil {
			logDebug(
				cfg.Logger,
				ctx,
				"comprobacion de actualizacion omitida",
				"error_code",
				FailureCode(err),
			)
			return
		}
		if !ok {
			return
		}

		logInfo(
			cfg.Logger,
			ctx,
			"actualizacion disponible",
			"version_actual",
			strings.TrimSpace(cfg.CurrentVersion),
			"version_remota",
			result.Version,
			"url",
			result.URL,
		)

		if cfg.Notify == nil {
			return
		}

		notifyCtx, cancelNotify := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelNotify()
		_ = cfg.Notify.Notify(
			notifyCtx,
			"Actualizacion disponible",
			fmt.Sprintf("Hay una version nueva de GrxFirma: %s", result.Version),
		)
	}()
}

func Check(ctx context.Context, currentVersion, source string) (Result, bool, error) {
	latestURL, err := resolveLatestURL(source)
	if err != nil {
		return Result{}, false, err
	}
	endpoint, err := validateEndpoint(latestURL)
	if err != nil {
		return Result{}, false, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestURL, nil)
	if err != nil {
		return Result{}, false, fmt.Errorf("creando peticion: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "GrxFirma-UpdateCheck")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := secureHTTPClient(nil, endpoint, defaultTimeout).Do(req)
	if err != nil {
		return Result{}, false, fmt.Errorf("consultando actualizaciones: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{}, false, fmt.Errorf("respuesta inesperada %d", resp.StatusCode)
	}

	var payload releasePayload
	body, err := readResponseBody(resp)
	if err != nil {
		return Result{}, false, err
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Result{}, false, fmt.Errorf("parseando respuesta JSON: %w", err)
	}

	result := Result{
		Version: strings.TrimSpace(firstNonEmpty(payload.TagName, payload.Version)),
		URL:     strings.TrimSpace(firstNonEmpty(payload.HTMLURL, payload.URL)),
	}
	if result.Version == "" {
		return Result{}, false, fmt.Errorf("respuesta sin version")
	}
	if result.URL == "" && !isOfficialLatestEndpoint(endpoint) {
		result.URL = strings.TrimSpace(source)
	}
	result.URL, err = releaseDestination(endpoint, result.URL)
	if err != nil {
		return Result{}, false, err
	}

	return result, isRemoteNewer(strings.TrimSpace(currentVersion), result.Version), nil
}

func resolveLatestURL(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", fmt.Errorf("url de actualizaciones vacia")
	}

	if owner, repo, ok := parseGitHubRepo(source); ok {
		return fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo), nil
	}

	u, err := url.Parse(source)
	if err != nil {
		return "", fmt.Errorf("url de actualizaciones invalida: %w", err)
	}
	if _, err := validateEndpoint(u.String()); err != nil {
		return "", err
	}
	return source, nil
}

func parseGitHubRepo(raw string) (owner, repo string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	if strings.HasPrefix(raw, "git@github.com:") {
		raw = "https://github.com/" + strings.TrimPrefix(raw, "git@github.com:")
	}
	if strings.HasPrefix(raw, "github.com/") {
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", "", false
	}
	host := strings.ToLower(strings.TrimSpace(u.Host))
	if host != "github.com" && host != "www.github.com" && host != "api.github.com" {
		return "", "", false
	}
	path := strings.Trim(strings.TrimSpace(u.Path), "/")
	parts := strings.Split(path, "/")
	if host == "api.github.com" && len(parts) >= 4 && parts[0] == "repos" {
		return strings.TrimSpace(parts[1]), strings.TrimSuffix(strings.TrimSpace(parts[2]), ".git"), true
	}
	if len(parts) < 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSuffix(strings.TrimSpace(parts[1]), ".git"), true
}

func isRemoteNewer(currentVersion, remoteVersion string) bool {
	if strings.EqualFold(strings.TrimSpace(currentVersion), strings.TrimSpace(remoteVersion)) {
		return false
	}

	current, currentOK := parseComparableVersion(currentVersion)
	remote, remoteOK := parseComparableVersion(remoteVersion)
	switch {
	case !remoteOK:
		return false
	case !currentOK:
		// Una build de desarrollo o una versión dañada no es comparable. Avisar
		// como si fuera antigua produciría falsos positivos en QA.
		return false
	}

	maxLen := len(current.Numbers)
	if len(remote.Numbers) > maxLen {
		maxLen = len(remote.Numbers)
	}
	for i := 0; i < maxLen; i++ {
		cv := 0
		rv := 0
		if i < len(current.Numbers) {
			cv = current.Numbers[i]
		}
		if i < len(remote.Numbers) {
			rv = remote.Numbers[i]
		}
		if rv > cv {
			return true
		}
		if rv < cv {
			return false
		}
	}

	return current.Prerelease && !remote.Prerelease
}

func parseComparableVersion(raw string) (comparableVersion, bool) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "v")
	raw = strings.TrimPrefix(raw, "V")
	if raw == "" {
		return comparableVersion{}, false
	}

	end := 0
	for end < len(raw) {
		ch := raw[end]
		if (ch >= '0' && ch <= '9') || ch == '.' {
			end++
			continue
		}
		break
	}
	prefix := strings.Trim(raw[:end], ".")
	if prefix == "" {
		return comparableVersion{}, false
	}

	parts := strings.Split(prefix, ".")
	numbers := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return comparableVersion{}, false
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return comparableVersion{}, false
		}
		numbers = append(numbers, n)
	}

	return comparableVersion{
		Numbers:    numbers,
		Prerelease: end < len(raw),
	}, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func logInfo(logger *slog.Logger, ctx context.Context, msg string, args ...any) {
	if logger == nil {
		return
	}
	logger.InfoContext(ctx, msg, args...)
}

func logDebug(logger *slog.Logger, ctx context.Context, msg string, args ...any) {
	if logger == nil {
		return
	}
	logger.DebugContext(ctx, msg, args...)
}
