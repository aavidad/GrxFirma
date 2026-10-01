// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Análisis de las actualizaciones incrementales posteriores a una firma PDF.
//
// Una firma solo cubre su revisión. Lo que se añade después puede ser
// legítimo (otras firmas, sellos de tiempo, material de validación LTV) o
// un ataque de "actualización incremental" que redefine objetos ya firmados
// (contenido de páginas, árbol de páginas, recursos, anotaciones) para que el
// visor muestre algo distinto de lo firmado. AutoFirma Java marca estos
// documentos como sospechosos; aquí se clasifica cada objeto redefinido.
//
// Los objetos nuevos no alteran lo que se ve por sí solos: solo pueden
// mostrarse si un objeto ya firmado pasa a referenciarlos, y esas
// redefiniciones son precisamente lo que se controla.

const (
	maxPDFIncrementalObjects   = 200_000
	maxPDFObjectStreamBytes    = 64 << 20
	maxPDFObjectStreamsTotal   = 256 << 20
	pdfIncrementalMaxViolation = 8
)

type pdfUpdateVerdict struct {
	// Violations enumera redefiniciones no permitidas tras la firma.
	Violations []string
	// Unanalyzable indica que la actualización no se pudo inspeccionar y,
	// por tanto, no puede darse por inocua.
	Unanalyzable string
}

type pdfObjectDef struct {
	num    int
	body   string
	offset int
}

var (
	pdfObjectHeader = regexp.MustCompile(`(?:^|[\x00\t\n\f\r ])(\d{1,10})[\x00\t\n\f\r ]+(\d{1,5})[\x00\t\n\f\r ]+obj\b`)
	pdfRootRef      = regexp.MustCompile(`/Root[\x00\t\n\f\r ]*(\d{1,10})[\x00\t\n\f\r ]+\d{1,5}[\x00\t\n\f\r ]+R`)
)

// pdfIncrementalAnalyzer recorre el PDF una sola vez y luego evalúa cada
// revisión firmada, para que un documento con muchas firmas no multiplique
// el coste del análisis.
type pdfIncrementalAnalyzer struct {
	pdf  []byte
	defs []pdfObjectDef
	err  error
}

func newPDFIncrementalAnalyzer(pdf []byte) *pdfIncrementalAnalyzer {
	budget := maxPDFObjectStreamsTotal
	defs, err := scanPDFObjects(pdf, &budget)
	return &pdfIncrementalAnalyzer{pdf: pdf, defs: defs, err: err}
}

func analyzePDFIncrementalUpdate(pdf []byte, revisionEnd int) pdfUpdateVerdict {
	return newPDFIncrementalAnalyzer(pdf).analyze(revisionEnd)
}

