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
	"sort"
	"strings"
)

const xmlNamespaceURI = "http://www.w3.org/XML/1998/namespace"

type c14nMode struct {
	inclusive bool
}

type c14nElement struct {
	prefix       string
	local        string
	namespaceURI string
	attributes   []xml.Attr
}

type c14nFrame struct {
	sourceNamespaces map[string]string
	sourceXMLAttrs   map[string]string
	renderedNS       map[string]string
	renderedXMLAttrs map[string]string
	captured         bool
	prefix           string
	local            string
}

type c14nAttribute struct {
	qname        string
	local        string
	namespaceURI string
	value        string
}

// canonicalizeXML aplica el algoritmo indicado a la raíz de un documento o
// fragmento XML bien formado. Usa RawToken para conservar los prefijos
// literales, que encoding/xml.Token traduce a URI y no permite reconstruir de
// forma fiable cuando varios prefijos apuntan al mismo espacio de nombres.
//
// Canonicaliza el documento entero: incluye las instrucciones de proceso de
// fuera del elemento raíz (C14N 1.0 §2.1 y §2.3), que un visor sí aplica.
func canonicalizeXML(xmlData []byte, algorithm string) ([]byte, error) {
	rootFound := false
	return canonicalizarSeleccion(xmlData, algorithm, func(c14nElement) bool {
		if rootFound {
			return false
		}
		rootFound = true
		return true
	}, true)
}

// canonicalizeElementInDocument conserva el contexto xmlns y xml:* heredado
// por el elemento seleccionado, necesario para C14N inclusiva.
func canonicalizeElementInDocument(xmlData []byte, namespace, local, algorithm string) ([]byte, error) {
	return canonicalizeXMLSelection(xmlData, algorithm, func(element c14nElement) bool {
		return element.local == local && (namespace == "" || element.namespaceURI == namespace)
	})
}

func canonicalizeElementByIDInDocument(xmlData []byte, id, algorithm string) ([]byte, error) {
	return canonicalizeXMLSelection(xmlData, algorithm, func(element c14nElement) bool {
		for _, attribute := range element.attributes {
			if strings.EqualFold(attribute.Name.Local, "Id") && attribute.Value == id {
				return true
			}
		}
		return false
	})
}

func canonicalizeXMLSelection(xmlData []byte, algorithm string, matches func(c14nElement) bool) ([]byte, error) {
	return canonicalizarSeleccion(xmlData, algorithm, matches, false)
}

