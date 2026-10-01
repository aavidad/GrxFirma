//go:build linux && pkcs11_preview && !production

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package tokenruntime

import (
	"os"
	"path/filepath"

	"grxfirma/internal/adapters/outbound/desktop/isolatedtokenstore"
	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
)

func EnabledInBuild() bool { return true }

func newRuntime(configDir string, prompt Prompt) *Runtime {
	modules, err := readConfig(configDir)
	if err != nil {
		return &Runtime{inactive: err, diagnostic: err}
	}
	r := &Runtime{modules: modules, prompt: prompt, routes: make(map[string]route)}
	if len(modules) == 0 {
		return r
	}
	executable, err := os.Executable()
	if err != nil {
		return &Runtime{inactive: ErrHelperUnavailable, diagnostic: ErrHelperUnavailable}
	}
	helper, err := resolveHelper(executable)
	if err != nil {
		return &Runtime{inactive: err, diagnostic: err}
	}
	r.factory = func(module string, pin pkcs11worker.PINSource) source {
		return isolatedtokenstore.New(pkcs11worker.Client{Executable: helper, ModulePath: module, PINSource: pin})
	}
	return r
}

// resolveHelper accepts only installation-relative locations. Configuration
// cannot choose an executable. Native hosts already installed in lib/bin use
// the same sibling rule as a staged CLI bundle.
func resolveHelper(executable string) (string, error) {
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil || !filepath.IsAbs(resolved) {
		return "", ErrHelperUnavailable
	}
	for _, candidate := range helperCandidates(resolved) {
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			continue
		}
		path, err := pkcs11worker.ValidateModulePath(candidate)
		if err != nil {
			return "", ErrHelperUnavailable
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm()&0111 == 0 {
			return "", ErrHelperUnavailable
		}
		return path, nil
	}
	return "", ErrHelperUnavailable
}

func helperCandidates(executable string) []string {
	dir := filepath.Dir(executable)
	candidates := []string{filepath.Join(dir, "grxfirma-pkcs11-worker")}
	if dir == "/usr/bin" || (filepath.Base(dir) == "bin" && filepath.Base(filepath.Dir(dir)) == ".local") {
		candidates = append(candidates, filepath.Join(dir, "..", "lib", "grxfirma", "bin", "grxfirma-pkcs11-worker"))
	}
	return candidates
}
