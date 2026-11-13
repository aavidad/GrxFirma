// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Utilidades de análisis textual de objetos PDF que usa la clasificación de
// actualizaciones incrementales del análisis v2 (pades_v2_changes.go).
//
// El antiguo analizador textual (pdfIncrementalAnalyzer) se retiró: buscaba
// objetos con una expresión regular sin seguir la xref, y un atacante podía
// redefinir objetos que el visor sí resuelve (hallazgo E2-02). El
// verificador PAdES usa ahora el análisis v2, que sigue la cadena xref/Prev.

// maxPDFObjectStreamBytes acota el cuerpo de un objeto que se analiza.
const maxPDFObjectStreamBytes = 64 << 20

var pdfObjectHeader = regexp.MustCompile(`(?:^|[\x00\t\n\f\r ])(\d{1,10})[\x00\t\n\f\r ]+(\d{1,5})[\x00\t\n\f\r ]+obj\b`)

type pdfLookup func(int) (string, bool)

func checkCatalogUpdate(oldDict, newDict map[string]string, lookupOld, lookupNew pdfLookup) string {
	if msg := changedKeysOutside(oldDict, newDict, "/DSS", "/AcroForm", "/Extensions", "/Perms", "/Metadata"); msg != "" {
		return msg
	}
	oldForm, newForm := oldDict["/AcroForm"], newDict["/AcroForm"]
	if oldForm == newForm {
		return ""
	}
	if _, isRef := pdfSingleRef(newForm); isRef {
		if _, wasRef := pdfSingleRef(oldForm); wasRef || oldForm == "" {
			return ""
		}
	}
	oldInline, _ := parsePDFDictValue(oldForm)
	newInline, ok := parsePDFDictValue(newForm)
	if !ok {
		return "/AcroForm con formato no admitido"
	}
	if oldInline == nil {
		oldInline = map[string]string{}
	}
	return checkAcroFormUpdate(oldInline, newInline, lookupOld, lookupNew)
}

func checkPageUpdate(oldDict, newDict map[string]string, lookupOld, lookupNew pdfLookup, appended map[int]string) string {
	if msg := changedKeysOutside(oldDict, newDict, "/Annots"); msg != "" {
		return msg
	}
	return checkAnnotsUpdate(oldDict["/Annots"], newDict["/Annots"], lookupOld, lookupNew, appended)
}

// checkAnnotsUpdate admite solo añadir widgets de firma: ninguna anotación
// firmada desaparece y las nuevas son campos /Sig definidos en la propia
// actualización.
func checkAnnotsUpdate(oldValue, newValue string, lookupOld, lookupNew pdfLookup, appended map[int]string) string {
	oldAnnots, okOld := resolvePDFRefArray(oldValue, lookupOld)
	newAnnots, okNew := resolvePDFRefArray(newValue, lookupNew)
	if !okOld || !okNew {
		return "/Annots con formato no admitido"
	}
	previous := make(map[int]struct{}, len(oldAnnots))
	for _, ref := range oldAnnots {
		previous[ref] = struct{}{}
	}
	current := make(map[int]struct{}, len(newAnnots))
	for _, ref := range newAnnots {
		current[ref] = struct{}{}
	}
	for ref := range previous {
		if _, ok := current[ref]; !ok {
			return "se eliminan anotaciones firmadas"
		}
	}
	for ref := range current {
		if _, ok := previous[ref]; ok {
			continue
		}
		body, ok := appended[ref]
		if !ok {
			return fmt.Sprintf("se añade la anotación %d, que no forma parte de la actualización", ref)
		}
		annot, ok := parsePDFObjectDict(body)
		if !ok || pdfName(annot["/Subtype"]) != "/Widget" {
			return fmt.Sprintf("se añade una anotación %d que no es un campo de firma", ref)
		}
		if ft := pdfName(annot["/FT"]); ft != "" && ft != "/Sig" {
			return fmt.Sprintf("se añade un campo de formulario %s", ft)
		}
	}
	return ""
}

func checkRefSuperset(oldValue, newValue string, lookupOld, lookupNew pdfLookup) string {
	oldRefs, okOld := resolvePDFRefArray(oldValue, lookupOld)
	newRefs, okNew := resolvePDFRefArray(newValue, lookupNew)
	if !okOld || !okNew {
		return "lista de referencias con formato no admitido"
	}
	current := make(map[int]struct{}, len(newRefs))
	for _, ref := range newRefs {
		current[ref] = struct{}{}
	}
	for _, ref := range oldRefs {
		if _, ok := current[ref]; !ok {
			return "se eliminan elementos firmados"
		}
	}
	return ""
}