// canonicalizarSeleccion canonicaliza el primer elemento que cumple matches.
// Con documentoEntero, la selección es el nodo documento y se emiten también
// las instrucciones de proceso anteriores y posteriores al elemento raíz.
func canonicalizarSeleccion(xmlData []byte, algorithm string, matches func(c14nElement) bool, documentoEntero bool) ([]byte, error) {
	mode, err := resolveC14NMode(algorithm)
	if err != nil {
		return nil, err
	}

	decoder := xml.NewDecoder(bytes.NewReader(normalizarEspaciosEnAtributos(xmlData)))
	decoder.Strict = true
	var output bytes.Buffer
	frames := make([]c14nFrame, 0, 16)
	found := false
	captureDepth := -1

	for {
		token, tokenErr := decoder.RawToken()
		if errors.Is(tokenErr, io.EOF) {
			break
		}
		if tokenErr != nil {
			return nil, fmt.Errorf("parseando XML para canonicalizar: %w", tokenErr)
		}

		switch current := token.(type) {
		case xml.StartElement:
			parentNamespaces := map[string]string{"xml": xmlNamespaceURI}
			parentXMLAttrs := map[string]string{}
			parentRenderedNS := map[string]string{}
			parentRenderedXMLAttrs := map[string]string{}
			parentCaptured := false
			if len(frames) > 0 {
				parent := frames[len(frames)-1]
				parentNamespaces = parent.sourceNamespaces
				parentXMLAttrs = parent.sourceXMLAttrs
				parentRenderedNS = parent.renderedNS
				parentRenderedXMLAttrs = parent.renderedXMLAttrs
				parentCaptured = parent.captured
			}

			sourceNamespaces := cloneStringMap(parentNamespaces)
			sourceXMLAttrs := cloneStringMap(parentXMLAttrs)
			for _, attribute := range current.Attr {
				switch {
				case attribute.Name.Space == "xmlns":
					sourceNamespaces[attribute.Name.Local] = attribute.Value
				case attribute.Name.Space == "" && attribute.Name.Local == "xmlns":
					sourceNamespaces[""] = attribute.Value
				case attribute.Name.Space == "xml":
					sourceXMLAttrs[attribute.Name.Local] = attribute.Value
				}
			}

			namespaceURI, namespaceErr := resolveElementNamespace(current.Name.Space, sourceNamespaces)
			if namespaceErr != nil {
				return nil, namespaceErr
			}
			element := c14nElement{
				prefix:       current.Name.Space,
				local:        current.Name.Local,
				namespaceURI: namespaceURI,
				attributes:   current.Attr,
			}
			captured := parentCaptured
			if !found && matches(element) {
				found = true
				captured = true
				captureDepth = len(frames)
				parentRenderedNS = map[string]string{}
				parentRenderedXMLAttrs = map[string]string{}
			}

			renderedNS := cloneStringMap(parentRenderedNS)
			renderedXMLAttrs := cloneStringMap(parentRenderedXMLAttrs)
			if captured {
				var writeErr error
				renderedNS, renderedXMLAttrs, writeErr = writeCanonicalStart(
					&output,
					current,
					sourceNamespaces,
					sourceXMLAttrs,
					parentRenderedNS,
					parentRenderedXMLAttrs,
					mode,
					len(frames) == captureDepth,
				)
				if writeErr != nil {
					return nil, writeErr
				}
			}
			frames = append(frames, c14nFrame{
				sourceNamespaces: sourceNamespaces,
				sourceXMLAttrs:   sourceXMLAttrs,
				renderedNS:       renderedNS,
				renderedXMLAttrs: renderedXMLAttrs,
				captured:         captured,
				prefix:           current.Name.Space,
				local:            current.Name.Local,
			})

		case xml.EndElement:
			if len(frames) == 0 {
				return nil, errors.New("cierre XML sin elemento abierto")
			}
			frame := frames[len(frames)-1]
			if frame.captured {
				output.WriteString("</")
				output.WriteString(qualifiedXMLName(frame.prefix, frame.local))
				output.WriteByte('>')
			}
			frames = frames[:len(frames)-1]

		case xml.CharData:
			if len(frames) > 0 && frames[len(frames)-1].captured {
				output.WriteString(escapeXMLText(string(current)))
			}

		case xml.Directive:
			// encoding/xml no aplica los valores por defecto ni las entidades
			// de un subconjunto interno de DTD; un visor sí. No se puede
			// canonicalizar ese documento como lo vería quien lo muestra.
			if bytes.HasPrefix(bytes.TrimSpace(current), []byte("DOCTYPE")) && bytes.ContainsRune(current, '[') {
				return nil, errors.New("documento XML con DTD interna no admitido")
			}

		case xml.ProcInst:
			if len(frames) == 0 && documentoEntero && current.Target != "xml" {
				if found {
					output.WriteByte('\n')
				}
				output.WriteString("<?")
				output.WriteString(current.Target)
				if len(current.Inst) > 0 {
					output.WriteByte(' ')
					output.Write(current.Inst)
				}
				output.WriteString("?>")
				if !found {
					output.WriteByte('\n')
				}
				continue
			}
			if len(frames) > 0 && frames[len(frames)-1].captured && current.Target != "xml" {
				output.WriteString("<?")
				output.WriteString(current.Target)
				if len(current.Inst) > 0 {
					output.WriteByte(' ')
					output.Write(current.Inst)
				}
				output.WriteString("?>")
			}
		}
	}

	if len(frames) != 0 {
		return nil, errors.New("documento XML incompleto")
	}
	if !found {
		return nil, errors.New("elemento XML a canonicalizar no encontrado")
	}
	return output.Bytes(), nil
}

func resolveC14NMode(algorithm string) (c14nMode, error) {
	switch strings.TrimSpace(algorithm) {
	case "", algExcC14N:
		return c14nMode{}, nil
	case algC14N:
		return c14nMode{inclusive: true}, nil
	default:
		return c14nMode{}, fmt.Errorf("algoritmo de canonicalización XML no soportado: %s", algorithm)
	}
}

