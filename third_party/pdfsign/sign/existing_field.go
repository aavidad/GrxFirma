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

const maxProfundidadCampos = 32

type campoExistente struct {
	valor pdf.Value
	id    uint32
	gen   uint16
}

func (context *SignContext) buscarCampoFirma(nombre string) (campoExistente, error) {
	acroForm := context.PDFReader.Trailer().Key("Root").Key("AcroForm")
	if acroForm.IsNull() {
		return campoExistente{}, fmt.Errorf("el PDF no tiene formulario: no existe el campo de firma %q", nombre)
	}
	var encontrado []campoExistente
	var recorrer func(campos pdf.Value, prefijo, ftHeredado string, profundidad int) error
	recorrer = func(campos pdf.Value, prefijo, ftHeredado string, profundidad int) error {
		if profundidad > maxProfundidadCampos {
			return errors.New("formulario con anidamiento excesivo")
		}
		for i := 0; i < campos.Len(); i++ {
			campo := campos.Index(i)
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
