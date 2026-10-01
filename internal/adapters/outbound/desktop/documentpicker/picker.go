// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package documentpicker

import (
	"context"
	"errors"
	"strings"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// ErrSeleccionCancelada se retorna cuando el usuario cancela el selector documental.
var ErrSeleccionCancelada = errors.New("selección documental cancelada por el usuario")

// Resolver abstrae la interacción concreta con el selector documental del escritorio.
type Resolver interface {
	ResolveDocument(ctx context.Context) (SelectedDocument, error)
}

// ResolveFunc adapta una función al contrato Resolver.
type ResolveFunc func(ctx context.Context) (SelectedDocument, error)

// ResolveDocument ejecuta la función adaptada.
func (f ResolveFunc) ResolveDocument(ctx context.Context) (SelectedDocument, error) {
	return f(ctx)
}

// SelectedDocument representa la carga cruda devuelta por la UI del selector documental.
type SelectedDocument struct {
	Name     string
	Content  []byte
	MIMEType string
}

// Picker implementa ports.DocumentPicker para escritorio.
type Picker struct {
	resolver Resolver
}

// Nuevo crea un Picker asociado a un resolver concreto.
func Nuevo(resolver Resolver) *Picker {
	return &Picker{resolver: resolver}
}

// Pick abre el selector documental y traduce el resultado a dominio.
func (p Picker) Pick(ctx context.Context) (domain.Document, error) {
	if err := ctx.Err(); err != nil {
		return domain.Document{}, err
	}
	if p.resolver == nil {
		return domain.Document{}, errors.New("desktop-document-picker: selector documental no configurado")
	}

	selected, err := p.resolver.ResolveDocument(ctx)
	if err != nil {
		return domain.Document{}, err
	}

	name := strings.TrimSpace(selected.Name)
	if name == "" {
		name = "documento"
	}
	mimeType := strings.TrimSpace(selected.MIMEType)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return domain.NewDocument(name, selected.Content, mimeType)
}

var _ ports.DocumentPicker = (*Picker)(nil)
