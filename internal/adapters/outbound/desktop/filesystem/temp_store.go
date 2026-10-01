// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package filesystem implementa el puerto TempFileStore y las operaciones
// de escritura de resultados de firma en disco para plataformas de escritorio.
package filesystem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// AlmacenTemporal implementa ports.TempFileStore usando el sistema de ficheros del SO.
// Los temporales se guardan en un subdirectorio propio dentro del directorio temporal del sistema
// para facilitar la limpieza y evitar colisiones con otros procesos.
type AlmacenTemporal struct {
	mu         sync.Mutex
	directorio string
	activos    map[string]struct{}
}

// NuevoAlmacenTemporal crea un AlmacenTemporal en un subdirectorio del temporal del sistema.
// Debe llamarse a Cerrar al terminar para eliminar los temporales no borrados explicitamente.
func NuevoAlmacenTemporal() (*AlmacenTemporal, error) {
	dir, err := os.MkdirTemp("", "grxfirma-*")
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear el directorio de temporales: %w", err)
	}
	return &AlmacenTemporal{
		directorio: dir,
		activos:    make(map[string]struct{}),
	}, nil
}

// Write escribe los datos en un fichero temporal y devuelve su ruta absoluta.
func (a *AlmacenTemporal) Write(_ context.Context, data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("no se pueden escribir datos vacios en el almacen temporal")
	}

	f, err := os.CreateTemp(a.directorio, "tmp-*")
	if err != nil {
		return "", fmt.Errorf("no se pudo crear el fichero temporal: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("no se pudo escribir en el fichero temporal: %w", err)
	}

	ruta := f.Name()
	a.mu.Lock()
	a.activos[ruta] = struct{}{}
	a.mu.Unlock()

	return ruta, nil
}

// Delete elimina el fichero temporal indicado por su ruta.
func (a *AlmacenTemporal) Delete(_ context.Context, ruta string) error {
	// Verificar que la ruta pertenece a nuestro directorio para evitar path traversal.
	if !a.perteneceAlDirectorio(ruta) {
		return fmt.Errorf("la ruta '%s' no pertenece al almacen temporal gestionado", ruta)
	}

	if err := os.Remove(ruta); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("no se pudo eliminar el fichero temporal: %w", err)
	}

	a.mu.Lock()
	delete(a.activos, ruta)
	a.mu.Unlock()

	return nil
}

// Cerrar elimina todos los temporales activos y el directorio base.
// Se debe llamar al finalizar la sesion de uso del almacen.
func (a *AlmacenTemporal) Cerrar() error {
	if err := os.RemoveAll(a.directorio); err != nil {
		return fmt.Errorf("no se pudo limpiar el directorio de temporales: %w", err)
	}
	a.mu.Lock()
	a.activos = make(map[string]struct{})
	a.mu.Unlock()
	return nil
}

func (a *AlmacenTemporal) perteneceAlDirectorio(ruta string) bool {
	abs, err := filepath.Abs(ruta)
	if err != nil {
		return false
	}
	dirAbs, err := filepath.Abs(a.directorio)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(dirAbs, abs)
	if err != nil {
		return false
	}
	// La ruta no debe salir del directorio base (no empieza por "..")
	return len(rel) > 0 && rel[0] != '.'
}
