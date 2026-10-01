// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"encoding/xml"
	"sort"
	"strings"
)

// legacyExclusiveC14N reproduce el serializador simplificado utilizado por las
// primeras candidatas de GrxFirma. Solo se usa como candidato de lectura:
// todas las firmas nuevas emplean la canonicalización W3C de xml_c14n.go.
func legacyExclusiveC14N(xmlStr string) []byte {
	decoder := xml.NewDecoder(strings.NewReader(xmlStr))
	var output bytes.Buffer
	var namespaceStack []map[string]string

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch current := token.(type) {
		case xml.StartElement:
			parentScope := legacyNamespaceScope(namespaceStack)
			scope := make(map[string]string, len(parentScope))
			for prefix, uri := range parentScope {
				scope[prefix] = uri
			}

			elementPrefix := legacyResolvePrefix(current.Name.Space)
			tagName := current.Name.Local
			if elementPrefix != "" {
				tagName = elementPrefix + ":" + tagName
			}
			output.WriteByte('<')
			output.WriteString(tagName)

			type canonicalAttribute struct {
				name  string
				value string
			}
			var namespaceAttributes []canonicalAttribute
			var attributes []canonicalAttribute

			if current.Name.Space != "" && scope[elementPrefix] != current.Name.Space {
				namespaceAttributes = append(namespaceAttributes, canonicalAttribute{
					name:  "xmlns:" + elementPrefix,
					value: current.Name.Space,
				})
				scope[elementPrefix] = current.Name.Space
			}

			for _, attribute := range current.Attr {
				if attribute.Name.Space == "xmlns" || (attribute.Name.Space == "" && attribute.Name.Local == "xmlns") {
					prefix := attribute.Name.Local
					name := "xmlns:" + prefix
					if attribute.Name.Space == "" {
						prefix = ""
						name = "xmlns"
					}
					if scope[prefix] != attribute.Value {
						namespaceAttributes = append(namespaceAttributes, canonicalAttribute{name: name, value: attribute.Value})
					}
					scope[prefix] = attribute.Value
					continue
				}
				name := attribute.Name.Local
				if attribute.Name.Space != "" {
					prefix := legacyResolvePrefix(attribute.Name.Space)
					if scope[prefix] != attribute.Name.Space {
						namespaceAttributes = append(namespaceAttributes, canonicalAttribute{
							name:  "xmlns:" + prefix,
							value: attribute.Name.Space,
						})
						scope[prefix] = attribute.Name.Space
					}
					name = prefix + ":" + name
				}
				attributes = append(attributes, canonicalAttribute{name: name, value: attribute.Value})
			}

			sort.Slice(namespaceAttributes, func(i, j int) bool {
				return namespaceAttributes[i].name < namespaceAttributes[j].name
			})
			sort.Slice(attributes, func(i, j int) bool {
				return attributes[i].name < attributes[j].name
			})
			for _, attribute := range append(namespaceAttributes, attributes...) {
				output.WriteByte(' ')
				output.WriteString(attribute.name)
				output.WriteString(`="`)
				output.WriteString(escapeXMLAttr(attribute.value))
				output.WriteByte('"')
			}
			output.WriteByte('>')
			namespaceStack = append(namespaceStack, scope)

		case xml.EndElement:
			prefix := legacyResolvePrefix(current.Name.Space)
			tagName := current.Name.Local
			if prefix != "" {
				tagName = prefix + ":" + tagName
			}
			output.WriteString("</")
			output.WriteString(tagName)
			output.WriteByte('>')
			if len(namespaceStack) > 0 {
				namespaceStack = namespaceStack[:len(namespaceStack)-1]
			}

		case xml.CharData:
			output.WriteString(escapeXMLText(string(current)))
		}
	}
	return output.Bytes()
}

func legacyNamespaceScope(stack []map[string]string) map[string]string {
	if len(stack) == 0 {
		return map[string]string{}
	}
	return stack[len(stack)-1]
}

func legacyResolvePrefix(namespace string) string {
	switch namespace {
	case nsXMLDSig:
		return "ds"
	case nsXAdES:
		return "xades"
	default:
		return ""
	}
}
