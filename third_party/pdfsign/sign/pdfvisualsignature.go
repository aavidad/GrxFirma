package sign

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/digitorus/pdf"
)

// Define annotation flag constants.
const (
	AnnotationFlagInvisible      = 1 << 0
	AnnotationFlagHidden         = 1 << 1
	AnnotationFlagPrint          = 1 << 2
	AnnotationFlagNoZoom         = 1 << 3
	AnnotationFlagNoRotate       = 1 << 4
	AnnotationFlagNoView         = 1 << 5
	AnnotationFlagReadOnly       = 1 << 6
	AnnotationFlagLocked         = 1 << 7
	AnnotationFlagToggleNoView   = 1 << 8
	AnnotationFlagLockedContents = 1 << 9
)

// createVisualSignature creates a visual signature field in a PDF document.
// visible: determines if the signature field should be visible or not.
// pageNumber: the page number where the signature should be placed.
// rect: the rectangle defining the position and size of the signature field.
// Returns the visual signature string, the page object id and an error if any.
func (context *SignContext) createVisualSignature(visible bool, pageNumber uint32, rect [4]float64, fieldIndex int) ([]byte, uint32, error) {
	var visual_signature bytes.Buffer
	fieldName := "Signature " + strconv.Itoa(fieldIndex)
	annotationName := "SignatureWidget " + strconv.Itoa(fieldIndex)
	description := context.visualSignatureDescription()

	visual_signature.WriteString("<<\n")

	// Define the object as an annotation.
	visual_signature.WriteString("  /Type /Annot\n")
	// Specify the annotation subtype as a widget.
	visual_signature.WriteString("  /Subtype /Widget\n")

	if visible {
		// Set the position and size of the signature field if visible.
		visual_signature.WriteString(fmt.Sprintf("  /Rect [%f %f %f %f]\n", rect[0], rect[1], rect[2], rect[3]))

		appearance, err := context.createAppearance(rect)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create appearance: %w", err)
		}

		appearanceObjectId, err := context.addObject(appearance)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to add appearance object: %w", err)
		}

		// An appearance dictionary specifying how the annotation
		// shall be presented visually on the page (see 12.5.5, "Appearance streams").
		visual_signature.WriteString(fmt.Sprintf("  /AP << /N %d 0 R >>\n", appearanceObjectId))

	} else {
		// Set the rectangle to zero if the signature is invisible.
		visual_signature.WriteString("  /Rect [0 0 0 0]\n")
	}

	// Retrieve the root object from the PDF trailer.
	root := context.PDFReader.Trailer().Key("Root")
	// Get all keys from the root object.
	root_keys := root.Keys()
	found_pages := false
	for _, key := range root_keys {
		if key == "Pages" {
			// Check if the root object contains the "Pages" key.
			found_pages = true
			break
		}
	}

	// Get the pointer to the root object.
	rootPtr := root.GetPtr()
	// Store the root object reference in the catalog data.
	context.CatalogData.RootString = strconv.Itoa(int(rootPtr.GetID())) + " " + strconv.Itoa(int(rootPtr.GetGen())) + " R"

	if found_pages {
		// Find the page object by its number.
		page, err := findPageByNumber(root.Key("Pages"), pageNumber)
		if err != nil {
			return nil, 0, err
		}

		// Get the pointer to the page object.
		page_ptr := page.GetPtr()

		// Store the page ID in the visual signature context so that we can add it to xref table later.
		context.VisualSignData.pageObjectId = page_ptr.GetID()

		// Add the page reference to the visual signature.
		visual_signature.WriteString("  /P " + strconv.Itoa(int(page_ptr.GetID())) + " " + strconv.Itoa(int(page_ptr.GetGen())) + " R\n")
	}

	// Define the annotation flags for the signature field (132)
	annotationFlags := AnnotationFlagPrint | AnnotationFlagLocked
	visual_signature.WriteString(fmt.Sprintf("  /F %d\n", annotationFlags))
	visual_signature.WriteString(fmt.Sprintf("  /NM %s\n", pdfString(annotationName)))
	if !context.SignData.Signature.Info.Date.IsZero() {
		visual_signature.WriteString(fmt.Sprintf("  /M %s\n", pdfDateTime(context.SignData.Signature.Info.Date)))
	}

	// Define the field type as a signature.
	visual_signature.WriteString("  /FT /Sig\n")
	// Set a unique title for the signature field.
	visual_signature.WriteString(fmt.Sprintf("  /T %s\n", pdfString(fieldName)))
	visual_signature.WriteString(fmt.Sprintf("  /TU %s\n", pdfString(description)))
	visual_signature.WriteString(fmt.Sprintf("  /Contents %s\n", pdfString(description)))

	// Reference the signature dictionary.
	visual_signature.WriteString(fmt.Sprintf("  /V %d 0 R\n", context.SignData.objectId))

	// Close the dictionary and end the object.
	visual_signature.WriteString(">>\n")

	return visual_signature.Bytes(), context.VisualSignData.pageObjectId, nil
}

