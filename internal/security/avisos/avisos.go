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
	"context"
	"log/slog"
	"strings"
	"sync"
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
)

// Registrar anota un aviso para la operación en curso. Los duplicados se
// ignoran para no repetir el mismo motivo en cada fase.
func Registrar(titulo, motivo string) {
	titulo = strings.TrimSpace(titulo)
	motivo = strings.TrimSpace(motivo)
	if titulo == "" {
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

// Recolector junta las claves de catálogo de los avisos de UNA operación
// (por ejemplo, una firma pedida por Qt o WinUI). A diferencia de Registrar,
// que guarda textos en una lista global para afirma://, viaja en el contexto
// de la petición: dos operaciones a la vez no mezclan sus avisos y el
// resultado los lleva para que la interfaz los traduzca (lote 16d).
type Recolector struct {
	mu     sync.Mutex
	claves []string
}

type claveContextoRecolector struct{}

// ConRecolector devuelve un contexto con un recolector nuevo. Un recolector
// anidado recoge solo lo que se anota con su contexto.
func ConRecolector(ctx context.Context) (context.Context, *Recolector) {
	if ctx == nil {
		ctx = context.Background()
	}
	r := &Recolector{}
	return context.WithValue(ctx, claveContextoRecolector{}, r), r
}

// Anotar añade la clave de catálogo de un aviso al recolector del contexto.
// Sin recolector no hace nada: el llamador que no lo pide (la CLI, el móvil)
// sigue con su registro y sus avisos de siempre.
func Anotar(ctx context.Context, clave string) {
	clave = strings.TrimSpace(clave)
	if ctx == nil || clave == "" {
		return
	}
	r, _ := ctx.Value(claveContextoRecolector{}).(*Recolector)
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.claves {
		if c == clave {
			return
		}
	}
	if len(r.claves) < maxAvisos {
		r.claves = append(r.claves, clave)
	}
}

// Claves devuelve una copia de las claves anotadas, en el orden en que se
// anotaron; nil si no hay ninguna.
func (r *Recolector) Claves() []string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.claves) == 0 {
		return nil
	}
	return append([]string(nil), r.claves...)
}
