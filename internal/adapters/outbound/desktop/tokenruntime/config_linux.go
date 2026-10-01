//go:build linux && pkcs11_preview && !production

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package tokenruntime

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
)

const maxConfigBytes = 64 * 1024
const maxModules = 8

type config struct {
	Version int   `json:"version"`
	Enabled *bool `json:"enabled"`
	Modules []struct {
		Path string `json:"path"`
	} `json:"modules"`
}

func readConfig(configDir string) ([]string, error) {
	if !filepath.IsAbs(configDir) || len(configDir) > 4096 || strings.ContainsRune(configDir, 0) {
		return nil, ErrInvalidConfig
	}
	path := filepath.Join(configDir, "tokens.json")
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil, nil
	}
	// Reuse owner/ancestor/symlink checks; tighten the leaf bound for JSON.
	resolved, err := pkcs11worker.ValidateModulePath(path)
	if err != nil {
		return nil, ErrInvalidConfig
	}
	f, err := os.Open(resolved)
	if err != nil {
		return nil, ErrInvalidConfig
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxConfigBytes {
		return nil, ErrInvalidConfig
	}
	data, err := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	if err != nil || len(data) > maxConfigBytes {
		return nil, ErrInvalidConfig
	}
	cfg, err := decodeConfig(data)
	if err != nil {
		return nil, err
	}
	if !*cfg.Enabled {
		return nil, nil
	}
	modules := make([]string, 0, len(cfg.Modules))
	seen := make(map[string]bool)
	for _, module := range cfg.Modules {
		resolved, err := pkcs11worker.ValidateModulePath(module.Path)
		if err != nil || seen[resolved] {
			return nil, ErrInvalidConfig
		}
		seen[resolved] = true
		modules = append(modules, resolved)
	}
	return modules, nil
}

func decodeConfig(data []byte) (config, error) {
	var cfg config
	if len(data) == 0 || len(data) > maxConfigBytes || !uniqueJSONKeys(data) {
		return cfg, ErrInvalidConfig
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cfg) != nil {
		return cfg, ErrInvalidConfig
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || cfg.Version != 1 || cfg.Enabled == nil || len(cfg.Modules) > maxModules {
		return cfg, ErrInvalidConfig
	}
	if *cfg.Enabled && len(cfg.Modules) == 0 {
		return cfg, ErrInvalidConfig
	}
	seen := make(map[string]bool)
	for _, module := range cfg.Modules {
		if !filepath.IsAbs(module.Path) || len(module.Path) > 4096 || strings.ContainsRune(module.Path, 0) || seen[module.Path] {
			return cfg, ErrInvalidConfig
		}
		seen[module.Path] = true
	}
	return cfg, nil
}

// encoding/json otherwise accepts duplicate keys and case-insensitive fields.
// Restrict object keys to the exact versioned schema before decoding it.
func uniqueJSONKeys(data []byte) bool {
	d := json.NewDecoder(bytes.NewReader(data))
	var value func(int) bool
	value = func(depth int) bool {
		if depth > 4 {
			return false
		}
		token, err := d.Token()
		if err != nil {
			return false
		}
		delim, container := token.(json.Delim)
		if !container {
			return true
		}
		switch delim {
		case '{':
			seen := make(map[string]bool)
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return false
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return false
				}
				seen[name] = true
				if (depth == 0 && name != "version" && name != "enabled" && name != "modules") || (depth == 2 && name != "path") || (depth != 0 && depth != 2) {
					return false
				}
				if !value(depth + 1) {
					return false
				}
			}
		case '[':
			for d.More() {
				if !value(depth + 1) {
					return false
				}
			}
		default:
			return false
		}
		_, err = d.Token()
		return err == nil
	}
	return value(0)
}
