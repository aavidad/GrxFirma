// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobiledocumentpicker

import (
	"context"
	"errors"
	"strings"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// Resolver abstrae el bridge nativo que interactua con Android SAF o UIDocumentPicker.
type Resolver interface {
	ResolveDocument(ctx context.Context) (SelectedDocument, error)
}

// ResolveFunc adapta una funcion al contrato Resolver.
type ResolveFunc func(ctx context.Context) (SelectedDocument, error)

// ResolveDocument ejecuta la funcion adaptada.
func (f ResolveFunc) ResolveDocument(ctx context.Context) (SelectedDocument, error) {
	return f(ctx)
}

// SelectedDocument representa la carga cruda devuelta por la capa nativa.
type SelectedDocument struct {
	Name     string
	Content  []byte
	MIMEType string
}

// Picker implementa la selección documental nativa de mobile.
type Picker struct {
	resolver Resolver
}

// Nuevo crea un Picker asociado a un resolver nativo.
func Nuevo(resolver Resolver) *Picker {
	return &Picker{resolver: resolver}
}

// Pick abre el selector documental de la plataforma.
func (p Picker) Pick(ctx context.Context) (domain.Document, error) {
	if err := ctx.Err(); err != nil {
		return domain.Document{}, err
	}
	if p.resolver == nil {
		return domain.Document{}, errors.New("mobile-document-picker: bridge nativo no configurado")
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

	document, err := domain.NewDocument(name, selected.Content, mimeType)
	if err != nil {
		return domain.Document{}, err
	}
	return document, nil
}

var _ ports.DocumentPicker = (*Picker)(nil)