func (a *pdfIncrementalAnalyzer) analyze(revisionEnd int) pdfUpdateVerdict {
	pdf := a.pdf
	if revisionEnd <= 0 || revisionEnd >= len(pdf) {
		return pdfUpdateVerdict{}
	}
	if a.err != nil {
		return pdfUpdateVerdict{Unanalyzable: "el PDF no se pudo analizar: " + a.err.Error()}
	}
	signedPart, appended := pdf[:revisionEnd], pdf[revisionEnd:]
	var oldDefs, newDefs []pdfObjectDef
	for _, def := range a.defs {
		if def.offset < revisionEnd {
			oldDefs = append(oldDefs, def)
		} else {
			newDefs = append(newDefs, def)
		}
	}
	if len(newDefs) == 0 {
		// Solo comentarios, espacios o un trailer sin objetos: no cambia
		// nada de lo que se muestra.
		if root := lastRootRef(appended); root >= 0 && root != lastRootRef(signedPart) {
			return pdfUpdateVerdict{Violations: []string{"el catálogo raíz (/Root) apunta a otro objeto tras la firma"}}
		}
		return pdfUpdateVerdict{}
	}

	old := make(map[int]string, len(oldDefs))
	for _, def := range oldDefs {
		old[def.num] = def.body
	}
	appendedNums := make(map[int]string, len(newDefs))
	for _, def := range newDefs {
		appendedNums[def.num] = def.body
	}

	oldRoot := lastRootRef(signedPart)
	newRoot := lastRootRef(appended)
	acroFormObj := -1
	if oldRoot >= 0 {
		if catalog, ok := parsePDFObjectDict(old[oldRoot]); ok {
			if ref, ok := pdfSingleRef(catalog["/AcroForm"]); ok {
				acroFormObj = ref
			}
		}
	}
	// /Annots y /Fields pueden ser referencias a un array independiente, que
	// también se redefine al añadir una firma.
	annotArrays := map[int]struct{}{}
	fieldArrays := map[int]struct{}{}
	for num, body := range old {
		dict, ok := parsePDFObjectDict(body)
		if !ok {
			continue
		}
		if pdfName(dict["/Type"]) == "/Page" {
			if ref, ok := pdfSingleRef(dict["/Annots"]); ok {
				annotArrays[ref] = struct{}{}
			}
		}
		if num == acroFormObj || dict["/Fields"] != "" {
			if ref, ok := pdfSingleRef(dict["/Fields"]); ok {
				fieldArrays[ref] = struct{}{}
			}
		}
	}
	lookup := func(num int) (string, bool) {
		if body, ok := appendedNums[num]; ok {
			return body, true
		}
		body, ok := old[num]
		return body, ok
	}
	lookupOld := func(num int) (string, bool) {
		body, ok := old[num]
		return body, ok
	}

	var verdict pdfUpdateVerdict
	add := func(format string, args ...any) {
		if len(verdict.Violations) < pdfIncrementalMaxViolation {
			verdict.Violations = append(verdict.Violations, fmt.Sprintf(format, args...))
		}
	}
	// Algunas herramientas (AutoFirma Java, pdfsign) escriben un catálogo
	// nuevo al añadir una firma. Se admite si equivale al firmado salvo en
	// las claves permitidas (/AcroForm, /DSS, ...).
	if newRoot >= 0 && newRoot != oldRoot {
		oldCatalog, okOld := parsePDFObjectDict(old[oldRoot])
		newBody, found := lookup(newRoot)
		newCatalog, okNew := parsePDFObjectDict(newBody)
		switch {
		case oldRoot < 0 || !okOld:
			return pdfUpdateVerdict{Unanalyzable: "no se localizó el catálogo de la revisión firmada"}
		case !found || !okNew:
			return pdfUpdateVerdict{Violations: []string{"el nuevo catálogo raíz no es analizable"}}
		default:
			if msg := checkCatalogUpdate(oldCatalog, newCatalog, lookupOld, lookup); msg != "" {
				add("catálogo: %s", msg)
			}
		}
	}
	for _, def := range newDefs {
		previous, existed := old[def.num]
		if !existed || normalizePDFBody(previous) == normalizePDFBody(def.body) {
			continue
		}
		if _, ok := annotArrays[def.num]; ok {
			if msg := checkAnnotsUpdate(previous, def.body, nil, nil, appendedNums); msg != "" {
				add("anotaciones (objeto %d): %s", def.num, msg)
			}
			continue
		}
		if _, ok := fieldArrays[def.num]; ok {
			if msg := checkRefSuperset(previous, def.body, lookupOld, lookup); msg != "" {
				add("campos (objeto %d): %s", def.num, msg)
			}
			continue
		}
		oldDict, okOld := parsePDFObjectDict(previous)
		newDict, okNew := parsePDFObjectDict(def.body)
		if !okOld || !okNew {
			add("objeto %d redefinido tras la firma", def.num)
			continue
		}
		switch {
		case def.num == oldRoot:
			if msg := checkCatalogUpdate(oldDict, newDict, lookupOld, lookup); msg != "" {
				add("catálogo: %s", msg)
			}
		case pdfName(oldDict["/Type"]) == "/Page":
			if msg := checkPageUpdate(oldDict, newDict, lookupOld, lookup, appendedNums); msg != "" {
				add("página (objeto %d): %s", def.num, msg)
			}
		case def.num == acroFormObj || oldDict["/Fields"] != "":
			if msg := checkAcroFormUpdate(oldDict, newDict, lookupOld, lookup); msg != "" {
				add("formulario (objeto %d): %s", def.num, msg)
			}
		case pdfName(oldDict["/FT"]) == "/Sig":
			if msg := changedKeysOutside(oldDict, newDict, "/V", "/AP", "/AS", "/Lock", "/SV"); msg != "" {
				add("campo de firma (objeto %d): %s", def.num, msg)
			}
		case pdfName(oldDict["/Type"]) == "/Metadata",
			pdfName(oldDict["/Type"]) == "/DSS",
			pdfName(oldDict["/Type"]) == "/VRI":
			// Metadatos XMP y material de validación LTV: no alteran el
			// contenido mostrado.
		default:
			tipo := pdfName(oldDict["/Type"])
			if tipo == "" {
				tipo = pdfName(oldDict["/Subtype"])
			}
			if tipo == "" {
				tipo = "contenido"
			}
			add("objeto %d (%s) redefinido tras la firma", def.num, tipo)
		}
	}
	return verdict
}

func lastRootRef(section []byte) int {
	matches := pdfRootRef.FindAllSubmatch(section, -1)
	if len(matches) == 0 {
		return -1
	}
	n, err := strconv.Atoi(string(matches[len(matches)-1][1]))
	if err != nil {
		return -1
	}
	return n
}

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

// scanPDFObjects enumera los objetos indirectos de una sección, incluidos
// los comprimidos en flujos de objetos (/ObjStm).
func scanPDFObjects(section []byte, budget *int) ([]pdfObjectDef, error) {
	var out []pdfObjectDef
	text := string(section)
	pos := 0
	for {
		loc := pdfObjectHeader.FindStringSubmatchIndex(text[pos:])
		if loc == nil {
			break
		}
		num, err := strconv.Atoi(text[pos+loc[2] : pos+loc[3]])
		if err != nil {
			return nil, err
		}
		bodyStart := pos + loc[1]
		body, next, err := readPDFObjectBody(text, bodyStart)
		if err != nil {
			return nil, fmt.Errorf("objeto %d: %w", num, err)
		}
		headerOffset := pos + loc[0]
		out = append(out, pdfObjectDef{num: num, body: body, offset: headerOffset})
		if len(out) > maxPDFIncrementalObjects {
			return nil, errors.New("demasiados objetos")
		}
		if dict, ok := parsePDFObjectDict(body); ok && pdfName(dict["/Type"]) == "/ObjStm" {
			inner, err := expandPDFObjectStream(dict, body, budget)
			if err != nil {
				return nil, fmt.Errorf("flujo de objetos %d: %w", num, err)
			}
			for i := range inner {
				inner[i].offset = headerOffset
			}
			out = append(out, inner...)
		}
		pos = next
	}
	return out, nil
}