func checkAcroFormUpdate(oldDict, newDict map[string]string, lookupOld, lookupNew pdfLookup) string {
	if _, ok := newDict["/XFA"]; ok && newDict["/XFA"] != oldDict["/XFA"] {
		return "se añade o cambia contenido XFA"
	}
	if msg := changedKeysOutside(oldDict, newDict, "/Fields", "/SigFlags", "/DR", "/DA"); msg != "" {
		return msg
	}
	if msg := checkRefSuperset(oldDict["/Fields"], newDict["/Fields"], lookupOld, lookupNew); msg != "" {
		return "/Fields: " + msg
	}
	return ""
}

// changedKeysOutside devuelve una descripción si alguna clave distinta de
// las permitidas cambia, aparece o desaparece.
func changedKeysOutside(oldDict, newDict map[string]string, allowed ...string) string {
	permitted := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		permitted[key] = struct{}{}
	}
	keys := make(map[string]struct{}, len(oldDict)+len(newDict))
	for key := range oldDict {
		keys[key] = struct{}{}
	}
	for key := range newDict {
		keys[key] = struct{}{}
	}
	var changed []string
	for key := range keys {
		if _, ok := permitted[key]; ok {
			continue
		}
		if !pdfValuesEquivalent(oldDict[key], newDict[key]) {
			changed = append(changed, key)
		}
	}
	if len(changed) == 0 {
		return ""
	}
	sort.Strings(changed)
	return "cambia " + strings.Join(changed, ", ")
}

// pdfValuesEquivalent compara valores canónicos token a token, admitiendo
// diferencias numéricas de redondeo al reserializar (p. ej. 595.303937007874
// frente a 595.303937), que no cambian lo que se muestra.
func pdfValuesEquivalent(a, b string) bool {
	if a == b {
		return true
	}
	left, right := strings.Fields(a), strings.Fields(b)
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] == right[i] {
			continue
		}
		x, errX := strconv.ParseFloat(left[i], 64)
		y, errY := strconv.ParseFloat(right[i], 64)
		if errX != nil || errY != nil || math.Abs(x-y) > 0.01 {
			return false
		}
	}
	return true
}

// parsePDFObjectDict analiza el diccionario de nivel superior de un objeto.
func parsePDFObjectDict(body string) (map[string]string, bool) {
	trimmed := strings.TrimLeft(body, "\x00\t\n\f\r ")
	if !strings.HasPrefix(trimmed, "<<") {
		return nil, false
	}
	return parsePDFDictValue(trimmed)
}

// parsePDFDictValue convierte un valor "<< ... >>" en un mapa clave → valor
// canónico (tokens separados por un espacio), independiente del formato con
// que cada herramienta serialice el PDF.
func parsePDFDictValue(value string) (map[string]string, bool) {
	tokens, _, ok := pdfTokenizeValue(value, 0)
	if !ok || len(tokens) < 2 || tokens[0] != "<<" || tokens[len(tokens)-1] != ">>" {
		return nil, false
	}
	out := map[string]string{}
	i := 1
	for i < len(tokens)-1 {
		key := tokens[i]
		if !strings.HasPrefix(key, "/") {
			return nil, false
		}
		i++
		start := i
		end, ok := pdfValueEnd(tokens, i, len(tokens)-1)
		if !ok {
			return nil, false
		}
		if _, repeated := out[key]; repeated {
			return nil, false
		}
		out[key] = strings.Join(tokens[start:end], " ")
		i = end
	}
	return out, true
}

// pdfValueEnd devuelve el índice siguiente al valor que empieza en i.
func pdfValueEnd(tokens []string, i, limit int) (int, bool) {
	if i >= limit {
		return 0, false
	}
	switch tokens[i] {
	case "<<", "[":
		depth := 0
		for j := i; j < limit; j++ {
			switch tokens[j] {
			case "<<", "[":
				depth++
			case ">>", "]":
				depth--
				if depth == 0 {
					return j + 1, true
				}
			}
		}
		return 0, false
	}
	// Referencia indirecta "N G R".
	if i+2 < limit && isPDFInteger(tokens[i]) && isPDFInteger(tokens[i+1]) && tokens[i+2] == "R" {
		return i + 3, true
	}
	return i + 1, true
}

