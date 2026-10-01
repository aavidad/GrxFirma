package sign

// Document Security Store (DSS) para PAdES B-LT / "LTV enabled".
//
// Escribe en la sección incremental de la firma los objetos stream con los
// certificados de la cadena, respuestas OCSP y CRLs, y un diccionario /DSS
// (ISO 32000-2 §12.8.4.3) referenciado desde el catálogo. Al ir en la misma
// revisión que la firma, el material queda cubierto por su ByteRange.
//
// Si el documento ya tenía un /DSS de una revisión anterior (cofirma), sus
// referencias se conservan y se combinan con las nuevas.

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/digitorus/pdf"
)

// createDSSObjects escribe los streams de material de validación y el
// diccionario /DSS. Retorna el ID del objeto /DSS, o 0 si no hay nada que
// incrustar.
func (context *SignContext) createDSSObjects() (uint32, error) {
	vd := context.SignData.ValidationData
	if vd.IsEmpty() {
		return 0, nil
	}

	certRefs, err := context.addValidationStreams(vd.Certs)
	if err != nil {
		return 0, fmt.Errorf("failed to add DSS certificate streams: %w", err)
	}
	ocspRefs, err := context.addValidationStreams(vd.OCSPs)
	if err != nil {
		return 0, fmt.Errorf("failed to add DSS OCSP streams: %w", err)
	}
	crlRefs, err := context.addValidationStreams(vd.CRLs)
	if err != nil {
		return 0, fmt.Errorf("failed to add DSS CRL streams: %w", err)
	}

	// Conservar las referencias de un /DSS previo (streams de revisiones
	// anteriores siguen siendo objetos válidos).
	prevCerts, prevOCSPs, prevCRLs := context.existingDSSRefs()
	certRefs = append(prevCerts, certRefs...)
	ocspRefs = append(prevOCSPs, ocspRefs...)
	crlRefs = append(prevCRLs, crlRefs...)

	var dss bytes.Buffer
	dss.WriteString("<<\n  /Type /DSS\n")
	writeDSSArray(&dss, "Certs", certRefs)
	writeDSSArray(&dss, "OCSPs", ocspRefs)
	writeDSSArray(&dss, "CRLs", crlRefs)
	dss.WriteString(">>")

	dssID, err := context.addObject(dss.Bytes())
	if err != nil {
		return 0, fmt.Errorf("failed to add DSS object: %w", err)
	}
	return dssID, nil
}

// addValidationStreams escribe cada blob DER como objeto stream y retorna sus IDs.
func (context *SignContext) addValidationStreams(blobs [][]byte) ([]uint32, error) {
	var ids []uint32
	for _, blob := range blobs {
		if len(blob) == 0 {
			continue
		}
		var obj bytes.Buffer
		obj.WriteString("<< /Length " + strconv.Itoa(len(blob)) + " >>\nstream\n")
		obj.Write(blob)
		obj.WriteString("\nendstream")
		id, err := context.addObject(obj.Bytes())
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// existingDSSRefs recupera las referencias de los arrays Certs/OCSPs/CRLs de
// un /DSS previo del documento, si existe. Los elementos de esos arrays son
// siempre referencias indirectas a streams, por lo que GetPtr retorna el ID
// del propio stream.
func (context *SignContext) existingDSSRefs() (certs, ocsps, crls []uint32) {
	root := context.PDFReader.Trailer().Key("Root")
	dss := root.Key("DSS")
	if dss.IsNull() || dss.Kind() != pdf.Dict {
		return nil, nil, nil
	}
	collect := func(name string) []uint32 {
		arr := dss.Key(name)
		var ids []uint32
		for i := 0; i < arr.Len(); i++ {
			ptr := arr.Index(i).GetPtr()
			if id := ptr.GetID(); id != 0 {
				ids = append(ids, id)
			}
		}
		return ids
	}
	return collect("Certs"), collect("OCSPs"), collect("CRLs")
}

func writeDSSArray(buf *bytes.Buffer, name string, refs []uint32) {
	if len(refs) == 0 {
		return
	}
	buf.WriteString("  /" + name + " [")
	for i, id := range refs {
		if i > 0 {
			buf.WriteString(" ")
		}
		buf.WriteString(strconv.Itoa(int(id)) + " 0 R")
	}
	buf.WriteString("]\n")
}