func writeCanonicalStart(
	output *bytes.Buffer,
	start xml.StartElement,
	sourceNamespaces map[string]string,
	sourceXMLAttrs map[string]string,
	parentRenderedNS map[string]string,
	parentRenderedXMLAttrs map[string]string,
	mode c14nMode,
	selectionRoot bool,
) (map[string]string, map[string]string, error) {
	output.WriteByte('<')
	output.WriteString(qualifiedXMLName(start.Name.Space, start.Name.Local))

	namespacesToRender := make(map[string]string)
	if mode.inclusive {
		for prefix, uri := range sourceNamespaces {
			if prefix == "xml" {
				continue
			}
			if renderedURI, rendered := parentRenderedNS[prefix]; uri != renderedURI || (!rendered && uri != "") {
				namespacesToRender[prefix] = uri
			}
		}
		for prefix, renderedURI := range parentRenderedNS {
			if prefix == "xml" {
				continue
			}
			if _, exists := sourceNamespaces[prefix]; !exists && renderedURI != "" {
				namespacesToRender[prefix] = ""
			}
		}
	} else {
		visiblePrefixes := map[string]bool{start.Name.Space: true}
		for _, attribute := range start.Attr {
			if attribute.Name.Space != "" && attribute.Name.Space != "xmlns" && attribute.Name.Space != "xml" {
				visiblePrefixes[attribute.Name.Space] = true
			}
		}
		for prefix := range visiblePrefixes {
			uri, exists := sourceNamespaces[prefix]
			if prefix != "" && !exists {
				return nil, nil, fmt.Errorf("prefijo de espacio de nombres no declarado: %s", prefix)
			}
			if !exists {
				uri = ""
			}
			if renderedURI, rendered := parentRenderedNS[prefix]; uri != renderedURI || (!rendered && uri != "") {
				namespacesToRender[prefix] = uri
			}
		}
	}

	namespacePrefixes := make([]string, 0, len(namespacesToRender))
	for prefix := range namespacesToRender {
		namespacePrefixes = append(namespacePrefixes, prefix)
	}
	sort.Strings(namespacePrefixes)
	for _, prefix := range namespacePrefixes {
		output.WriteByte(' ')
		if prefix == "" {
			output.WriteString("xmlns")
		} else {
			output.WriteString("xmlns:")
			output.WriteString(prefix)
		}
		output.WriteString(`="`)
		output.WriteString(escapeXMLAttr(namespacesToRender[prefix]))
		output.WriteByte('"')
	}

	attributes := make([]c14nAttribute, 0, len(start.Attr)+len(sourceXMLAttrs))
	localXMLAttrs := make(map[string]string)
	for _, attribute := range start.Attr {
		switch {
		case attribute.Name.Space == "xmlns" || (attribute.Name.Space == "" && attribute.Name.Local == "xmlns"):
			continue
		case attribute.Name.Space == "xml":
			localXMLAttrs[attribute.Name.Local] = attribute.Value
			attributes = append(attributes, c14nAttribute{
				qname:        qualifiedXMLName("xml", attribute.Name.Local),
				local:        attribute.Name.Local,
				namespaceURI: xmlNamespaceURI,
				value:        attribute.Value,
			})
		default:
			namespaceURI := ""
			if attribute.Name.Space != "" {
				var exists bool
				namespaceURI, exists = sourceNamespaces[attribute.Name.Space]
				if !exists {
					return nil, nil, fmt.Errorf("prefijo de atributo no declarado: %s", attribute.Name.Space)
				}
			}
			attributes = append(attributes, c14nAttribute{
				qname:        qualifiedXMLName(attribute.Name.Space, attribute.Name.Local),
				local:        attribute.Name.Local,
				namespaceURI: namespaceURI,
				value:        attribute.Value,
			})
		}
	}

	renderedXMLAttrs := cloneStringMap(parentRenderedXMLAttrs)
	if mode.inclusive && selectionRoot {
		for local, value := range sourceXMLAttrs {
			if _, declaredLocally := localXMLAttrs[local]; declaredLocally {
				continue
			}
			attributes = append(attributes, c14nAttribute{
				qname:        qualifiedXMLName("xml", local),
				local:        local,
				namespaceURI: xmlNamespaceURI,
				value:        value,
			})
		}
	}
	for local, value := range localXMLAttrs {
		renderedXMLAttrs[local] = value
	}

	sort.Slice(attributes, func(i, j int) bool {
		if attributes[i].namespaceURI != attributes[j].namespaceURI {
			return attributes[i].namespaceURI < attributes[j].namespaceURI
		}
		return attributes[i].local < attributes[j].local
	})
	for _, attribute := range attributes {
		output.WriteByte(' ')
		output.WriteString(attribute.qname)
		output.WriteString(`="`)
		output.WriteString(escapeXMLAttr(attribute.value))
		output.WriteByte('"')
	}
	output.WriteByte('>')

	renderedNS := cloneStringMap(parentRenderedNS)
	for prefix, uri := range namespacesToRender {
		renderedNS[prefix] = uri
	}
	return renderedNS, renderedXMLAttrs, nil
}

