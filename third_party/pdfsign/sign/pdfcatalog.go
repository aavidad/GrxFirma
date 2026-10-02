package sign

import (
	"bytes"
	"fmt"
	"io"
	"strconv"

	"github.com/digitorus/pdf"
)

func (context *SignContext) createCatalog() ([]byte, error) {
	var catalog_buffer bytes.Buffer

	// Start the catalog object
	catalog_buffer.WriteString("<<\n")
	catalog_buffer.WriteString("  /Type /Catalog\n")

	// (Optional; PDF 1.4) The version of the PDF specification to which
	// the document conforms (for example, 1.4) if later than the version
	// specified in the file’s header (see 7.5.2, "File header"). If the header
	// specifies a later version, or if this entry is absent, the document
	// shall conform to the version specified in the header. This entry
	// enables a PDF processor to update the version using an incremental
	// update; see 7.5.6, "Incremental updates".
	// The value of this entry shall be a name object, not a number, and
	// therefore shall be preceded by a SOLIDUS (2Fh) character (/) when
	// written in the PDF file (for example, /1.4).
	//
	// If an incremental upgrade requires a version that is higher than specified by the document.
	// Ensure PDF version is at least 1.5 to support SigFlags in acroFormDict (1.4) and UF in the fileSpecDict (1.5)
	versionEscrita := false
	if v, err := strconv.ParseFloat(context.PDFReader.PDFVersion, 64); err == nil && v < 1.5 {
		catalog_buffer.WriteString("  /Version /1.5\n")
		versionEscrita = true
	}

	// Retrieve the root, its pointer and set the root string
	root := context.PDFReader.Trailer().Key("Root")
	if context.SignData.Signature.CertType == CertificationSignature {
		if len(context.existingSignatures) != 0 || !root.Key("Perms").IsNull() {
			return nil, fmt.Errorf("una firma de certificación debe ser la primera")
		}
	}
	rootPtr := root.GetPtr()
	context.CatalogData.RootString = strconv.Itoa(int(rootPtr.GetID())) + " " + strconv.Itoa(int(rootPtr.GetGen())) + " R"

	// Copy over existing catalog entries except for type and AcroForum
	// (y DSS si esta firma escribe uno nuevo que ya combina el anterior).
	for _, key := range root.Keys() {
		if key == "Type" || key == "AcroForm" {
			continue
		}
		// Una firma anterior ya pudo subir /Version: no se duplica la clave (qpdf lo
		// marca como diccionario con claves repetidas y algunos lectores lo rechazan).
		if key == "Version" && versionEscrita {
			continue
		}
		if key == "DSS" && context.dssObjectID != 0 {
			continue
		}
		_, _ = fmt.Fprintf(&catalog_buffer, "  /%s ", key)
		context.serializeCatalogEntry(&catalog_buffer, rootPtr.GetID(), root.Key(key))
		catalog_buffer.WriteString("\n")
	}

	if context.dssObjectID != 0 {
		_, _ = fmt.Fprintf(&catalog_buffer, "  /DSS %d 0 R\n", context.dssObjectID)
	}
	if context.SignData.Signature.CertType == CertificationSignature {
		_, _ = fmt.Fprintf(&catalog_buffer, "  /Perms << /DocMDP %d 0 R >>\n", context.SignData.objectId)
	}

	// AcroForm: se conservan TODOS los campos y claves del formulario original
	// (texto, casillas, /DR, /DA...). Antes solo se copiaban las firmas y
	// firmar un formulario lo dejaba sin campos.
	acroForm := root.Key("AcroForm")
	acroFormPtr := acroForm.GetPtr()
	acroFormID := acroFormPtr.GetID()
	if acroFormID == 0 {
		acroFormID = rootPtr.GetID()
	}
	catalog_buffer.WriteString("  /AcroForm <<\n")
	catalog_buffer.WriteString("    /Fields [")
	primero := true
	escribir := func(s string) {
		if !primero {
			catalog_buffer.WriteString(" ")
		}
		primero = false
		catalog_buffer.WriteString(s)
	}
	presentes := map[uint32]bool{}
	fields := acroForm.Key("Fields")
	for i := 0; i < fields.Len(); i++ {
		campo := fields.Index(i)
		campoPtr := campo.GetPtr()
		if id := campoPtr.GetID(); id != 0 && id != acroFormID {
			presentes[id] = true
		}
		var b bytes.Buffer
		context.serializeCatalogEntry(&b, acroFormID, campo)
		escribir(b.String())
	}
	for _, sig := range context.existingSignatures {
		if !presentes[sig.objectId] {
			presentes[sig.objectId] = true
			escribir(strconv.Itoa(int(sig.objectId)) + " 0 R")
		}
	}

	// Add the visual signature field(s) to the AcroForm dictionary.
	fieldRefs := make([]uint32, 0, 1)
	switch {
	case len(context.VisualSignData.annotObjectIDs) > 0:
		fieldRefs = append(fieldRefs, context.VisualSignData.annotObjectIDs...)
	case context.VisualSignData.fieldObjectID != 0:
		fieldRefs = append(fieldRefs, context.VisualSignData.fieldObjectID)
	case context.VisualSignData.objectId != 0:
		fieldRefs = append(fieldRefs, context.VisualSignData.objectId)
	}
	for _, ref := range fieldRefs {
		if !presentes[ref] {
			presentes[ref] = true
			escribir(strconv.Itoa(int(ref)) + " 0 R")
		}
	}

	catalog_buffer.WriteString("]\n") // close Fields array

	for _, key := range acroForm.Keys() {
		if key == "Fields" || key == "SigFlags" {
			continue
		}
		_, _ = fmt.Fprintf(&catalog_buffer, "    /%s ", key)
		context.serializeCatalogEntry(&catalog_buffer, acroFormID, acroForm.Key(key))
		catalog_buffer.WriteString("\n")
	}

	// (Optional; deprecated in PDF 2.0) A flag specifying whether
	// to construct appearance streams and appearance
	// dictionaries for all widget annotations in the document (see
	// 12.7.4.3, "Variable text"). Default value: false. A PDF writer
	// shall include this key, with a value of true, if it has not
	// provided appearance streams for all visible widget
	// annotations present in the document.
	// if context.SignData.Visible {
	// 	catalog_buffer.WriteString(" /NeedAppearances true")
	// } else {
	// 	catalog_buffer.WriteString(" /NeedAppearances false")
	// }

	// Signature flags (Table 225)
	//
	// Bit position 1: SignaturesExist
	// If set, the document contains at least one signature field. This
	// flag allows an interactive PDF processor to enable user
	// interface items (such as menu items or push-buttons) related to
	// signature processing without having to scan the entire
	// document for the presence of signature fields.
	//
	// Bit position 2: AppendOnly
	// If set, the document contains signatures that may be invalidated
	// if the PDF file is saved (written) in a way that alters its previous
	// contents, as opposed to an incremental update. Merely updating
	// the PDF file by appending new information to the end of the
	// previous version is safe (see H.7, "Updating example").
	// Interactive PDF processors may use this flag to inform a user
	// requesting a full save that signatures will be invalidated and
	// require explicit confirmation before continuing with the
	// operation.
	//
	// Set SigFlags and Permissions based on Signature Type
	switch context.SignData.Signature.CertType {
	case CertificationSignature, ApprovalSignature, TimeStampSignature:
		catalog_buffer.WriteString("    /SigFlags 3\n")
	case UsageRightsSignature:
		catalog_buffer.WriteString("    /SigFlags 1\n")
	}

	// Finalize the AcroForm and Catalog object
	catalog_buffer.WriteString("  >>\n") // Close AcroForm
	catalog_buffer.WriteString(">>\n")   // Close Catalog

	return catalog_buffer.Bytes(), nil
}

