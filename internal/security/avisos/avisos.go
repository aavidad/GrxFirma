// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package avisos recoge las decisiones automáticas que la aplicación toma por
// compatibilidad o por seguridad durante una operación (elevar SHA-1 a
// SHA-256, usar el intercambio heredado de AutoFirma 1.x, rechazar un
// destino peligroso...). La interfaz las muestra al usuario con su motivo:
// ninguna decisión que afecte a la firma debe quedar solo en el registro.
package avisos

import (
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
)

// Aviso es una decisión automática explicada al usuario.
type Aviso struct {
	Titulo string
	Motivo string
}

const maxAvisos = 16

var (
	mu    sync.Mutex
	lista []Aviso
	// soloRegistro: el proceso no enseña esta lista (lote 17b).
	soloRegistro atomic.Bool
)

// SoloEnRegistro deja de guardar avisos en la lista global de este proceso:
// desde ese momento Registrar solo los anota en el registro. La usan los
// procesos de larga duración que atienden varias operaciones y no leen la
// lista, como el motor de escritorio, que da los avisos de cada firma por su
// propio registro de operación (domain.AnotarAviso). Así la lista no se
// llena con avisos de operaciones distintas que nadie va a enseñar. Quien sí
// la enseña (afirma://) la vacía con Reiniciar al empezar cada operación.
func SoloEnRegistro() {
	soloRegistro.Store(true)
	Reiniciar()
}

// Registrar anota un aviso para la operación en curso. Los duplicados se
// ignoran para no repetir el mismo motivo en cada fase.
func Registrar(titulo, motivo string) {
	titulo = strings.TrimSpace(titulo)
	motivo = strings.TrimSpace(motivo)
	if titulo == "" {
		return
	}
	if soloRegistro.Load() {
		slog.Info("aviso_usuario", "titulo", titulo)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for _, a := range lista {
		if a.Titulo == titulo && a.Motivo == motivo {
			return
		}
	}
	if len(lista) >= maxAvisos {
		return
	}
	lista = append(lista, Aviso{Titulo: titulo, Motivo: motivo})
	slog.Info("aviso_usuario", "titulo", titulo)
}

// Todos devuelve una copia de los avisos registrados.
func Todos() []Aviso {
	mu.Lock()
	defer mu.Unlock()
	return append([]Aviso(nil), lista...)
}

// Texto presenta los avisos para un diálogo; vacío si no hay ninguno.
func Texto() string {
	avisos := Todos()
	if len(avisos) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Avisos de esta operación:")
	for _, a := range avisos {
		b.WriteString("\n• ")
		b.WriteString(a.Titulo)
		if a.Motivo != "" {
			b.WriteString(": ")
			b.WriteString(a.Motivo)
		}
	}
	return b.String()
}

// ConAvisos añade los avisos pendientes a un texto de la interfaz.
func ConAvisos(texto string) string {
	extra := Texto()
	if extra == "" {
		return texto
	}
	if strings.TrimSpace(texto) == "" {
		return extra
	}
	return texto + "\n\n" + extra
}

// Reiniciar vacía los avisos al empezar una operación nueva.
func Reiniciar() {
	mu.Lock()
	defer mu.Unlock()
	lista = nil
}
