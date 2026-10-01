// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package proxysecretstore

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"grxfirma/internal/ports"
)

var ErrRuntimeProxyStoreRequired = errors.New("proxysecretstore: proxySecretId requiere backend seguro configurado")
var ErrRuntimeProxyBackendUnavailable = errors.New("proxysecretstore: backend seguro no disponible")

// RuntimeProxyConfig representa la configuracion efectiva de proxy lista para
// consumo runtime sin exponer credenciales en settings.
type RuntimeProxyConfig struct {
	Enabled      bool
	Type         string
	Host         string
	Port         int
	SecretID     string
	Realm        string
	Username     string
	Password     []byte
	ExcludedURLs []string
}

// ResolveRuntimeProxy proyecta la configuracion tipada de proxy a una
// configuracion efectiva lista para consumo runtime. Si se ha persistido un
// proxySecretId, resuelve el material sensible desde el almacen seguro.
func ResolveRuntimeProxy(ctx context.Context, proxy ports.ConfiguracionUsuarioProxy, store ports.ProxySecretStore) (RuntimeProxyConfig, error) {
	cfg := RuntimeProxyConfig{
		ExcludedURLs: append([]string(nil), proxy.ExcludedURLs...),
	}

	if proxy.Enabled == nil || !*proxy.Enabled {
		return cfg, nil
	}
	cfg.Enabled = true

	typ := strings.ToLower(strings.TrimSpace(ptrString(proxy.Type)))
	if typ == "" {
		if strings.TrimSpace(ptrString(proxy.Host)) != "" || ptrInt(proxy.Port) > 0 || strings.TrimSpace(ptrString(proxy.SecretID)) != "" {
			typ = "manual"
		}
	}
	cfg.Type = typ
	switch cfg.Type {
	case "none":
		cfg.Enabled = false
		return cfg, nil
	case "system":
		return cfg, nil
	case "manual":
		// continúa con la validación del proxy manual.
	default:
		return RuntimeProxyConfig{}, errors.New("proxysecretstore: tipo de proxy invalido")
	}

	cfg.Host = strings.TrimSpace(ptrString(proxy.Host))
	cfg.Port = ptrInt(proxy.Port)
	cfg.SecretID = strings.TrimSpace(ptrString(proxy.SecretID))
	cfg.Realm = strings.TrimSpace(ptrString(proxy.Realm))

	if cfg.Host == "" {
		return RuntimeProxyConfig{}, errors.New("proxysecretstore: proxy manual sin host")
	}
	if strings.HasPrefix(cfg.Host, "[") && strings.HasSuffix(cfg.Host, "]") {
		cfg.Host = strings.TrimSuffix(strings.TrimPrefix(cfg.Host, "["), "]")
	}
	if strings.ContainsAny(cfg.Host, `/\\@?#`) ||
		strings.ContainsAny(cfg.Host, "\r\n\t ") ||
		(strings.Contains(cfg.Host, ":") && net.ParseIP(cfg.Host) == nil) {
		return RuntimeProxyConfig{}, errors.New("proxysecretstore: host de proxy invalido")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return RuntimeProxyConfig{}, errors.New("proxysecretstore: proxy manual sin puerto valido")
	}
	if cfg.SecretID == "" {
		return cfg, nil
	}
	normalizedID, err := normalizeSecretID(cfg.SecretID)
	if err != nil {
		return RuntimeProxyConfig{}, err
	}
	cfg.SecretID = normalizedID
	if store == nil {
		return RuntimeProxyConfig{}, ErrRuntimeProxyStoreRequired
	}
	status, err := store.Status(ctx)
	if err != nil {
		return RuntimeProxyConfig{}, fmt.Errorf("proxysecretstore: consultando estado del backend seguro: %w", err)
	}
	if !status.Available {
		reason := strings.TrimSpace(status.Reason)
		if reason == "" {
			reason = "sin motivo detallado"
		}
		return RuntimeProxyConfig{}, fmt.Errorf(
			"proxysecretstore: backend %s no disponible en %s: %s: %w",
			strings.TrimSpace(status.Backend),
			strings.TrimSpace(status.Platform),
			reason,
			ErrRuntimeProxyBackendUnavailable,
		)
	}

	material, err := store.Load(ctx, cfg.SecretID)
	if err != nil {
		return RuntimeProxyConfig{}, fmt.Errorf("proxysecretstore: resolviendo secreto: %w", err)
	}
	defer zeroSecretBytes(material.Password)
	materialRealm := strings.TrimSpace(material.Realm)
	if cfg.Realm != "" && materialRealm != cfg.Realm {
		return RuntimeProxyConfig{}, errors.New("proxysecretstore: el realm del secreto no coincide con el proxy configurado")
	}
	cfg.Username = material.Username
	cfg.Password = append([]byte(nil), material.Password...)
	return cfg, nil
}

func ptrString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func ptrInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func IsRuntimeProxyBackendUnavailable(err error) bool {
	return errors.Is(err, ErrRuntimeProxyBackendUnavailable) ||
		errors.Is(err, ErrRuntimeProxyStoreRequired) ||
		isPlatformStoreUnavailable(err)
}
