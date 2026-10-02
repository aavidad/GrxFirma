// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"errors"
	"math"
	"regexp"
	"strconv"

	pdf "github.com/digitorus/pdf"
)

var docMDPLevel = regexp.MustCompile(`/TransformMethod\s*/DocMDP[\s\S]{0,1000}?/P\s+([123])\b`)

func clasificarActualizacionesPAdESV2(data []byte, out *InspeccionPDFV2) error {
	if len(out.Firmas) == 0 {
		return errors.New("sin_firmas_pdf")
	}
	signed := make(map[int]int, len(out.Firmas))
	for i := range out.Firmas {
		signed[out.Firmas[i].RevisionLongitud] = i
	}
	lastSignedRevision := -1
	pending := CambiosPDFV2{}
	var certLevel *int
	fieldLock := ""
	for revision := range out.Revisiones {
		currentSignature, hasSignature := signed[out.Revisiones[revision].Length]
		if revision == 0 {
			if hasSignature {
				out.Firmas[currentSignature].CambiosDesdeAnterior = CambiosPDFV2{Estado: "ninguno", Detalle: []string{}}
				lastSignedRevision = revision
				applySignaturePolicy(data, out.Revisiones[revision], &out.Firmas[currentSignature], &certLevel, &fieldLock)
				if out.Firmas[currentSignature].TipoFirma == "certificacion" && !catalogoDocMDPValido(data[:out.Revisiones[revision].Length], out.Firmas[currentSignature].objetoFirma) {
					out.Firmas[currentSignature].CambiosDesdeAnterior = CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"docmdp_sin_referencia_catalogo"}}
				}
			}
			continue
		}
		var sig *FirmaPDFV2
		priorLevel := certLevel
		priorFieldLock := fieldLock
		if hasSignature {
			sig = &out.Firmas[currentSignature]
			applySignaturePolicy(data, out.Revisiones[revision], sig, &certLevel, &fieldLock)
		}
		change := clasificarUnaActualizacion(data, out.Revisiones[revision-1], out.Revisiones[revision], sig)
		if sig != nil {
			change = combinarCambios(pending, change)
			pending = CambiosPDFV2{}
			if priorLevel != nil && *priorLevel == 1 && change.Estado == "permitidos" {
				change = CambiosPDFV2{Estado: "no_permitidos", Detalle: []string{"docmdp_nivel_1_impide_firma_posterior"}}
			}
			if priorFieldLock != "" && change.Estado == "permitidos" {
				if priorFieldLock == "All" {
					change = CambiosPDFV2{Estado: "no_permitidos", Detalle: []string{"fieldmdp_bloquea_campo"}}
				} else {
					change = CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"fieldmdp_no_evaluado"}}
				}
			}
			out.Firmas[currentSignature].CambiosDesdeAnterior = change
			lastSignedRevision = revision
			if sig.TipoFirma == "certificacion" {
				switch {
				case currentSignature > 0:
					out.Firmas[currentSignature].CambiosDesdeAnterior = CambiosPDFV2{Estado: "no_permitidos", Detalle: []string{"docmdp_no_es_primera_firma"}}
				case !catalogoDocMDPValido(data[:out.Revisiones[revision].Length], sig.objetoFirma):
					out.Firmas[currentSignature].CambiosDesdeAnterior = CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"docmdp_sin_referencia_catalogo"}}
				}
			}
		} else {
			pending = combinarCambios(pending, change)
		}
	}
	out.CambiosPosteriores = pending
	if out.CambiosPosteriores.Estado == "" {
		out.CambiosPosteriores = CambiosPDFV2{Estado: "ninguno", Detalle: []string{}}
	}
	if out.BytesDespuesUltimaRevision != 0 || lastSignedRevision < len(out.Revisiones)-1 && out.CambiosPosteriores.Estado == "ninguno" {
		out.CambiosPosteriores = combinarCambios(out.CambiosPosteriores, CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"bytes_posteriores_sin_firma"}, BytesNoFirmados: len(data) - out.Firmas[len(out.Firmas)-1].RevisionLongitud})
	} else {
		out.CambiosPosteriores.BytesNoFirmados = len(data) - out.Firmas[len(out.Firmas)-1].RevisionLongitud
	}
	return nil
}

