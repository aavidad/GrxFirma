// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certselectionstore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"grxfirma/internal/adapters/outbound/common/securefile"
)

type Store struct {
	persistentPath string
	sessionPath    string
	mu             sync.Mutex
}

const maxPreferenceFileBytes = 1024 * 1024

func New(configDir string) *Store {
	return &Store{
		persistentPath: filepath.Join(configDir, "preferred-certificates.json"),
		sessionPath:    filepath.Join(runtimeDir(), "preferred-certificates-session.json"),
	}
}

func (s *Store) LoadSession(_ context.Context, origin string) (string, bool, error) {
	return s.load(s.sessionPath, origin)
}

func (s *Store) SaveSession(_ context.Context, origin, certificateID string) error {
	return s.save(s.sessionPath, origin, certificateID)
}

func (s *Store) LoadPersistent(_ context.Context, origin string) (string, bool, error) {
	return s.load(s.persistentPath, origin)
}

func (s *Store) SavePersistent(_ context.Context, origin, certificateID string) error {
	return s.save(s.persistentPath, origin, certificateID)
}

func (s *Store) load(path, origin string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := securefile.ReadFileLimit(path, maxPreferenceFileBytes)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("leer preferencias de certificado: %w", err)
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", false, fmt.Errorf("parsear preferencias de certificado: %w", err)
	}
	origin = strings.TrimSpace(origin)
	id, ok := raw[origin]
	if !ok || strings.TrimSpace(id) == "" {
		return "", false, nil
	}
	return strings.TrimSpace(id), true, nil
}

func (s *Store) save(path, origin, certificateID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	origin = strings.TrimSpace(origin)
	certificateID = strings.TrimSpace(certificateID)
	if origin == "" || certificateID == "" {
		return nil
	}

	raw := map[string]string{}
	if data, readErr := securefile.ReadFileLimit(path, maxPreferenceFileBytes); readErr == nil {
		_ = json.Unmarshal(data, &raw)
	} else if !os.IsNotExist(readErr) {
		return fmt.Errorf("leer preferencias de certificado existentes: %w", readErr)
	}
	raw[origin] = certificateID

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("crear directorio de preferencias: %w", err)
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("serializar preferencias de certificado: %w", err)
	}
	if err := securefile.WriteFileAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("guardar preferencias de certificado: %w", err)
	}
	return nil
}

func runtimeDir() string {
	if dir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); dir != "" {
		return filepath.Join(dir, "grxfirma")
	}
	user := strings.TrimSpace(os.Getenv("USER"))
	if user == "" {
		user = "default"
	}
	return filepath.Join(os.TempDir(), "grxfirma-"+user)
}