// pdfTokenizeValue tokeniza un único valor PDF (típicamente un diccionario)
// desde start y devuelve los tokens y la posición final.
func pdfTokenizeValue(text string, start int) ([]string, int, bool) {
	var tokens []string
	depth := 0
	i := start
	for i < len(text) {
		c := text[i]
		switch {
		case isPDFWhitespace(c):
			i++
			continue
		case c == '%':
			for i < len(text) && text[i] != '\n' && text[i] != '\r' {
				i++
			}
			continue
		case c == '<' && i+1 < len(text) && text[i+1] == '<':
			tokens = append(tokens, "<<")
			depth++
			i += 2
		case c == '>' && i+1 < len(text) && text[i+1] == '>':
			tokens = append(tokens, ">>")
			depth--
			i += 2
		case c == '[':
			tokens = append(tokens, "[")
			depth++
			i++
		case c == ']':
			tokens = append(tokens, "]")
			depth--
			i++
		case c == '(':
			j, ok := skipPDFLiteralString(text, i)
			if !ok {
				return nil, 0, false
			}
			tokens = append(tokens, text[i:j])
			i = j
		case c == '<':
			j := strings.IndexByte(text[i:], '>')
			if j < 0 {
				return nil, 0, false
			}
			tokens = append(tokens, strings.Join(strings.Fields(text[i:i+j+1]), ""))
			i += j + 1
		default:
			j := i + 1
			if c == '/' {
				for j < len(text) && !isPDFWhitespace(text[j]) && !isPDFDelimiter(text[j]) {
					j++
				}
			} else {
				for j < len(text) && !isPDFWhitespace(text[j]) && !isPDFDelimiter(text[j]) {
					j++
				}
			}
			word := text[i:j]
			if depth == 0 && (word == "stream" || word == "endobj") {
				return tokens, i, len(tokens) > 0
			}
			tokens = append(tokens, word)
			i = j
		}
		if depth < 0 {
			return nil, 0, false
		}
		if depth == 0 && len(tokens) > 0 {
			return tokens, i, true
		}
		if len(tokens) > maxPDFDictionaryTokens {
			return nil, 0, false
		}
	}
	return tokens, i, depth == 0 && len(tokens) > 0
}

func skipPDFLiteralString(text string, start int) (int, bool) {
	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '\\':
			i++
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1, true
			}
		}
	}
	return 0, false
}

func isPDFWhitespace(c byte) bool {
	return c == 0 || c == '\t' || c == '\n' || c == '\f' || c == '\r' || c == ' '
}

func isPDFDelimiter(c byte) bool {
	return strings.IndexByte("()<>[]{}/%", c) >= 0
}

func isPDFInteger(token string) bool {
	_, err := strconv.Atoi(token)
	return err == nil
}

func pdfName(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "/") && !strings.Contains(value, " ") {
		return value
	}
	return ""
}

func pdfSingleRef(value string) (int, bool) {
	fields := strings.Fields(value)
	if len(fields) != 3 || fields[2] != "R" {
		return 0, false
	}
	n, err := strconv.Atoi(fields[0])
	return n, err == nil
}

// resolvePDFRefArray admite el array directo o una referencia a un objeto
// array.
func resolvePDFRefArray(value string, lookup pdfLookup) ([]int, bool) {
	if ref, ok := pdfSingleRef(value); ok && lookup != nil {
		body, found := lookup(ref)
		if !found {
			return nil, false
		}
		tokens, _, ok := pdfTokenizeValue(body, 0)
		if !ok {
			return nil, false
		}
		return pdfRefArray(strings.Join(tokens, " "))
	}
	return pdfRefArray(value)
}

// pdfRefArray interpreta "[ a 0 R b 0 R ]" (o vacío) como lista de objetos.
func pdfRefArray(value string) ([]int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, true
	}
	fields, _, ok := pdfTokenizeValue(value, 0)
	if !ok {
		return nil, false
	}
	if len(fields) < 2 || fields[0] != "[" || fields[len(fields)-1] != "]" {
		return nil, false
	}
	inner := fields[1 : len(fields)-1]
	if len(inner)%3 != 0 {
		return nil, false
	}
	out := make([]int, 0, len(inner)/3)
	for i := 0; i < len(inner); i += 3 {
		n, err := strconv.Atoi(inner[i])
		if err != nil || !isPDFInteger(inner[i+1]) || inner[i+2] != "R" {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

func normalizePDFBody(body string) string {
	return strings.Join(strings.Fields(body), " ")
}