// serializeCatalogEntry takes a pdf.Value and serializes it to the given writer.
func (context *SignContext) serializeCatalogEntry(w io.Writer, rootObjId uint32, value pdf.Value) {
	if ptr := value.GetPtr(); ptr.GetID() != 0 && ptr.GetID() != rootObjId {
		// Indirect object
		_, _ = fmt.Fprintf(w, "%d %d R", ptr.GetID(), ptr.GetGen())
		return
	}
	// ptr.GetID() == 0 significa que el valor es directo (pertenece al objeto
	// raíz) o una referencia que el lector no pudo resolver (p. ej. objetos en
	// un xref híbrido /XRefStm no soportado). En ambos casos hay que serializar
	// el valor directo; emitir "0 0 R" dejaría una referencia indirecta colgante
	// que qpdf reporta como "bad indirect reference (0 0 R)".

	// Direct object
	switch value.Kind() {
	case pdf.String:
		_, _ = fmt.Fprintf(w, "(%s)", value.RawString())
	case pdf.Null:
		_, _ = fmt.Fprint(w, "null")
	case pdf.Bool:
		if value.Bool() {
			_, _ = fmt.Fprint(w, "true")
		} else {
			_, _ = fmt.Fprint(w, "false")
		}
	case pdf.Integer:
		_, _ = fmt.Fprintf(w, "%d", value.Int64())
	case pdf.Real:
		_, _ = fmt.Fprintf(w, "%f", value.Float64())
	case pdf.Name:
		_, _ = fmt.Fprintf(w, "/%s", value.Name())
	case pdf.Dict:
		_, _ = fmt.Fprint(w, "<<")
		for idx, key := range value.Keys() {
			if idx > 0 {
				_, _ = fmt.Fprint(w, " ") // Space between items
			}
			_, _ = fmt.Fprintf(w, "/%s ", key)
			context.serializeCatalogEntry(w, rootObjId, value.Key(key))
		}
		_, _ = fmt.Fprint(w, ">>")
	case pdf.Array:
		_, _ = fmt.Fprint(w, "[")
		for idx := range value.Len() {
			if idx > 0 {
				_, _ = fmt.Fprint(w, " ") // Space between items
			}
			context.serializeCatalogEntry(w, rootObjId, value.Index(idx))
		}
		_, _ = fmt.Fprint(w, "]")
	case pdf.Stream:
		panic("stream cannot be a direct object")
	}
}
