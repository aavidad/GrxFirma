// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import "errors"

// Document representa un documento sobre el que se va a operar (firmar, verificar).
// El contenido es inmutable una vez creado.
type Document struct {
	Name     string
	Content  []byte
	MIMEType string
}

func NewDocument(name string, content []byte, mimeType string) (Document, error) {
	if len(content) == 0 {
		return Document{}, errors.New("el contenido del documento no puede estar vacio")
	}
	return Document{Name: name, Content: content, MIMEType: mimeType}, nil
}

func (d Document) Size() int {
	return len(d.Content)
}
