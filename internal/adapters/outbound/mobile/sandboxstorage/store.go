// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package sandboxstorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"grxfirma/internal/ports"
)

// Store implementa TempFileStore respetando el sandbox móvil.
type Store struct {
	mu        sync.Mutex
	baseDir   string
	activos   map[string]struct{}
	generador func(dir, pattern string) (*os.File, error)
}

// Nuevo crea un Store sobre un directorio base ya resuelto por la capa nativa.
// El directorio debe pertenecer al sandbox de la aplicacion en Android/iOS.
func Nuevo(baseDir string) (*Store, error) {
	baseDir = filepath.Clean(baseDir)
	if baseDir == "." || strings.TrimSpace(baseDir) == "" {
		return nil, errors.New("sandbox-storage: directorio base no configurado")
	}
	if rutaCompartida(baseDir) {
		return nil, fmt.Errorf("sandbox-storage: la ruta '%s' apunta a almacenamiento compartido no permitido", baseDir)
	}
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, fmt.Errorf("sandbox-storage: no se pudo preparar el directorio base: %w", err)
	}
	return &Store{
		baseDir:   baseDir,
		activos:   make(map[string]struct{}),
		generador: os.CreateTemp,
	}, nil
}

// Write escribe un temporal en el sandbox de la aplicación.
func (s *Store) Write(ctx context.Context, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s == nil || s.baseDir == "" {
		return "", errors.New("sandbox-storage: directorio base no configurado")
	}
	if len(data) == 0 {
		return "", errors.New("sandbox-storage: no se pueden escribir datos vacios")
	}

	f, err := s.generador(s.baseDir, "grxfirma-mobile-*")
	if err != nil {
		return "", fmt.Errorf("sandbox-storage: no se pudo crear el temporal: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("sandbox-storage: no se pudo escribir el temporal: %w", err)
	}

	ruta := f.Name()
	s.mu.Lock()
	s.activos[ruta] = struct{}{}
	s.mu.Unlock()
	return ruta, nil
}

// Delete elimina un temporal del sandbox.
func (s *Store) Delete(ctx context.Context, ruta string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.baseDir == "" {
		return errors.New("sandbox-storage: directorio base no configurado")
	}
	if !s.perteneceAlSandbox(ruta) {
		return fmt.Errorf("sandbox-storage: la ruta '%s' no pertenece al sandbox gestionado", ruta)
	}
	if err := os.Remove(ruta); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("sandbox-storage: no se pudo eliminar el temporal: %w", err)
	}
	s.mu.Lock()
	delete(s.activos, ruta)
	s.mu.Unlock()
	return nil
}

var _ ports.TempFileStore = (*Store)(nil)

func (s *Store) perteneceAlSandbox(ruta string) bool {
	absRuta, err := filepath.Abs(ruta)
	if err != nil {
		return false
	}
	absBase, err := filepath.Abs(s.baseDir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absBase, absRuta)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, "..")
}

func rutaCompartida(baseDir string) bool {
	ruta := filepath.ToSlash(strings.ToLower(filepath.Clean(baseDir)))
	for _, prefijo := range []string{
		"/sdcard",
		"/mnt/sdcard",
		"/storage/emulated",
		"/storage/self/primary",
	} {
		if ruta == prefijo || strings.HasPrefix(ruta, prefijo+"/") {
			return true
		}
	}
	return false
}
