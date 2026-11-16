// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certselectionstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/appdirs"
)

type Store struct {
	persistentPath string
	sessionPath    string
	sessionErr     error
	mu             sync.Mutex
}

const (
	maxPreferenceFileBytes = 1024 * 1024
	sessionFileName        = "preferred-certificates-session.json"
)

func New(configDir string) *Store {
	sessionPath, sessionErr := SessionPath()
	return &Store{
		persistentPath: filepath.Join(configDir, "preferred-certificates.json"),
		sessionPath:    sessionPath,
		sessionErr:     sessionErr,
	}
}

// SessionPath devuelve dónde se guardan las preferencias que solo valen
// durante la sesión: <carpeta privada de ejecución>/grxfirma/. Nunca está en la
// carpeta temporal compartida; ver appdirs.RuntimeUsuario.
func SessionPath() (string, error) {
	dir, err := appdirs.RuntimeUsuario()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "grxfirma", sessionFileName), nil
}

// LoadSession no usa preferencias de una carpeta que no sea privada del
// usuario: las trata como inexistentes y el usuario vuelve a elegir.
func (s *Store) LoadSession(_ context.Context, origin string) (string, bool, error) {
	if s.sessionPath == "" {
		return "", false, nil
	}
	if err := securefile.CheckPrivateDir(filepath.Dir(s.sessionPath)); err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, securefile.ErrNotPrivateDir) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("comprobar carpeta de preferencias de sesión: %w", err)
	}
	return s.load(s.sessionPath, origin)
}

func (s *Store) SaveSession(_ context.Context, origin, certificateID string) error {
	if s.sessionPath == "" {
		return fmt.Errorf("guardar preferencias de sesión: %w", s.sessionErr)
	}
	if strings.TrimSpace(origin) == "" || strings.TrimSpace(certificateID) == "" {
		return nil
	}
	if err := securefile.EnsurePrivateDir(filepath.Dir(s.sessionPath)); err != nil {
		return fmt.Errorf("crear carpeta privada de preferencias de sesión: %w", err)
	}
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