func (context *SignContext) createVisualSignatureField(fieldName string, widgetObjectIDs []uint32) ([]byte, error) {
	var field bytes.Buffer
	description := context.visualSignatureDescription()

	field.WriteString("<<\n")
	field.WriteString("  /FT /Sig\n")
	field.WriteString(fmt.Sprintf("  /T %s\n", pdfString(fieldName)))
	field.WriteString(fmt.Sprintf("  /TU %s\n", pdfString(description)))
	field.WriteString(fmt.Sprintf("  /V %d 0 R\n", context.SignData.objectId))
	field.WriteString("  /Kids [")
	for i, widgetID := range widgetObjectIDs {
		if i > 0 {
			field.WriteString(" ")
		}
		field.WriteString(fmt.Sprintf("%d 0 R", widgetID))
	}
	field.WriteString("]\n")
	field.WriteString(">>\n")

	return field.Bytes(), nil
}

func (context *SignContext) createVisualSignatureWidget(pageNumber uint32, rect [4]float64, appearanceObjectID, fieldObjectID uint32) ([]byte, uint32, error) {
	var widget bytes.Buffer
	description := context.visualSignatureDescription()

	root := context.PDFReader.Trailer().Key("Root")
	rootPtr := root.GetPtr()
	context.CatalogData.RootString = strconv.Itoa(int(rootPtr.GetID())) + " " + strconv.Itoa(int(rootPtr.GetGen())) + " R"

	page, err := findPageByNumber(root.Key("Pages"), pageNumber)
	if err != nil {
		return nil, 0, err
	}
	pagePtr := page.GetPtr()

	widget.WriteString("<<\n")
	widget.WriteString("  /Type /Annot\n")
	widget.WriteString("  /Subtype /Widget\n")
	widget.WriteString(fmt.Sprintf("  /Rect [%f %f %f %f]\n", rect[0], rect[1], rect[2], rect[3]))
	widget.WriteString(fmt.Sprintf("  /AP << /N %d 0 R >>\n", appearanceObjectID))
	widget.WriteString(fmt.Sprintf("  /P %d %d R\n", pagePtr.GetID(), pagePtr.GetGen()))
	annotationFlags := AnnotationFlagPrint | AnnotationFlagLocked
	widget.WriteString(fmt.Sprintf("  /F %d\n", annotationFlags))
	widget.WriteString(fmt.Sprintf("  /NM %s\n", pdfString(fmt.Sprintf("SignatureWidget %d", fieldObjectID))))
	if !context.SignData.Signature.Info.Date.IsZero() {
		widget.WriteString(fmt.Sprintf("  /M %s\n", pdfDateTime(context.SignData.Signature.Info.Date)))
	}
	widget.WriteString(fmt.Sprintf("  /Contents %s\n", pdfString(description)))
	widget.WriteString(fmt.Sprintf("  /Parent %d 0 R\n", fieldObjectID))
	widget.WriteString(">>\n")

	return widget.Bytes(), pagePtr.GetID(), nil
}

func (context *SignContext) visualSignatureDescription() string {
	if description := strings.TrimSpace(context.SignData.Signature.Info.Description); description != "" {
		return description
	}
	parts := []string{"Firma digital"}
	if name := strings.TrimSpace(context.SignData.Signature.Info.Name); name != "" {
		parts = append(parts, "Firmante: "+name)
	}
	if reason := strings.TrimSpace(context.SignData.Signature.Info.Reason); reason != "" {
		parts = append(parts, "Motivo: "+reason)
	}
	if location := strings.TrimSpace(context.SignData.Signature.Info.Location); location != "" {
		parts = append(parts, "Ubicación: "+location)
	}
	if !context.SignData.Signature.Info.Date.IsZero() {
		parts = append(parts, "Fecha: "+context.SignData.Signature.Info.Date.Format("2006-01-02 15:04 -07:00"))
	}
	return strings.Join(parts, " | ")
}

func (context *SignContext) resolveAppearancePages() ([]uint32, error) {
	if len(context.SignData.Appearance.PerPage) > 0 {
		pages := make([]uint32, len(context.SignData.Appearance.PerPage))
		for i, placement := range context.SignData.Appearance.PerPage {
			pages[i] = placement.Page
		}
		return pages, nil
	}
	totalPages := context.PDFReader.NumPage()
	if totalPages <= 0 {
		totalPages = 1
	}

	if context.SignData.Appearance.AllPages {
		pages := make([]uint32, totalPages)
		for i := 0; i < totalPages; i++ {
			pages[i] = uint32(i + 1)
		}
		return pages, nil
	}

	if len(context.SignData.Appearance.Pages) > 0 {
		seen := make(map[uint32]struct{}, len(context.SignData.Appearance.Pages))
		pages := make([]uint32, 0, len(context.SignData.Appearance.Pages))
		for _, page := range context.SignData.Appearance.Pages {
			if page == 0 || int(page) > totalPages {
				return nil, fmt.Errorf("page number %d not found", page)
			}
			if _, ok := seen[page]; ok {
				continue
			}
			seen[page] = struct{}{}
			pages = append(pages, page)
		}
		if len(pages) > 0 {
			return pages, nil
		}
	}

	page := context.SignData.Appearance.Page
	if page == 0 {
		page = 1
	}
	if int(page) > totalPages {
		return nil, fmt.Errorf("page number %d not found", page)
	}
	return []uint32{page}, nil
}