func applySignaturePolicy(data []byte, snapshot pdf.XRefSnapshot, sig *FirmaPDFV2, certLevel **int, fieldLock *string) {
	body := cuerpoActivoPDF(data, snapshot, sig.objetoFirma)
	if match := docMDPLevel.FindStringSubmatch(body); len(match) == 2 {
		level, _ := strconv.Atoi(match[1])
		sig.TipoFirma = "certificacion"
		sig.NivelDocMDP = &level
		*certLevel = &level
	}
	if bytes.Contains([]byte(body), []byte("/TransformMethod /FieldMDP")) {
		switch {
		case bytes.Contains([]byte(body), []byte("/Action /All")):
			*fieldLock = "All"
		case bytes.Contains([]byte(body), []byte("/Action /Include")):
			*fieldLock = "Include"
		case bytes.Contains([]byte(body), []byte("/Action /Exclude")):
			*fieldLock = "Exclude"
		default:
			*fieldLock = "Unknown"
		}
	}
}

func catalogoDocMDPValido(data []byte, signatureObject uint32) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	perms := r.Trailer().Key("Root").Key("Perms")
	keys := perms.Keys()
	if len(keys) != 1 || keys[0] != "DocMDP" {
		return false
	}
	ref, _ := perms.Key("DocMDP").ObjectReference()
	return ref == signatureObject
}

func combinarCambios(a, b CambiosPDFV2) CambiosPDFV2 {
	rank := map[string]int{"": 0, "ninguno": 1, "permitidos": 2, "no_comprobados": 3, "no_permitidos": 4}
	if rank[b.Estado] > rank[a.Estado] {
		a.Estado = b.Estado
	}
	a.Detalle = append(a.Detalle, b.Detalle...)
	if b.BytesNoFirmados > a.BytesNoFirmados {
		a.BytesNoFirmados = b.BytesNoFirmados
	}
	return a
}

