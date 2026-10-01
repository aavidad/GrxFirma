// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package localtlstrust

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"grxfirma/internal/adapters/outbound/common/securefile"
)

func linuxNSSPaths(home string) ([]string, error) {
	chrome := filepath.Join(home, ".pki", "nssdb")
	if err := mkdirNoSymlink(filepath.Join(home, ".pki")); err != nil {
		return nil, err
	}
	if err := mkdirNoSymlink(chrome); err != nil {
		return nil, err
	}
	profiles, err := discoverFirefoxProfiles(home)
	return append([]string{chrome}, profiles...), err
}

func discoverFirefoxProfiles(home string) ([]string, error) {
	roots := []string{
		filepath.Join(home, ".mozilla", "firefox"),
		filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox"),
		filepath.Join(home, ".var", "app", "org.mozilla.firefox", ".mozilla", "firefox"),
	}
	seen := make(map[string]bool)
	var paths []string
	var failures []error
	for _, root := range roots {
		ini := filepath.Join(root, "profiles.ini")
		if _, err := os.Lstat(ini); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if !allComponentsReal(ini, false) {
			failures = append(failures, fmt.Errorf("localtlstrust: profiles.ini de Firefox inaccesible o enlace simbólico: %s", ini))
			continue
		}
		raw, err := securefile.ReadFileLimit(ini, 1024*1024)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("localtlstrust: leer perfiles Firefox %s: %w", ini, err))
			continue
		}
		for _, p := range parseFirefoxProfilesINI(raw) {
			var path string
			if p.absolute {
				path = filepath.Clean(p.path)
			} else {
				path = filepath.Clean(filepath.Join(root, p.path))
			}
			if !filepath.IsAbs(path) || !withinPath(home, path) || (!p.absolute && !withinPath(root, path)) {
				failures = append(failures, fmt.Errorf("localtlstrust: ruta de perfil Firefox fuera de la carpeta permitida: %s", p.path))
				continue
			}
			if seen[path] {
				continue
			}
			if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
				continue
			}
			if !allComponentsReal(path, true) {
				failures = append(failures, fmt.Errorf("localtlstrust: perfil Firefox inaccesible o enlace simbólico: %s", path))
				continue
			}
			if _, err := os.Lstat(filepath.Join(path, "cert9.db")); err == nil && !regularNoSymlink(filepath.Join(path, "cert9.db")) {
				failures = append(failures, fmt.Errorf("localtlstrust: cert9.db de Firefox inseguro: %s", path))
				continue
			}
			if err := validateNSSStore(path); err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", path, err))
				continue
			}
			seen[path] = true
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths, errors.Join(failures...)
}

type firefoxINIProfile struct {
	path     string
	absolute bool
}

func parseFirefoxProfilesINI(raw []byte) []firefoxINIProfile {
	var profiles []firefoxINIProfile
	var current firefoxINIProfile
	inProfile := false
	flush := func() {
		if inProfile && current.path != "" {
			profiles = append(profiles, current)
		}
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			flush()
			name := strings.TrimSpace(line[1 : len(line)-1])
			inProfile = strings.HasPrefix(name, "Profile")
			current = firefoxINIProfile{}
			continue
		}
		if !inProfile {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "Path":
			current.path = strings.TrimSpace(value)
		case "IsRelative":
			current.absolute = strings.TrimSpace(value) == "0"
		}
	}
	flush()
	return profiles
}

func validateNSSStore(path string) error {
	if !filepath.IsAbs(path) || path != filepath.Clean(path) {
		return errors.New("ruta NSS no absoluta o no normalizada")
	}
	if !allComponentsReal(path, true) {
		return errors.New("ruta NSS insegura o enlace simbólico")
	}
	for _, name := range []string{"cert9.db", "key4.db", "pkcs11.txt", "cert9.db-wal", "cert9.db-shm", "cert9.db-journal", "key4.db-wal", "key4.db-shm", "key4.db-journal"} {
		p := filepath.Join(path, name)
		info, err := os.Lstat(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("fichero NSS inseguro: %s", p)
		}
	}
	return nil
}

func allComponentsReal(path string, leafDir bool) bool {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return false
	}
	current := string(filepath.Separator)
	parts := strings.Split(strings.TrimPrefix(clean, current), current)
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return false
		}
		if index < len(parts)-1 || leafDir {
			if !info.IsDir() {
				return false
			}
		} else if !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}

func regularNoSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func mkdirNoSymlink(path string) error {
	if !allComponentsReal(filepath.Dir(path), true) {
		return fmt.Errorf("localtlstrust: carpeta padre NSS insegura: %s", path)
	}
	err := os.Mkdir(path, 0o700)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if !allComponentsReal(path, true) {
		return fmt.Errorf("localtlstrust: carpeta NSS insegura: %s", path)
	}
	return nil
}

func withinPath(root, child string) bool {
	rel, err := filepath.Rel(root, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func allowedLinuxStorePath(home, path string) bool {
	if !filepath.IsAbs(path) || path != filepath.Clean(path) || !withinPath(home, path) {
		return false
	}
	if path == filepath.Join(home, ".pki", "nssdb") {
		return true
	}
	for _, root := range []string{filepath.Join(home, ".mozilla", "firefox"), filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox"), filepath.Join(home, ".var", "app", "org.mozilla.firefox", ".mozilla", "firefox")} {
		if withinPath(root, path) {
			return true
		}
	}
	// Firefox también admite IsRelative=0 bajo HOME; se registró la ruta
	// concreta al instalar y se verifica la huella antes de cualquier baja.
	return true
}