func (context *SignContext) createIncPageUpdate(pageNumber, annot uint32) ([]byte, error) {
	var page_buffer bytes.Buffer

	// Retrieve the root object from the PDF trailer.
	root := context.PDFReader.Trailer().Key("Root")
	page, err := findPageByNumber(root.Key("Pages"), pageNumber)
	if err != nil {
		return nil, err
	}
	pageIncPtr := page.GetPtr()
	pageObjectID := pageIncPtr.GetID()

	// Anotaciones nuevas de esta página: sellos de imagen pendientes y, si
	// la hay, la de la firma. Se escriben en una única actualización.
	nuevas := append([]uint32(nil), context.VisualSignData.annotsPendientes[pageNumber]...)
	delete(context.VisualSignData.annotsPendientes, pageNumber)
	if annot != 0 {
		nuevas = append(nuevas, annot)
	}

	page_buffer.WriteString("<<\n")

	// TODO: Update digitorus/pdf to get raw values without resolving pointers
	for _, key := range page.Keys() {
		switch key {
		case "Parent":
			ptr := page.Key(key).GetPtr()
			page_buffer.WriteString(fmt.Sprintf("  /%s %d 0 R\n", key, ptr.GetID()))
		case "Contents":
			// Special handling for Contents - must preserve stream structure
			contentsValue := page.Key(key)
			if contentsValue.Kind() == pdf.Array {
				// If Contents is an array, keep it as an array reference
				page_buffer.WriteString("  /Contents [")
				for i := 0; i < contentsValue.Len(); i++ {
					ptr := contentsValue.Index(i).GetPtr()
					page_buffer.WriteString(fmt.Sprintf(" %d 0 R", ptr.GetID()))
				}
				page_buffer.WriteString(" ]\n")
			} else {
				// If Contents is a single reference, keep it as a single reference
				ptr := contentsValue.GetPtr()
				page_buffer.WriteString(fmt.Sprintf("  /%s %d 0 R\n", key, ptr.GetID()))
			}
		case "Annots":
			page_buffer.WriteString("  /Annots [\n")
			for i := 0; i < page.Key("Annots").Len(); i++ {
				ptr := page.Key(key).Index(i).GetPtr()
				page_buffer.WriteString(fmt.Sprintf("    %d 0 R\n", ptr.GetID()))
			}
			for _, extra := range nuevas {
				page_buffer.WriteString(fmt.Sprintf("    %d 0 R\n", extra))
			}
			page_buffer.WriteString("  ]\n")
		default:
			// serializeCatalogEntry distingue valores directos (GetPtr devuelve
			// el id del objeto contenedor, aquí la página) de referencias
			// indirectas reales (id distinto), preservándolas como "N G R" en
			// lugar de inlinearlas con .String() — el inlining expande el
			// objeto resuelto y las referencias anidadas no resolubles acaban
			// como "0 0 R" colgantes.
			_, _ = fmt.Fprintf(&page_buffer, "  /%s ", key)
			context.serializeCatalogEntry(&page_buffer, pageObjectID, page.Key(key))
			page_buffer.WriteString("\n")
		}
	}

	if page.Key("Annots").IsNull() {
		page_buffer.WriteString("  /Annots [")
		for i, extra := range nuevas {
			if i > 0 {
				page_buffer.WriteString(" ")
			}
			page_buffer.WriteString(fmt.Sprintf("%d 0 R", extra))
		}
		page_buffer.WriteString("]\n")
	}

	page_buffer.WriteString(">>\n")

	return page_buffer.Bytes(), nil
}

// Helper function to find a page by its number
func findPageByNumber(pages pdf.Value, pageNumber uint32) (pdf.Value, error) {
	page, remaining, err := findPageByNumberRec(pages, pageNumber)
	if err != nil {
		return pdf.Value{}, err
	}
	if remaining != 0 {
		return pdf.Value{}, fmt.Errorf("page number %d not found", pageNumber)
	}
	return page, nil
}

// Internal recursive helper that returns the found page and the remaining page number to find.
func findPageByNumberRec(pages pdf.Value, pageNumber uint32) (pdf.Value, uint32, error) {
	if pages.Key("Type").Name() == "Pages" {
		kids := pages.Key("Kids")
		for i := 0; i < kids.Len(); i++ {
			page, remaining, err := findPageByNumberRec(kids.Index(i), pageNumber)
			if err == nil && remaining == 0 {
				return page, 0, nil
			}
			pageNumber = remaining
		}
		return pdf.Value{}, pageNumber, fmt.Errorf("page number %d not found", pageNumber)
	} else if pages.Key("Type").Name() == "Page" {
		if pageNumber == 1 {
			return pages, 0, nil
		}
		return pdf.Value{}, pageNumber - 1, nil
	}
	return pdf.Value{}, pageNumber, fmt.Errorf("page number %d not found", pageNumber)
}
