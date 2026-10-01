// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// xmlElementSpan describe un elemento real del documento según el parser XML
// (nunca una coincidencia textual dentro de comentarios, CDATA o atributos).
type xmlElementSpan struct {
	name  xml.Name
	id    string
	start int64
	end   int64
}

func scanXMLElements(xmlData []byte) ([]xmlElementSpan, error) {
	decoder := xml.NewDecoder(bytes.NewReader(xmlData))
	var (
		spans []xmlElementSpan
		stack []int
	)
	for {
		offset := decoder.InputOffset()
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch node := token.(type) {
		case xml.StartElement:
			span := xmlElementSpan{name: node.Name, start: offset, end: -1}
			for _, attribute := range node.Attr {
				if strings.EqualFold(attribute.Name.Local, "Id") {
					if span.id != "" {
						return nil, errors.New("elemento XML con varios atributos Id")
					}
					span.id = attribute.Value
				}
			}
			stack = append(stack, len(spans))
			spans = append(spans, span)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, errors.New("cierre XML sin apertura")
			}
			spans[stack[len(stack)-1]].end = decoder.InputOffset()
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) != 0 {
		return nil, errors.New("documento XML incompleto")
	}
	return spans, nil
}

// uniqueElementByID exige que el Id referenciado identifique exactamente un
// elemento. Con Id duplicados, el verificador y quien muestra el documento
// podrían resolver nodos distintos (XML Signature Wrapping).
func uniqueElementByID(spans []xmlElementSpan, id string) (xmlElementSpan, error) {
	var found []xmlElementSpan
	for _, span := range spans {
		if span.id == id {
			found = append(found, span)
		}
	}
	if len(found) != 1 {
		return xmlElementSpan{}, fmt.Errorf("el Id %q no identifica un único elemento (%d coincidencias)", id, len(found))
	}
	return found[0], nil
}

// literalElementOffset localiza "<tag" como etiqueta completa (el carácter
// siguiente debe cerrar el nombre), evitando confundir <CONTENT con <CONTENTX.
func literalElementOffset(source, tag string) int {
	from := 0
	for {
		index := strings.Index(source[from:], "<"+tag)
		if index < 0 {
			return -1
		}
		index += from
		next := index + 1 + len(tag)
		if next >= len(source) {
			return -1
		}
		switch source[next] {
		case ' ', '\t', '\r', '\n', '>', '/':
			return index
		}
		from = next
	}
}

// literalMatchesSpan confirma que la etiqueta literal es exactamente el
// elemento que el parser identificó (y no texto en un comentario o CDATA).
func literalMatchesSpan(xmlData []byte, tag string, span xmlElementSpan) bool {
	return int64(literalElementOffset(string(xmlData), tag)) == span.start
}

// referencedSignedProperties selecciona el SignedProperties que la propia
// firma referencia desde su SignedInfo. Antes se usaba el primer literal
// "<xades:SignedProperties" del fragmento, que podía ser uno no firmado.
func referencedSignedProperties(xmlData, signedInfoXML []byte) ([]byte, error) {
	var info signedInfoForVerify
	if err := xml.Unmarshal(signedInfoXML, &info); err != nil {
		return nil, err
	}
	referenced := map[string]struct{}{}
	for _, ref := range info.References {
		uri := strings.TrimSpace(ref.URI)
		if strings.HasPrefix(uri, "#") && len(uri) > 1 {
			referenced[uri[1:]] = struct{}{}
		}
	}
	spans, err := scanXMLElements(xmlData)
	if err != nil {
		return nil, err
	}
	var selected []xmlElementSpan
	for _, span := range spans {
		if span.name.Local != "SignedProperties" || !strings.HasPrefix(span.name.Space, "http://uri.etsi.org/01903/") {
			continue
		}
		if _, ok := referenced[span.id]; ok && span.id != "" {
			selected = append(selected, span)
		}
	}
	if len(selected) != 1 {
		return nil, fmt.Errorf("se esperaba un único SignedProperties referenciado por SignedInfo y hay %d", len(selected))
	}
	if _, err := uniqueElementByID(spans, selected[0].id); err != nil {
		return nil, err
	}
	span := selected[0]
	if span.start < 0 || span.end <= span.start || span.end > int64(len(xmlData)) {
		return nil, errors.New("SignedProperties fuera de rango")
	}
	return append([]byte(nil), xmlData[span.start:span.end]...), nil
}
