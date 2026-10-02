// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"

	pdf "github.com/digitorus/pdf"
)

const maxPAdESV2Revisions = 20
const maxPAdESV2Signatures = 20
const maxPAdESV2Bytes = 72 << 20
const hardMaxPAdESV2Bytes = 100 << 20

// FirmaPDFV2 contains the exact bytes needed to verify one PDF signature.
// CMS and signed content are internal material; callers must never serialize
// them. Field names are deliberately absent from this type.
type FirmaPDFV2 struct {
	ByteRange             [4]int
	RevisionLongitud      int
	RevisionHuellaSHA256  string
	ContenidoHuellaSHA256 string
	CMSDER                []byte
	ContenidoFirmado      []byte
	CubreRevisionCompleta bool
	TipoFirma             string
	NivelDocMDP           *int
	CambiosDesdeAnterior  CambiosPDFV2
	objetoFirma           uint32
}

type CambiosPDFV2 struct {
	Estado          string
	Detalle         []string
	BytesNoFirmados int
}

type InspeccionPDFV2 struct {
	Firmas                     []FirmaPDFV2
	Revisiones                 []pdf.XRefSnapshot
	Longitudes                 []int
	BytesDespuesUltimaRevision int
	CambiosPosteriores         CambiosPDFV2
}

var pdfStartXRefEOF = regexp.MustCompile(`startxref[\x00\t\n\f\r ]+([0-9]{1,12})[\x00\t\n\f\r ]+%%EOF`)

// InspeccionarPAdESV2 validates every complete update, the /Prev chain and
// the link between each active signature field and its ByteRange. A failure
// means the caller must issue an indeterminate verdict, never an approval.
func InspeccionarPAdESV2(data []byte, maxFirmas, maxRevisiones int, maxBytes ...int) (InspeccionPDFV2, error) {
	limit := maxPAdESV2Bytes
	if len(maxBytes) > 0 && maxBytes[0] > 0 {
		limit = maxBytes[0]
		if limit > hardMaxPAdESV2Bytes {
			limit = hardMaxPAdESV2Bytes
		}
	}
	if len(data) == 0 {
		return InspeccionPDFV2{}, errors.New("pdf_vacio")
	}
	if len(data) > limit {
		return InspeccionPDFV2{}, errors.New("tamano_pdf_fuera_de_limite")
	}
	if maxFirmas <= 0 {
		maxFirmas = maxPAdESV2Signatures
	}
	if maxRevisiones <= 0 {
		maxRevisiones = maxPAdESV2Revisions
	}
	if maxFirmas > maxPAdESV2Signatures {
		maxFirmas = maxPAdESV2Signatures
	}
	if maxRevisiones > maxPAdESV2Revisions {
		maxRevisiones = maxPAdESV2Revisions
	}
	lengths := make([]int, 0, 4)
	for _, loc := range pdfStartXRefEOF.FindAllSubmatchIndex(data, -1) {
		startXRef, err := strconv.ParseInt(string(data[loc[2]:loc[3]]), 10, 64)
		if err != nil || startXRef <= 0 || startXRef >= int64(loc[0]) {
			continue
		}
		end := loc[1]
		if end < len(data) && data[end] == '\r' {
			end++
		}
		if end < len(data) && data[end] == '\n' {
			end++
		}
		lengths = append(lengths, end)
	}
	if len(lengths) == 0 || len(lengths) > maxRevisiones {
		return InspeccionPDFV2{}, errors.New("revisiones_pdf_fuera_de_limite")
	}
	snapshots, err := pdf.ReadXRefSnapshots(data, lengths, maxRevisiones)
	if err != nil {
		return InspeccionPDFV2{}, fmt.Errorf("xref_no_comprobados: %w", err)
	}
	if snapshots[0].PrevXRef != 0 {
		return InspeccionPDFV2{}, errors.New("cadena_xref_incompleta")
	}
	for i := 1; i < len(snapshots); i++ {
		if snapshots[i].PrevXRef != snapshots[i-1].StartXRef {
			return InspeccionPDFV2{}, errors.New("cadena_xref_incompleta")
		}
	}
	signatures, err := extractPDFEmbeddedSignatures(data)
	if err != nil {
		return InspeccionPDFV2{}, fmt.Errorf("firmas_pdf_no_comprobadas: %w", err)
	}
	if len(signatures) > maxFirmas {
		return InspeccionPDFV2{}, errors.New("firmas_pdf_fuera_de_limite")
	}
	sort.Slice(signatures, func(i, j int) bool { return signatures[i].RevisionEnd < signatures[j].RevisionEnd })
	out := InspeccionPDFV2{Revisiones: snapshots, Longitudes: lengths, BytesDespuesUltimaRevision: len(data) - lengths[len(lengths)-1]}
	last := 0
	for _, sig := range signatures {
		revisionLength := revisionLengthForEnd(lengths, sig.RevisionEnd)
		if sig.RevisionEnd <= last || revisionLength == 0 || revisionLength <= last {
			return InspeccionPDFV2{}, errors.New("byterange_no_cubre_revision_completa")
		}
		objectNumber, active := firmaEnCampoActivo(data[:revisionLength], sig.ByteRange)
		entry := snapshots[revisionIndex(lengths, revisionLength)].Entries[objectNumber]
		if !active || !entry.Active || entry.InStream || entry.Offset < 0 || int(entry.Offset) > sig.ContentsFrom || sig.ContentsTo > revisionLength {
			return InspeccionPDFV2{}, errors.New("firma_sin_campo_activo")
		}
		if end := bytes.Index(data[entry.Offset:revisionLength], []byte("endobj")); end < 0 || sig.ContentsTo > int(entry.Offset)+end {
			return InspeccionPDFV2{}, errors.New("firma_no_coincide_con_xref_activo")
		}
		content, err := extractPDFSignedBytes(data, sig.ByteRange)
		if err != nil {
			return InspeccionPDFV2{}, err
		}
		revisionHash := sha256.Sum256(data[:revisionLength])
		contentHash := sha256.Sum256(content)
		out.Firmas = append(out.Firmas, FirmaPDFV2{
			ByteRange: sig.ByteRange, RevisionLongitud: revisionLength,
			RevisionHuellaSHA256:  hex.EncodeToString(revisionHash[:]),
			ContenidoHuellaSHA256: hex.EncodeToString(contentHash[:]),
			CMSDER:                sig.CMSDER, ContenidoFirmado: content,
			CubreRevisionCompleta: sig.RevisionEnd == revisionLength,
			TipoFirma:             "aprobacion",
			objetoFirma:           objectNumber,
		})
		last = revisionLength
	}
	if err := clasificarActualizacionesPAdESV2(data, &out); err != nil {
		return InspeccionPDFV2{}, err
	}
	return out, nil
}

