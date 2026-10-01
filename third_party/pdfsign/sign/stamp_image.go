package sign

import (
	"bytes"
	"fmt"
	"sort"
)

const maxStamps = 32

// crearSellosImagen añade las imágenes pedidas como anotaciones /Stamp
// bloqueadas e imprimibles. Visualmente equivale al estampado de AutoFirma
// Java sin reescribir el contenido ni los recursos heredados de la página.
func (context *SignContext) crearSellosImagen() error {
	sellos := context.SignData.Appearance.Stamps
	if len(sellos) == 0 {
		return nil
	}
	if len(sellos) > maxStamps {
		return fmt.Errorf("demasiadas imágenes que estampar (%d)", len(sellos))
	}
	root := context.PDFReader.Trailer().Key("Root")
	for i, sello := range sellos {
		if sello.Rect[2]-sello.Rect[0] < 1 || sello.Rect[3]-sello.Rect[1] < 1 || len(sello.Image) == 0 {
			return fmt.Errorf("imagen %d: área o imagen no válida", i+1)
		}
		page, err := findPageByNumber(root.Key("Pages"), sello.Page)
		if err != nil {
			return fmt.Errorf("imagen %d: %w", i+1, err)
		}
		pagePtr := page.GetPtr()
		ap, err := context.aparienciaImagen(sello)
		if err != nil {
			return fmt.Errorf("imagen %d: %w", i+1, err)
		}
		apID, err := context.addObject(ap)
		if err != nil {
			return err
		}
		var b bytes.Buffer
		b.WriteString("<<\n  /Type /Annot\n  /Subtype /Stamp\n")
		fmt.Fprintf(&b, "  /Rect [%f %f %f %f]\n", sello.Rect[0], sello.Rect[1], sello.Rect[2], sello.Rect[3])
		fmt.Fprintf(&b, "  /P %d %d R\n", pagePtr.GetID(), pagePtr.GetGen())
		fmt.Fprintf(&b, "  /F %d\n", AnnotationFlagPrint|AnnotationFlagLocked|AnnotationFlagLockedContents)
		fmt.Fprintf(&b, "  /NM %s\n", pdfString(fmt.Sprintf("AutoFirmaV2 imagen %d", i+1)))
		fmt.Fprintf(&b, "  /AP << /N %d 0 R >>\n>>\n", apID)
		annotID, err := context.addObject(b.Bytes())
		if err != nil {
			return err
		}
		if context.VisualSignData.annotsPendientes == nil {
			context.VisualSignData.annotsPendientes = map[uint32][]uint32{}
		}
		context.VisualSignData.annotsPendientes[sello.Page] = append(context.VisualSignData.annotsPendientes[sello.Page], annotID)
	}
	return nil
}

// aparienciaImagen reutiliza el generador de apariencias con la imagen del
// sello y sin texto.
func (context *SignContext) aparienciaImagen(sello StampImage) ([]byte, error) {
	original := context.SignData.Appearance
	defer func() { context.SignData.Appearance = original }()
	context.SignData.Appearance.Image = sello.Image
	context.SignData.Appearance.ImageAsWatermark = false
	context.SignData.Appearance.LogoImage = nil
	return context.createAppearance(sello.Rect)
}

// actualizarPaginasPendientes escribe las páginas que solo reciben sellos
// de imagen (las que también llevan la firma ya se actualizaron con ella).
func (context *SignContext) actualizarPaginasPendientes() error {
	paginas := make([]int, 0, len(context.VisualSignData.annotsPendientes))
	for p := range context.VisualSignData.annotsPendientes {
		paginas = append(paginas, int(p))
	}
	sort.Ints(paginas)
	root := context.PDFReader.Trailer().Key("Root")
	for _, p := range paginas {
		page, err := findPageByNumber(root.Key("Pages"), uint32(p)) // #nosec G115 -- número de página procedente de un uint32.
		if err != nil {
			return err
		}
		update, err := context.createIncPageUpdate(uint32(p), 0) // #nosec G115 -- número de página procedente de un uint32.
		if err != nil {
			return err
		}
		ptr := page.GetPtr()
		if err := context.updateObject(ptr.GetID(), update); err != nil {
			return err
		}
	}
	return nil
}
