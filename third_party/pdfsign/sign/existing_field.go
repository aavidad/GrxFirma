package sign

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/digitorus/pdf"
)

// Firma en un campo de firma vacío que ya existe en el formulario
// (signatureField de AutoFirma Java). Se localiza el campo por su nombre
// completo, se exige que sea /FT /Sig sin /V y que sea a la vez campo y
// widget, y se reescribe su diccionario conservando todas sus claves; solo
// se añaden /V y, si hay imagen de sello, una apariencia ajustada a su /Rect.

const (
	maxProfundidadCampos = 32
	// maxNodosCampos acota el recorrido del formulario. AutoFirmaV2: un
	// /Kids que repite el mismo hijo en cada nivel crece de forma
	// exponencial aunque la profundidad esté limitada.
	maxNodosCampos = 50000
)

type campoExistente struct {
	valor pdf.Value
	id    uint32
	gen   uint16
}

func (context *SignContext) buscarCampoFirma(nombre string) (campoExistente, error) {
	return buscarCampoFirmaEn(context.PDFReader, nombre)
}

func buscarCampoFirmaEn(rdr *pdf.Reader, nombre string) (campoExistente, error) {
	acroForm := rdr.Trailer().Key("Root").Key("AcroForm")
	if acroForm.IsNull() {
		return campoExistente{}, fmt.Errorf("el PDF no tiene formulario: no existe el campo de firma %q", nombre)
	}
	var encontrado []campoExistente
	vistos := make(map[uint32]bool)
	nodos := 0
	var recorrer func(campos pdf.Value, prefijo, ftHeredado string, profundidad int) error
	recorrer = func(campos pdf.Value, prefijo, ftHeredado string, profundidad int) error {
		if profundidad > maxProfundidadCampos {
			return errors.New("formulario con anidamiento excesivo")
		}
		contenedor, _ := campos.ObjectReference()
		for i := 0; i < campos.Len(); i++ {
			if nodos++; nodos > maxNodosCampos {
				return errors.New("formulario con demasiados campos")
			}
			campo := campos.Index(i)
			// Un campo indirecto que ya se ha visitado (ciclo o repetición)
			// no se recorre otra vez.
			if id, _ := campo.ObjectReference(); id != 0 && id != contenedor {
				if vistos[id] {
					continue
				}
				vistos[id] = true
			}
			parcial := campo.Key("T").Text()
			completo := parcial
			if prefijo != "" && parcial != "" {
				completo = prefijo + "." + parcial
			} else if parcial == "" {
				completo = prefijo
			}
			ft := ftHeredado
			if v := campo.Key("FT"); !v.IsNull() {
				ft = v.Name()
			}
			if kids := campo.Key("Kids"); !kids.IsNull() && kids.Len() > 0 && kids.Index(0).Key("T").Kind() != pdf.Null {
				if err := recorrer(kids, completo, ft, profundidad+1); err != nil {
					return err
				}
				continue
			}
			if completo == nombre || parcial == nombre {
				ptr := campo.GetPtr()
				if ft != "Sig" {
					return fmt.Errorf("el campo %q no es un campo de firma", nombre)
				}
				if !campo.Key("V").IsNull() {
					return fmt.Errorf("el campo de firma %q ya está firmado", nombre)
				}
				if !campo.Key("Kids").IsNull() {
					return fmt.Errorf("el campo de firma %q tiene varios widgets; no está soportado", nombre)
				}
				if ptr.GetID() == 0 {
					return fmt.Errorf("el campo de firma %q no es un objeto indirecto", nombre)
				}
				encontrado = append(encontrado, campoExistente{valor: campo, id: ptr.GetID(), gen: ptr.GetGen()})
			}
		}
		return nil
	}
	if err := recorrer(acroForm.Key("Fields"), "", "", 0); err != nil {
		return campoExistente{}, err
	}
	switch len(encontrado) {
	case 0:
		return campoExistente{}, fmt.Errorf("no existe el campo de firma %q en el PDF", nombre)
	case 1:
		return encontrado[0], nil
	default:
		return campoExistente{}, fmt.Errorf("el nombre de campo %q es ambiguo", nombre)
	}
}

// PaginaCampoFirma devuelve la página en la que está el widget del campo de
// firma vacío nombre, localizado con las mismas reglas que la firma
// (signatureField). Usa /P del widget y, si falta, busca el widget en las
// /Annots de las páginas. Devuelve una página nula si no la encuentra.
func PaginaCampoFirma(rdr *pdf.Reader, nombre string) (pdf.Page, error) {
	if !nombreCampoValido(nombre) {
		return pdf.Page{}, fmt.Errorf("nombre de campo de firma no válido")
	}
	campo, err := buscarCampoFirmaEn(rdr, nombre)
	if err != nil {
		return pdf.Page{}, err
	}
	if p := campo.valor.Key("P"); p.Key("Type").Name() == "Page" {
		return pdf.Page{V: p}, nil
	}
	// Un solo recorrido del árbol, acotado en páginas y anotaciones.
	var pagina pdf.Page
	vistas := 0
	err = recorrerHojas(rdr.Trailer().Key("Root").Key("Pages"), func(hoja pdf.Value) bool {
		if vistas++; vistas > maxPaginasTodas {
			return false
		}
		annots := hoja.Key("Annots")
		for i := 0; i < annots.Len() && i < maxNodosCampos; i++ {
			if id, _ := annots.Index(i).ObjectReference(); id == campo.id {
				pagina = pdf.Page{V: hoja}
				return false
			}
		}
		return true
	})
	if err != nil {
		return pdf.Page{}, err
	}
	return pagina, nil
}

// rellenarCampoFirma reescribe el campo existente apuntando a la firma.
func (context *SignContext) rellenarCampoFirma(nombre string) error {
	campo, err := context.buscarCampoFirma(nombre)
	if err != nil {
		return err
	}
	var rect [4]float64
	if r := campo.valor.Key("Rect"); r.Len() == 4 {
		for i := 0; i < 4; i++ {
			rect[i] = r.Index(i).Float64()
		}
	}
	apariencia := uint32(0)
	visible := rect[2] > rect[0] && rect[3] > rect[1]
	if visible && len(context.SignData.Appearance.Image) > 0 {
		ap, err := context.createAppearance(rect)
		if err != nil {
			return fmt.Errorf("apariencia del campo de firma: %w", err)
		}
		if apariencia, err = context.addObject(ap); err != nil {
			return err
		}
	}
	var b bytes.Buffer
	b.WriteString("<<\n")
	for _, clave := range campo.valor.Keys() {
		if clave == "V" || (clave == "AP" && apariencia != 0) {
			continue
		}
		fmt.Fprintf(&b, "  /%s ", clave)
		context.serializeCatalogEntry(&b, campo.id, campo.valor.Key(clave))
		b.WriteString("\n")
	}
	if apariencia != 0 {
		fmt.Fprintf(&b, "  /AP << /N %d 0 R >>\n", apariencia)
	}
	fmt.Fprintf(&b, "  /V %d 0 R\n>>\n", context.SignData.objectId)
	if campo.gen != 0 {
		return errors.New("el campo de firma pertenece a un objeto con generación distinta de 0; no está soportado")
	}
	if err := context.updateObject(campo.id, b.Bytes()); err != nil {
		return err
	}
	context.VisualSignData.objectId = campo.id
	return nil
}

func nombreCampoValido(nombre string) bool {
	return strings.TrimSpace(nombre) != "" && len(nombre) <= 256
}
