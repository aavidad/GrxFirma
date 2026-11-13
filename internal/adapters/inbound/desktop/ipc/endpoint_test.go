// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// D-07: sin ruta explícita no existe un nombre por defecto predecible que otro
// usuario pueda ocupar antes; Escuchar debe negarse.
func TestEscuchar_RutaVaciaDevuelveError(t *testing.T) {
	t.Parallel()
	srv := New(&Manejador{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Escuchar(ctx, "") }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Escuchar(\"\") debe devolver error")
		}
	case <-time.After(time.Second):
		srv.Cerrar()
		t.Fatal("Escuchar(\"\") no debe abrir un endpoint por defecto")
	}
}

// D-07: la comprobación de zonas del sistema compara por componentes de ruta,
// no por prefijo de texto.
func TestRutaEnZonas_ComparaPorComponentes(t *testing.T) {
	t.Parallel()
	raiz := string(filepath.Separator) + "sistema"
	zona := filepath.Join(raiz, "privado")
	zonas := []string{zona}
	casos := []struct {
		ruta      string
		prohibida bool
	}{
		{zona, true},
		{filepath.Join(zona, "clave"), true},
		{filepath.Join(zona, "a", "..", "b"), true},
		{zona + "-copia", false},
		{filepath.Join(raiz, "otro"), false},
	}
	for _, c := range casos {
		err := rutaEnZonas(filepath.Clean(c.ruta), zonas)
		if (err != nil) != c.prohibida {
			t.Fatalf("rutaEnZonas(%q) = %v; prohibida=%v", c.ruta, err, c.prohibida)
		}
	}
}