func resolveElementNamespace(prefix string, namespaces map[string]string) (string, error) {
	if prefix == "" {
		return namespaces[""], nil
	}
	namespace, exists := namespaces[prefix]
	if !exists {
		return "", fmt.Errorf("prefijo de elemento no declarado: %s", prefix)
	}
	return namespace, nil
}

func qualifiedXMLName(prefix, local string) string {
	if prefix == "" {
		return local
	}
	return prefix + ":" + local
}

func cloneStringMap(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

// normalizarEspaciosEnAtributos aplica la normalización de valores de
// atributo de XML 1.0 §3.3.3 que encoding/xml no hace: el tabulador, el salto
// de línea y el retorno de carro escritos tal cual dentro de un valor de
// atributo valen un espacio (y "\r\n" uno solo). Los que llegan como
// referencia de carácter (&#9;, &#xA;...) no se tocan, porque aquí aún no
// están resueltos. Comentarios, CDATA, instrucciones de proceso y
// declaraciones se copian sin cambios.
func normalizarEspaciosEnAtributos(data []byte) []byte {
	if bytes.IndexAny(data, "\t\n\r") < 0 {
		return data
	}
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); {
		if data[i] != '<' {
			out = append(out, data[i])
			i++
			continue
		}
		resto := data[i:]
		var cierre string
		switch {
		case bytes.HasPrefix(resto, []byte("<!--")):
			cierre = "-->"
		case bytes.HasPrefix(resto, []byte("<![CDATA[")):
			cierre = "]]>"
		case bytes.HasPrefix(resto, []byte("<?")):
			cierre = "?>"
		}
		if cierre != "" {
			fin := bytes.Index(resto, []byte(cierre))
			if fin < 0 {
				return append(out, resto...)
			}
			out = append(out, resto[:fin+len(cierre)]...)
			i += fin + len(cierre)
			continue
		}
		if bytes.HasPrefix(resto, []byte("<!")) {
			// DOCTYPE u otra declaración: se copia hasta el primer '>' fuera de
			// comillas (no lleva atributos de elemento).
			out = append(out, '<')
			i++
			var comilla byte
			for i < len(data) {
				c := data[i]
				out = append(out, c)
				i++
				if comilla != 0 {
					if c == comilla {
						comilla = 0
					}
				} else if c == '"' || c == '\'' {
					comilla = c
				} else if c == '>' {
					break
				}
			}
			continue
		}
		// Etiqueta de elemento: dentro de comillas se normaliza.
		out = append(out, '<')
		i++
		var comilla byte
		for i < len(data) {
			c := data[i]
			i++
			if comilla != 0 {
				switch {
				case c == comilla:
					comilla = 0
					out = append(out, c)
				case c == '\r' && i < len(data) && data[i] == '\n':
					out = append(out, ' ')
					i++
				case c == '\t' || c == '\n' || c == '\r':
					out = append(out, ' ')
				default:
					out = append(out, c)
				}
				continue
			}
			out = append(out, c)
			if c == '"' || c == '\'' {
				comilla = c
			} else if c == '>' {
				break
			}
		}
	}
	return out
}