func clasificarUnaActualizacion(data []byte, before, after pdf.XRefSnapshot, signature *FirmaPDFV2) CambiosPDFV2 {
	if before.EncryptObject != after.EncryptObject || before.EncryptGeneration != after.EncryptGeneration {
		return CambiosPDFV2{Estado: "no_permitidos", Detalle: []string{"cifrado_pdf_modificado"}}
	}
	if before.DocumentID != after.DocumentID {
		return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"identificador_pdf_modificado"}}
	}
	if before.InfoValue != after.InfoValue || before.InfoObject != after.InfoObject || before.InfoGeneration != after.InfoGeneration {
		return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"metadatos_pdf_modificados"}}
	}
	if before.RootObject == 0 || after.RootObject == 0 {
		return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"catalogo_no_resuelto"}}
	}
	changed := pdf.ChangedXRefObjects(before, after)
	if len(changed) == 0 {
		if before.RootObject != after.RootObject || before.RootGeneration != after.RootGeneration {
			return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"catalogo_cambiado_sin_xref"}}
		}
		return CambiosPDFV2{Estado: "ninguno", Detalle: []string{}}
	}
	if signature == nil {
		return clasificarCambioNoFirmado(data, before, after, changed)
	}
	var sigObject, widgetObject uint32
	for _, number := range changed {
		entry := after.Entries[number]
		if !entry.Active || entry.InStream {
			return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"xref_liberado_o_comprimido"}}
		}
		if old, exists := before.Entries[number]; exists && old.Active {
			return clasificarCambioNoFirmado(data, before, after, changed)
		}
		body := cuerpoActivoPDF(data, after, number)
		if body == "" {
			return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"objeto_activo_no_resuelto"}}
		}
		dict, ok := parsePDFObjectDict(body)
		if !ok {
			return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"objeto_nuevo_no_clasificado"}}
		}
		switch {
		case number == after.RootObject:
			// Checked below against the signed catalog.
		case pdfName(dict["/Type"]) == "/Sig":
			br, err := extractPDFByteRange([]byte(body))
			if err != nil || br != signature.ByteRange || sigObject != 0 {
				return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"firma_nueva_no_corresponde"}}
			}
			sigObject = number
		case pdfName(dict["/Subtype"]) == "/Widget" && pdfName(dict["/FT"]) == "/Sig":
			if widgetObject != 0 {
				return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"varios_widgets_nuevos"}}
			}
			widgetObject = number
		case pdfName(dict["/Type"]) == "/XRef":
			// A stream xref is structural, not a document change.
		default:
			return clasificarCambioNoFirmado(data, before, after, changed)
		}
	}
	if sigObject == 0 || widgetObject == 0 || after.RootObject == before.RootObject {
		return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"actualizacion_firma_incompleta"}}
	}
	oldCatalog, okOld := parsePDFObjectDict(cuerpoActivoPDF(data, before, before.RootObject))
	newCatalog, okNew := parsePDFObjectDict(cuerpoActivoPDF(data, after, after.RootObject))
	if !okOld || !okNew {
		return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"catalogo_no_analizable"}}
	}
	// The PAdES writer raises a PDF 1.4 catalog to 1.5 when adding its
	// signature. This name alone does not change the already signed page graph.
	if oldCatalog["/Version"] == "" && newCatalog["/Version"] == "/1.5" && bytes.HasPrefix(data[:before.Length], []byte("%PDF-1.4")) {
		delete(newCatalog, "/Version")
	}
	for _, sensitive := range []string{"/DSS", "/Extensions", "/Metadata"} {
		if oldCatalog[sensitive] != newCatalog[sensitive] {
			return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"catalogo_con_cambio_adicional"}}
		}
	}
	if oldCatalog["/Perms"] != newCatalog["/Perms"] && signature.TipoFirma != "certificacion" {
		return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"permisos_pdf_modificados"}}
	}
	if _, newRef := pdfSingleRef(newCatalog["/AcroForm"]); newRef && oldCatalog["/AcroForm"] != newCatalog["/AcroForm"] {
		return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"formulario_indirecto_no_equivalente"}}
	}
	lookupOld := func(number int) (string, bool) {
		n, ok := numeroObjetoPDF(number)
		if !ok {
			return "", false
		}
		body := cuerpoActivoPDF(data, before, n)
		return body, body != ""
	}
	lookupNew := func(number int) (string, bool) {
		n, ok := numeroObjetoPDF(number)
		if !ok {
			return "", false
		}
		body := cuerpoActivoPDF(data, after, n)
		return body, body != ""
	}
	if changed := checkCatalogUpdate(oldCatalog, newCatalog, lookupOld, lookupNew); changed != "" {
		return CambiosPDFV2{Estado: "no_permitidos", Detalle: []string{"catalogo_modificado"}}
	}
	oldFields, okOld := camposActivosPDF(data[:before.Length])
	newFields, okNew := camposActivosPDF(data[:after.Length])
	if !okOld || !okNew || len(newFields) != len(oldFields)+1 || newFields[len(newFields)-1] != widgetObject {
		return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"campos_firma_no_corresponden"}}
	}
	for i := range oldFields {
		if newFields[i] != oldFields[i] {
			return CambiosPDFV2{Estado: "no_permitidos", Detalle: []string{"campos_previos_modificados"}}
		}
	}
	widget, _ := parsePDFObjectDict(cuerpoActivoPDF(data, after, widgetObject))
	for key := range widget {
		switch key {
		case "/Type", "/Subtype", "/Rect", "/P", "/F", "/NM", "/M", "/FT", "/T", "/TU", "/Contents", "/V", "/AP", "/AS", "/BS", "/Border", "/MK", "/Ff", "/DA", "/DR":
		default:
			return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"widget_con_accion_no_analizada"}}
		}
	}
	if ref, ok := pdfSingleRef(widget["/V"]); !ok || !mismoObjetoPDF(ref, sigObject) {
		return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"widget_no_apunta_a_firma"}}
	}
	if widget["/AP"] != "" {
		return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"apariencia_widget_no_evaluada"}}
	}
	return CambiosPDFV2{Estado: "permitidos", Detalle: []string{"firma_anadida"}}
}