// readPDFObjectBody devuelve el cuerpo entre "obj" y "endobj", saltando el
// contenido binario de los flujos para no confundirlo con palabras clave.
func readPDFObjectBody(text string, start int) (string, int, error) {
	dictEnd := start
	if tokens, end, ok := pdfTokenizeValue(text, start); ok && len(tokens) > 0 {
		dictEnd = end
	}
	rest := text[dictEnd:]
	trimmed := strings.TrimLeft(rest, "\x00\t\n\f\r ")
	if strings.HasPrefix(trimmed, "stream") {
		streamStart := dictEnd + (len(rest) - len(trimmed)) + len("stream")
		if strings.HasPrefix(text[streamStart:], "\r\n") {
			streamStart += 2
		} else if strings.HasPrefix(text[streamStart:], "\n") {
			streamStart++
		}
		length := -1
		if dict, ok := parsePDFObjectDict(text[start:dictEnd]); ok {
			if n, err := strconv.Atoi(dict["/Length"]); err == nil && n >= 0 {
				length = n
			}
		}
		endStream := -1
		if length >= 0 && streamStart+length <= len(text) {
			after := strings.TrimLeft(text[streamStart+length:], "\x00\t\n\f\r ")
			if strings.HasPrefix(after, "endstream") {
				endStream = len(text) - len(after)
			}
		}
		if endStream < 0 {
			idx := strings.Index(text[streamStart:], "endstream")
			if idx < 0 {
				return "", 0, errors.New("flujo sin endstream")
			}
			endStream = streamStart + idx
		}
		idx := strings.Index(text[endStream:], "endobj")
		if idx < 0 {
			return "", 0, errors.New("objeto sin endobj")
		}
		return text[start : endStream+idx], endStream + idx + len("endobj"), nil
	}
	idx := strings.Index(text[dictEnd:], "endobj")
	if idx < 0 {
		return "", 0, errors.New("objeto sin endobj")
	}
	return text[start : dictEnd+idx], dictEnd + idx + len("endobj"), nil
}

func expandPDFObjectStream(dict map[string]string, body string, budget *int) ([]pdfObjectDef, error) {
	filter := strings.Trim(dict["/Filter"], "[] ")
	if filter != "" && filter != "/FlateDecode" {
		return nil, fmt.Errorf("filtro %s no admitido", filter)
	}
	if strings.TrimSpace(dict["/DecodeParms"]) != "" {
		return nil, errors.New("parámetros de decodificación no admitidos")
	}
	n, errN := strconv.Atoi(dict["/N"])
	first, errFirst := strconv.Atoi(dict["/First"])
	if errN != nil || errFirst != nil || n < 0 || n > maxPDFIncrementalObjects || first < 0 {
		return nil, errors.New("cabecera /N o /First inválida")
	}
	idx := strings.Index(body, "stream")
	if idx < 0 {
		return nil, errors.New("sin datos")
	}
	raw := body[idx+len("stream"):]
	raw = strings.TrimPrefix(strings.TrimPrefix(raw, "\r"), "\n")
	data := []byte(raw)
	if filter == "/FlateDecode" {
		reader, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		limit := maxPDFObjectStreamBytes
		if *budget < limit {
			limit = *budget
		}
		decoded, err := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
		_ = reader.Close()
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}
		if len(decoded) > limit {
			return nil, errors.New("flujo de objetos demasiado grande")
		}
		*budget -= len(decoded)
		data = decoded
	}
	if first > len(data) {
		return nil, errors.New("/First fuera de rango")
	}
	header := strings.Fields(string(data[:first]))
	if len(header) < 2*n {
		return nil, errors.New("cabecera de objetos incompleta")
	}
	type entry struct{ num, offset int }
	entries := make([]entry, n)
	for i := 0; i < n; i++ {
		num, err1 := strconv.Atoi(header[2*i])
		offset, err2 := strconv.Atoi(header[2*i+1])
		if err1 != nil || err2 != nil || offset < 0 || first+offset > len(data) {
			return nil, errors.New("entrada de objeto inválida")
		}
		entries[i] = entry{num, first + offset}
	}
	out := make([]pdfObjectDef, 0, n)
	for i, e := range entries {
		end := len(data)
		if i+1 < n && entries[i+1].offset >= e.offset {
			end = entries[i+1].offset
		}
		out = append(out, pdfObjectDef{num: e.num, body: string(data[e.offset:end])})
	}
	return out, nil
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