func revisionLengthForEnd(lengths []int, end int) int {
	for _, length := range lengths {
		if length >= end {
			return length
		}
	}
	return 0
}

func revisionIndex(lengths []int, want int) int {
	for i, got := range lengths {
		if got == want {
			return i
		}
	}
	return -1
}

func firmaEnCampoActivo(data []byte, byteRange [4]int) (number uint32, ok bool) {
	defer func() {
		if recover() != nil {
			number, ok = 0, false
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, false
	}
	root := r.Trailer().Key("Root")
	fields := root.Key("AcroForm").Key("Fields")
	seen := make(map[uint32]bool)
	var visit func(pdf.Value, string, int) uint32
	visit = func(field pdf.Value, inheritedFT string, depth int) uint32 {
		if depth > 32 {
			return 0
		}
		number, _ := field.ObjectReference()
		if number != 0 {
			if seen[number] {
				return 0
			}
			seen[number] = true
		}
		ft := field.Key("FT").Name()
		if ft == "" {
			ft = inheritedFT
		}
		if ft == "Sig" {
			v := field.Key("V")
			br := v.Key("ByteRange")
			if br.Len() == 4 {
				match := true
				for i := 0; i < 4; i++ {
					match = match && br.Index(i).Int64() == int64(byteRange[i])
				}
				if match && v.Key("Contents").Kind() == pdf.String {
					ref, _ := v.ObjectReference()
					return ref
				}
			}
		}
		kids := field.Key("Kids")
		for i := 0; i < kids.Len(); i++ {
			if ref := visit(kids.Index(i), ft, depth+1); ref != 0 {
				return ref
			}
		}
		return 0
	}
	for i := 0; i < fields.Len(); i++ {
		if ref := visit(fields.Index(i), "", 0); ref != 0 {
			return ref, true
		}
	}
	return 0, false
}
