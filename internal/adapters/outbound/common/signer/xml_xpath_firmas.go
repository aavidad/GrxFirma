// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// AutoFirma Java añade a toda firma XAdES Enveloped la transformación XPath
// not(ancestor-or-self::ds:Signature), que excluye cualquier firma del
// documento; así una cofirma no invalida las firmas anteriores. No se evalúa
// XPath arbitrario: solo se reconoce exactamente esta expresión.
const (
	algXPathFilter          = "http://www.w3.org/TR/1999/REC-xpath-19991116"
	xpathExcluirFirmasJava  = "not(ancestor-or-self::ds:Signature)"
	xpathTransformFirmasXML = `<ds:XPath xmlns:ds="` + nsXMLDSig + `">` + xpathExcluirFirmasJava + `</ds:XPath>`
)

// esXPathExcluirFirmas indica si los parámetros de una transformación XPath
// son exactamente la exclusión de firmas de AutoFirma Java.
func esXPathExcluirFirmas(transform algorithmAttr) bool {
	if strings.TrimSpace(transform.Algorithm) != algXPathFilter {
		return false
	}
	decoder := xml.NewDecoder(strings.NewReader(transform.Parameters))
	var (
		expresion strings.Builder
		prefijo   string
		elementos int
	)
	for {
		tok, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return false
		}
		switch t := tok.(type) {
		case xml.StartElement:
			elementos++
			if t.Name.Local != "XPath" || elementos > 1 {
				return false
			}
			for _, a := range t.Attr {
				if a.Name.Space == "xmlns" && a.Value == nsXMLDSig {
					prefijo = a.Name.Local
				}
			}
			if prefijo == "" && t.Name.Space == nsXMLDSig {
				prefijo = "ds"
			}
		case xml.CharData:
			if elementos == 1 {
				expresion.Write(t)
			}
		}
	}
	if elementos != 1 || prefijo == "" {
		return false
	}
	compacta := strings.Join(strings.Fields(expresion.String()), "")
	return compacta == "not(ancestor-or-self::"+prefijo+":Signature)"
}

// quitarTodasLasFirmas elimina del documento todas las ds:Signature (las más
// externas), usando offsets del parser, no búsquedas de texto.
func quitarTodasLasFirmas(document []byte) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(document))
	var (
		out        []byte
		copiado    int64
		profundo   int
		inicioFirm int64 = -1
	)
	for {
		offset := decoder.InputOffset()
		tok, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if inicioFirm >= 0 {
				profundo++
			} else if t.Name.Space == nsXMLDSig && t.Name.Local == "Signature" {
				inicioFirm = offset
				profundo = 0
			}
		case xml.EndElement:
			if inicioFirm < 0 {
				continue
			}
			if profundo > 0 {
				profundo--
				continue
			}
			fin := decoder.InputOffset()
			out = append(out, document[copiado:inicioFirm]...)
			copiado = fin
			inicioFirm = -1
		}
	}
	return append(out, document[copiado:]...), nil
}