func clasificarCambioNoFirmado(data []byte, before, after pdf.XRefSnapshot, changed []uint32) CambiosPDFV2 {
	if len(changed) == 2 && before.RootObject != after.RootObject {
		oldCatalog, oldOK := parsePDFObjectDict(cuerpoActivoPDF(data, before, before.RootObject))
		newCatalog, newOK := parsePDFObjectDict(cuerpoActivoPDF(data, after, after.RootObject))
		if oldOK && newOK && changedKeysOutside(oldCatalog, newCatalog, "/DSS") == "" {
			for _, number := range changed {
				if number == after.RootObject {
					continue
				}
				body := cuerpoActivoPDF(data, after, number)
				if ref, ok := pdfSingleRef(newCatalog["/DSS"]); ok && mismoObjetoPDF(ref, number) && bytes.Contains([]byte(body), []byte("/Type /DSS")) {
					return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"dss_no_verificado"}}
				}
			}
		}
	}
	if len(changed) == 1 {
		body := cuerpoActivoPDF(data, after, changed[0])
		if bytes.Contains([]byte(body), []byte("/Type /DocTimeStamp")) && bytes.Contains([]byte(body), []byte("/SubFilter /ETSI.RFC3161")) {
			return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"doctimestamp_no_verificado"}}
		}
	}
	pageObjects := objetosDePaginaPDF(data[:before.Length])
	for _, number := range changed {
		if pageObjects[number] {
			oldBody := cuerpoActivoPDF(data, before, number)
			newBody := cuerpoActivoPDF(data, after, number)
			if oldBody != "" && newBody != "" && normalizePDFBody(oldBody) == normalizePDFBody(newBody) {
				return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"objeto_pagina_reescrito_equivalente"}}
			}
			return CambiosPDFV2{Estado: "no_permitidos", Detalle: []string{"contenido_paginas_modificado"}}
		}
	}
	if before.RootObject != after.RootObject || before.RootGeneration != after.RootGeneration {
		oldCatalog, okOld := parsePDFObjectDict(cuerpoActivoPDF(data, before, before.RootObject))
		newCatalog, okNew := parsePDFObjectDict(cuerpoActivoPDF(data, after, after.RootObject))
		if !okOld || !okNew || oldCatalog["/Pages"] != newCatalog["/Pages"] {
			return CambiosPDFV2{Estado: "no_permitidos", Detalle: []string{"contenido_paginas_modificado"}}
		}
	}
	for _, number := range changed {
		body := cuerpoActivoPDF(data, after, number)
		if bytes.Contains([]byte(body), []byte("/Type /Page")) || bytes.Contains([]byte(body), []byte("/Type/Page")) || bytes.Contains([]byte(body), []byte("/Contents")) && number != after.RootObject {
			return CambiosPDFV2{Estado: "no_permitidos", Detalle: []string{"contenido_paginas_modificado"}}
		}
	}
	return CambiosPDFV2{Estado: "no_comprobados", Detalle: []string{"objetos_activos_no_clasificados"}}
}

func objetosDePaginaPDF(data []byte) (objects map[uint32]bool) {
	objects = make(map[uint32]bool)
	defer func() {
		if recover() != nil {
			objects = map[uint32]bool{}
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return objects
	}
	count := r.NumPage()
	if count < 0 || count > 10000 {
		return objects
	}
	pages := r.Trailer().Key("Root").Key("Pages")
	if number, _ := pages.ObjectReference(); number != 0 {
		objects[number] = true
	}
	for i := 1; i <= count; i++ {
		page := r.Page(i).V
		if number, _ := page.ObjectReference(); number != 0 {
			objects[number] = true
		}
		content := page.Key("Contents")
		if number, _ := content.ObjectReference(); number != 0 {
			objects[number] = true
		}
		for j := 0; j < content.Len(); j++ {
			if number, _ := content.Index(j).ObjectReference(); number != 0 {
				objects[number] = true
			}
		}
		resources := page.Key("Resources")
		if number, _ := resources.ObjectReference(); number != 0 {
			objects[number] = true
		}
	}
	return objects
}

func cuerpoActivoPDF(data []byte, snapshot pdf.XRefSnapshot, number uint32) string {
	entry, ok := snapshot.Entries[number]
	if !ok || !entry.Active || entry.InStream || entry.Offset < 0 || entry.Offset >= int64(snapshot.Length) {
		return ""
	}
	start := int(entry.Offset)
	limit := snapshot.Length
	if limit-start > maxPDFObjectStreamBytes {
		limit = start + maxPDFObjectStreamBytes
	}
	end := bytes.Index(data[start:limit], []byte("endobj"))
	if end < 0 {
		return ""
	}
	body := data[start : start+end]
	header := pdfObjectHeader.FindIndex(body)
	if header == nil || header[0] != 0 {
		return ""
	}
	return string(body[header[1]:])
}

func camposActivosPDF(data []byte) (fields []uint32, ok bool) {
	defer func() {
		if recover() != nil {
			fields, ok = nil, false
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, false
	}
	array := r.Trailer().Key("Root").Key("AcroForm").Key("Fields")
	if array.IsNull() {
		return []uint32{}, true
	}
	if array.Kind() != pdf.Array {
		return nil, false
	}
	fields = make([]uint32, 0, array.Len())
	for i := 0; i < array.Len(); i++ {
		number, _ := array.Index(i).ObjectReference()
		if number == 0 {
			return nil, false
		}
		fields = append(fields, number)
	}
	return fields, true
}

// numeroObjetoPDF descarta referencias negativas o fuera del rango de objetos.
func numeroObjetoPDF(number int) (uint32, bool) {
	if number < 0 || int64(number) > math.MaxUint32 {
		return 0, false
	}
	return uint32(number), true
}

func mismoObjetoPDF(ref int, object uint32) bool {
	n, ok := numeroObjetoPDF(ref)
	return ok && n == object
}
