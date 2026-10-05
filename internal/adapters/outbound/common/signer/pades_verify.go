// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"grxfirma/internal/domain"
)

const (
	maxPDFEmbeddedSignatures = 128
	maxPDFDictionaryTokens   = 1_000_000
	maxPDFDictionaryNesting  = 256
	maxPDFByteRangeText      = 256
	maxPDFSignatureCMSBytes  = 16 << 20
)

// PAdESVerifier verifica firmas PDF detached apoyándose en el verificador CMS.
type PAdESVerifier struct {
	cades *CAdESVerifier
}

func NewPAdESVerifier() *PAdESVerifier {
	return &PAdESVerifier{cades: NewCAdESVerifier()}
}

func NewPAdESVerifierWithCAdES(verifier *CAdESVerifier) *PAdESVerifier {
	if verifier == nil {
		verifier = NewCAdESVerifier()
	}
	return &PAdESVerifier{cades: verifier}
}

type pdfEmbeddedSignature struct {
	ByteRange    [4]int
	CMSDER       []byte
	SubFilter    string
	ContentsFrom int
	ContentsTo   int
	RevisionEnd  int
	// SigningTimeM es /M del diccionario de firma, solo si está dentro del
	// rango firmado y tiene zona horaria.
	SigningTimeM time.Time
}

type pdfDictionaryRange struct {
	Start int
	End   int
}

func (v *PAdESVerifier) Verify(ctx context.Context, signedDocument domain.Document, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	signatures, err := extractPDFEmbeddedSignatures(signedDocument.Content)
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}

	aggregate := domain.NewVerificationSuccess(string(domain.FormatPAdES), "firma PAdES válida", nil)
	aggregate.Details = nil
	aggregate.Integrity.Details = nil
	aggregate.Format = string(domain.FormatPAdES)
	allSigners := make([]domain.CertificateRef, 0, len(signatures))
	hasFullDocumentCoverage := false
	var modifications []string
	var unanalyzable []string
	var incremental *pdfIncrementalAnalyzer

	for idx, signature := range signatures {
		signedBytes, err := extractPDFSignedBytes(signedDocument.Content, signature.ByteRange)
		if err != nil {
			return domain.VerificationResult{}, nil, err
		}
		cmsDER, err := normalizeBERToDER(signature.CMSDER)
		if err != nil {
			return domain.VerificationResult{}, nil, fmt.Errorf("normalizando CMS BER/DER: %w", err)
		}

		result, signers, err := v.cades.VerifyDetachedCMSWithAnchors(ctx, cmsDER, signedBytes, anchors)
		if err != nil {
			return domain.VerificationResult{}, nil, err
		}
		// Sin fecha en el CMS (lo normal en PAdES), la /M firmada del PDF.
		if len(result.SigningTimes) == 0 && !signature.SigningTimeM.IsZero() && len(signers) == 1 {
			result.SigningTimes = []domain.VerificationSigningTime{{
				Fingerprint: signers[0].Fingerprint,
				Time:        signature.SigningTimeM,
				Source:      domain.SigningTimeSourceSignedAttribute,
			}}
		}

		aggregate = mergePAdESVerificationResult(aggregate, result)
		aggregate.Material.CoberturasPDF = append(aggregate.Material.CoberturasPDF, domain.CoberturaPDF{
			InicioHueco: signature.ByteRange[1],
			FinRevision: signature.RevisionEnd,
		})
		aggregate.Details = append(aggregate.Details,
			fmt.Sprintf("firma_pdf=%d", idx+1),
			fmt.Sprintf("byterange=%d,%d,%d,%d", signature.ByteRange[0], signature.ByteRange[1], signature.ByteRange[2], signature.ByteRange[3]),
			fmt.Sprintf("subfilter=%s", signature.SubFilter),
			"subfiltro PDF detached verificado",
		)
		aggregate.Integrity.Details = append(aggregate.Integrity.Details,
			fmt.Sprintf("firma_pdf=%d", idx+1),
			fmt.Sprintf("byterange=%d,%d,%d,%d", signature.ByteRange[0], signature.ByteRange[1], signature.ByteRange[2], signature.ByteRange[3]),
			fmt.Sprintf("subfilter=%s", signature.SubFilter),
			"subfiltro PDF detached verificado",
		)
		if signature.RevisionEnd == len(signedDocument.Content) {
			hasFullDocumentCoverage = true
			aggregate.Details = append(aggregate.Details, fmt.Sprintf("cobertura_firma_pdf_%d=documento_completo", idx+1))
			aggregate.Integrity.Details = append(aggregate.Integrity.Details, fmt.Sprintf("cobertura_firma_pdf_%d=documento_completo", idx+1))
		} else {
			detail := fmt.Sprintf("cobertura_firma_pdf_%d=revision_hasta_%d_de_%d", idx+1, signature.RevisionEnd, len(signedDocument.Content))
			aggregate.Details = append(aggregate.Details, detail)
			aggregate.Integrity.Details = append(aggregate.Integrity.Details, detail)
			if incremental == nil {
				incremental = newPDFIncrementalAnalyzer(signedDocument.Content)
			}
			verdict := incremental.analyze(signature.RevisionEnd)
			for _, violation := range verdict.Violations {
				modifications = append(modifications, fmt.Sprintf("firma %d: %s", idx+1, violation))
			}
			if verdict.Unanalyzable != "" {
				unanalyzable = append(unanalyzable, fmt.Sprintf("firma %d: %s", idx+1, verdict.Unanalyzable))
			}
		}
		allSigners = append(allSigners, signers...)
	}

	if !aggregate.Valid && aggregate.Reason == "" {
		aggregate.Reason = "una o más firmas PAdES no son válidas"
	}
	if hasFullDocumentCoverage {
		aggregate.Coverage = "full"
	} else {
		aggregate.Coverage = "partial"
		aggregate.Warnings = append(aggregate.Warnings, "el PDF contiene bytes posteriores que ninguna firma cubre")
		if aggregate.Integrity.Status != domain.VerificationStatusInvalid {
			aggregate.Integrity.Status = domain.VerificationStatusWarning
		}
	}
	if aggregate.Valid {
		if hasFullDocumentCoverage {
			aggregate.Reason = "firma PAdES válida"
		} else {
			aggregate.Reason = "firma PAdES válida con contenido posterior no cubierto"
		}
		aggregate.Integrity.Reason = aggregate.Reason
	}
	// Una firma matemáticamente correcta no basta si, tras ella, se han
	// redefinido objetos firmados: el visor mostraría un documento distinto
	// del firmado (ataque de actualización incremental).
	switch {
	case len(modifications) > 0:
		const reason = "el PDF se modificó después de la firma: lo que se muestra no es lo que se firmó"
		aggregate.Valid = false
		aggregate.Reason = reason
		aggregate.Integrity.Status = domain.VerificationStatusInvalid
		aggregate.Integrity.Reason = reason
		aggregate.Integrity.Details = append(aggregate.Integrity.Details, modifications...)
		aggregate.Warnings = append(aggregate.Warnings, reason)
	case len(unanalyzable) > 0:
		const reason = "el PDF contiene cambios posteriores a la firma que no se han podido analizar; revise el documento"
		aggregate.Valid = false
		aggregate.Reason = reason
		if aggregate.Integrity.Status != domain.VerificationStatusInvalid {
			aggregate.Integrity.Status = domain.VerificationStatusWarning
		}
		aggregate.Integrity.Reason = reason
		aggregate.Integrity.Details = append(aggregate.Integrity.Details, unanalyzable...)
		aggregate.Warnings = append(aggregate.Warnings, reason)
	}
	aggregate.Details = append([]string{fmt.Sprintf("firmas_pdf=%d", len(signatures))}, aggregate.Details...)
	aggregate.Integrity.Details = append([]string{fmt.Sprintf("firmas_pdf=%d", len(signatures))}, aggregate.Integrity.Details...)
	return aggregate.Normalize(), allSigners, nil
}

func extractPDFEmbeddedSignatures(pdfBytes []byte) ([]pdfEmbeddedSignature, error) {
	return extractPDFEmbeddedSignaturesWithTimestamps(pdfBytes, false)
}

func extractPDFEmbeddedSignaturesWithTimestamps(pdfBytes []byte, timestamps bool) ([]pdfEmbeddedSignature, error) {
	text := string(pdfBytes)
	if candidates := strings.Count(text, "/ByteRange"); candidates > maxPDFEmbeddedSignatures {
		return nil, fmt.Errorf("demasiadas firmas PDF: máximo %d", maxPDFEmbeddedSignatures)
	}
	dicts, err := parsePDFDictionaryRanges(text)
	if err != nil {
		return nil, err
	}
	if len(dicts) == 0 {
		return nil, errors.New("no se encontraron diccionarios PDF")
	}

	out := make([]pdfEmbeddedSignature, 0, 2)
	seen := make(map[pdfDictionaryRange]struct{})
	searchFrom := 0
	for {
		idx := strings.Index(text[searchFrom:], "/ByteRange")
		if idx < 0 {
			break
		}
		idx += searchFrom
		searchFrom = idx + len("/ByteRange")

		dict, ok := findContainingPDFDictionary(dicts, idx)
		if !ok {
			return nil, fmt.Errorf("no se pudo resolver el diccionario de firma para ByteRange en %d", idx)
		}
		if _, dup := seen[dict]; dup {
			continue
		}
		seen[dict] = struct{}{}

		block := []byte(text[dict.Start:dict.End])
		byteRange, err := extractPDFByteRange(block)
		if err != nil {
			return nil, err
		}
		subFilter, err := extractPDFSubFilter(block)
		if err != nil {
			return nil, err
		}
		if err := validatePDFSubFilter(subFilter); err != nil && !(timestamps && subFilter == "ETSI.RFC3161") {
			continue
		}
		cmsDER, contentsFrom, contentsTo, err := extractPDFSignatureCMSWithRange(block)
		if err != nil {
			return nil, err
		}
		signature := pdfEmbeddedSignature{
			ByteRange:    byteRange,
			CMSDER:       cmsDER,
			SubFilter:    subFilter,
			ContentsFrom: dict.Start + contentsFrom,
			ContentsTo:   dict.Start + contentsTo,
			SigningTimeM: extractPDFSigningTimeM(block, dict.Start, byteRange),
		}
		revisionEnd, err := validatePDFSignatureByteRange(len(pdfBytes), signature)
		if err != nil {
			return nil, fmt.Errorf("ByteRange de firma PDF inválido: %w", err)
		}
		signature.RevisionEnd = revisionEnd
		out = append(out, signature)
		if len(out) > maxPDFEmbeddedSignatures {
			return nil, fmt.Errorf("demasiadas firmas PDF: máximo %d", maxPDFEmbeddedSignatures)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no se encontró ninguna firma PAdES compatible en el PDF")
	}
	return out, nil
}

func extractPDFByteRange(pdf []byte) ([4]int, error) {
	var out [4]int
	raw, err := extractPDFBracketValue(string(pdf), "/ByteRange")
	if err != nil {
		return out, err
	}
	if len(raw) > maxPDFByteRangeText {
		return out, errors.New("ByteRange excede el tamaño máximo permitido")
	}
	parts := strings.Fields(raw)
	if len(parts) != 4 {
		return out, fmt.Errorf("ByteRange inválido: %q", raw)
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return out, fmt.Errorf("entero inválido en ByteRange %q: %w", part, err)
		}
		out[i] = n
	}
	return out, nil
}

func extractPDFSignedBytes(pdf []byte, byteRange [4]int) ([]byte, error) {
	if byteRange[0] != 0 {
		return nil, errors.New("ByteRange debe comenzar en 0")
	}
	if byteRange[1] < 0 || byteRange[2] < 0 || byteRange[3] < 0 {
		return nil, errors.New("ByteRange contiene valores negativos")
	}
	firstEnd, err := checkedPDFRangeEnd(byteRange[0], byteRange[1], len(pdf))
	if err != nil {
		return nil, err
	}
	secondEnd, err := checkedPDFRangeEnd(byteRange[2], byteRange[3], len(pdf))
	if err != nil {
		return nil, err
	}
	if firstEnd > byteRange[2] {
		return nil, errors.New("ByteRange inconsistente")
	}
	if byteRange[1] > len(pdf)-byteRange[3] {
		return nil, errors.New("longitud firmada de ByteRange inválida")
	}
	signed := make([]byte, 0, byteRange[1]+byteRange[3])
	signed = append(signed, pdf[byteRange[0]:firstEnd]...)
	signed = append(signed, pdf[byteRange[2]:secondEnd]...)
	return signed, nil
}

func extractPDFSignatureCMS(pdf []byte) ([]byte, error) {
	cms, _, _, err := extractPDFSignatureCMSWithRange(pdf)
	return cms, err
}

func extractPDFSignatureCMSWithRange(pdf []byte) ([]byte, int, int, error) {
	hexValue, contentsFrom, contentsTo, err := extractPDFAngleValueRange(string(pdf), "/Contents")
	if err != nil {
		return nil, 0, 0, err
	}
	hexValue, err = compactPDFHex(hexValue)
	if err != nil {
		return nil, 0, 0, err
	}
	cms, err := hex.DecodeString(hexValue)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("firma CMS hexadecimal inválida: %w", err)
	}
	if len(cms) == 0 {
		return nil, 0, 0, errors.New("firma CMS vacía")
	}
	return cms, contentsFrom, contentsTo, nil
}

func validatePDFSignatureByteRange(pdfSize int, signature pdfEmbeddedSignature) (int, error) {
	byteRange := signature.ByteRange
	if byteRange[0] != 0 {
		return 0, errors.New("ByteRange debe comenzar en 0")
	}
	if byteRange[1] < 0 || byteRange[2] < 0 || byteRange[3] < 0 {
		return 0, errors.New("ByteRange contiene valores negativos")
	}
	firstEnd, err := checkedPDFRangeEnd(byteRange[0], byteRange[1], pdfSize)
	if err != nil {
		return 0, err
	}
	secondEnd, err := checkedPDFRangeEnd(byteRange[2], byteRange[3], pdfSize)
	if err != nil {
		return 0, err
	}
	if firstEnd > byteRange[2] {
		return 0, errors.New("los tramos de ByteRange se solapan o están desordenados")
	}
	if signature.ContentsFrom < 0 || signature.ContentsTo < signature.ContentsFrom || signature.ContentsTo > pdfSize {
		return 0, errors.New("posición de /Contents fuera del PDF")
	}
	if firstEnd != signature.ContentsFrom || byteRange[2] != signature.ContentsTo {
		return 0, errors.New("el hueco de ByteRange no coincide exactamente con /Contents")
	}
	return secondEnd, nil
}

func checkedPDFRangeEnd(start, length, limit int) (int, error) {
	if start < 0 || length < 0 {
		return 0, errors.New("ByteRange contiene valores negativos")
	}
	if start > limit || length > limit-start {
		return 0, errors.New("ByteRange excede el tamaño del PDF")
	}
	return start + length, nil
}

func extractPDFSubFilter(pdf []byte) (string, error) {
	text := string(pdf)
	idx := strings.LastIndex(text, "/SubFilter")
	if idx < 0 {
		return "", errors.New("no se encontró /SubFilter")
	}
	idx += len("/SubFilter")
	idx = skipPDFSpaces(text, idx)
	if idx >= len(text) || text[idx] != '/' {
		return "", errors.New("SubFilter PDF inválido")
	}
	idx++
	start := idx
	for idx < len(text) && isPDFNameChar(rune(text[idx])) {
		idx++
	}
	if start == idx {
		return "", errors.New("SubFilter PDF vacío")
	}
	return text[start:idx], nil
}

func validatePDFSubFilter(subFilter string) error {
	switch subFilter {
	case "ETSI.CAdES.detached", "adbe.pkcs7.detached":
		return nil
	default:
		return fmt.Errorf("SubFilter PDF no soportado: %s", subFilter)
	}
}

func parsePDFDictionaryRanges(text string) ([]pdfDictionaryRange, error) {
	ranges := make([]pdfDictionaryRange, 0)
	stack := make([]int, 0)
	for i := 0; i < len(text)-1; i++ {
		switch {
		case text[i] == '<' && text[i+1] == '<':
			if len(stack) >= maxPDFDictionaryNesting {
				return nil, fmt.Errorf("anidamiento de diccionarios PDF excesivo: máximo %d", maxPDFDictionaryNesting)
			}
			stack = append(stack, i)
			i++
		case text[i] == '>' && text[i+1] == '>':
			if len(stack) == 0 {
				i++
				continue
			}
			start := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			ranges = append(ranges, pdfDictionaryRange{Start: start, End: i + 2})
			if len(ranges) > maxPDFDictionaryTokens {
				return nil, fmt.Errorf("demasiados diccionarios PDF: máximo %d", maxPDFDictionaryTokens)
			}
			i++
		}
	}
	return ranges, nil
}

func findContainingPDFDictionary(dicts []pdfDictionaryRange, idx int) (pdfDictionaryRange, bool) {
	best := pdfDictionaryRange{}
	found := false
	bestLen := 0
	for _, dict := range dicts {
		if idx < dict.Start || idx >= dict.End {
			continue
		}
		length := dict.End - dict.Start
		if !found || length < bestLen {
			best = dict
			bestLen = length
			found = true
		}
	}
	return best, found
}

func mergePAdESVerificationResult(base, next domain.VerificationResult) domain.VerificationResult {
	base.Valid = base.Valid && next.Valid
	if !next.Valid && next.Reason != "" {
		base.Reason = next.Reason
	}
	base.Details = append(base.Details, next.Details...)
	base.Warnings = append(base.Warnings, next.Warnings...)
	base.Errors = append(base.Errors, next.Errors...)
	base.Evidence = append(base.Evidence, next.Evidence...)
	base.Material = base.Material.Anadir(next.Material)
	base.SigningTimes = append(base.SigningTimes, next.SigningTimes...)
	base.Integrity = mergeVerificationAspect(base.Integrity, next.Integrity)
	base.Certificate = mergeVerificationAspect(base.Certificate, next.Certificate)
	base.Trust = mergeVerificationAspect(base.Trust, next.Trust)
	return base
}

func mergeVerificationAspect(base, next domain.VerificationAspect) domain.VerificationAspect {
	if verificationAspectRank(next.Status) > verificationAspectRank(base.Status) {
		base.Status = next.Status
		if next.Reason != "" {
			base.Reason = next.Reason
		}
	}
	base.Details = append(base.Details, next.Details...)
	if base.Reason == "" && next.Reason != "" {
		base.Reason = next.Reason
	}
	return base
}

func verificationAspectRank(status domain.VerificationAspectStatus) int {
	switch status {
	case domain.VerificationStatusInvalid:
		return 4
	case domain.VerificationStatusWarning:
		return 3
	case domain.VerificationStatusValid:
		return 2
	case domain.VerificationStatusUnknown:
		return 1
	default:
		return 0
	}
}

func extractPDFBracketValue(text, key string) (string, error) {
	idx := strings.LastIndex(text, key)
	if idx < 0 {
		return "", fmt.Errorf("no se encontró %s", key)
	}
	idx += len(key)
	idx = skipPDFSpaces(text, idx)
	if idx >= len(text) || text[idx] != '[' {
		return "", fmt.Errorf("%s inválido", strings.TrimPrefix(key, "/"))
	}
	idx++
	start := idx
	for idx < len(text) && text[idx] != ']' {
		idx++
	}
	if idx >= len(text) {
		return "", fmt.Errorf("no se encontró el terminador ]")
	}
	return text[start:idx], nil
}

func extractPDFAngleValueRange(text, key string) (string, int, int, error) {
	searchFrom := len(text)
	for searchFrom >= 0 {
		idx := strings.LastIndex(text[:searchFrom], key)
		if idx < 0 {
			return "", 0, 0, fmt.Errorf("no se encontró %s", key)
		}
		idx += len(key)
		idx = skipPDFSpaces(text, idx)
		if idx < len(text) && text[idx] == '<' {
			contentsFrom := idx
			idx++
			start := idx
			for idx < len(text) && text[idx] != '>' {
				idx++
			}
			if idx >= len(text) {
				return "", 0, 0, fmt.Errorf("no se encontró el terminador >")
			}
			return text[start:idx], contentsFrom, idx + 1, nil
		}
		searchFrom = idx - len(key)
	}
	return "", 0, 0, fmt.Errorf("no se encontró %s", key)
}

func skipPDFSpaces(text string, idx int) int {
	for idx < len(text) {
		switch text[idx] {
		case ' ', '\t', '\r', '\n', '\f', '\x00':
			idx++
		default:
			return idx
		}
	}
	return idx
}

func compactPDFHex(raw string) (string, error) {
	hexDigits := 0
	for _, r := range raw {
		if unicode.IsSpace(r) || r == '\x00' {
			continue
		}
		hexDigits++
		if hexDigits > hex.EncodedLen(maxPDFSignatureCMSBytes) {
			return "", fmt.Errorf("firma CMS excede el máximo de %d bytes", maxPDFSignatureCMSBytes)
		}
	}
	var compact strings.Builder
	compact.Grow(hexDigits)
	for _, r := range raw {
		if unicode.IsSpace(r) || r == '\x00' {
			continue
		}
		compact.WriteRune(r)
	}
	return compact.String(), nil
}

func isPDFNameChar(r rune) bool {
	switch r {
	case ' ', '\t', '\r', '\n', '\f', '\x00', '/', '[', ']', '<', '>', '(', ')':
		return false
	default:
		return true
	}
}

func normalizeBERToDER(data []byte) ([]byte, error) {
	value, offset, err := decodeTLV(data, 0)
	if err != nil {
		return nil, err
	}
	if offset != len(data) && !onlyZeroPadding(data[offset:]) {
		return nil, fmt.Errorf("datos ASN.1 sobrantes: %d bytes", len(data)-offset)
	}
	return encodeTLV(value), nil
}

func onlyZeroPadding(data []byte) bool {
	for _, b := range data {
		if b != 0x00 {
			return false
		}
	}
	return true
}

type tlvNode struct {
	TagClass    byte
	Tag         int
	Constructed bool
	Content     []byte
	Children    []tlvNode
}

func decodeTLV(data []byte, offset int) (tlvNode, int, error) {
	if offset >= len(data) {
		return tlvNode{}, offset, errors.New("fin inesperado de datos ASN.1")
	}
	tagByte := data[offset]
	offset++
	if tagByte&0x1f == 0x1f {
		return tlvNode{}, offset, errors.New("etiquetas ASN.1 de alta cardinalidad no soportadas")
	}
	node := tlvNode{
		TagClass:    tagByte & 0xc0,
		Constructed: tagByte&0x20 != 0,
		Tag:         int(tagByte & 0x1f),
	}
	if offset >= len(data) {
		return tlvNode{}, offset, errors.New("longitud ASN.1 ausente")
	}
	lengthByte := data[offset]
	offset++
	switch {
	case lengthByte == 0x80:
		if !node.Constructed {
			return tlvNode{}, offset, errors.New("indefinido BER en valor primitivo no soportado")
		}
		for {
			if offset+1 < len(data) && data[offset] == 0x00 && data[offset+1] == 0x00 {
				offset += 2
				break
			}
			child, next, err := decodeTLV(data, offset)
			if err != nil {
				return tlvNode{}, offset, err
			}
			node.Children = append(node.Children, child)
			offset = next
		}
	case lengthByte&0x80 == 0:
		length := int(lengthByte)
		if offset+length > len(data) {
			return tlvNode{}, offset, errors.New("longitud ASN.1 excede el buffer")
		}
		content := data[offset : offset+length]
		offset += length
		if node.Constructed {
			childOffset := 0
			for childOffset < len(content) {
				child, next, err := decodeTLV(content, childOffset)
				if err != nil {
					return tlvNode{}, offset, err
				}
				node.Children = append(node.Children, child)
				childOffset = next
			}
			if childOffset != len(content) {
				return tlvNode{}, offset, errors.New("contenido construido ASN.1 inconsistente")
			}
		} else {
			node.Content = append([]byte(nil), content...)
		}
	default:
		numBytes := int(lengthByte & 0x7f)
		if numBytes == 0 || numBytes > 4 || offset+numBytes > len(data) {
			return tlvNode{}, offset, errors.New("longitud ASN.1 larga inválida")
		}
		length := 0
		for i := 0; i < numBytes; i++ {
			length = (length << 8) | int(data[offset+i])
		}
		offset += numBytes
		if offset+length > len(data) {
			return tlvNode{}, offset, errors.New("longitud ASN.1 larga excede el buffer")
		}
		content := data[offset : offset+length]
		offset += length
		if node.Constructed {
			childOffset := 0
			for childOffset < len(content) {
				child, next, err := decodeTLV(content, childOffset)
				if err != nil {
					return tlvNode{}, offset, err
				}
				node.Children = append(node.Children, child)
				childOffset = next
			}
			if childOffset != len(content) {
				return tlvNode{}, offset, errors.New("contenido construido ASN.1 inconsistente")
			}
		} else {
			node.Content = append([]byte(nil), content...)
		}
	}
	return node, offset, nil
}

func encodeTLV(node tlvNode) []byte {
	tag := node.TagClass | byte(node.Tag&0x1f)
	if node.Constructed {
		tag |= 0x20
	}

	var content []byte
	if node.Constructed {
		var buf bytes.Buffer
		for _, child := range node.Children {
			buf.Write(encodeTLV(child))
		}
		content = buf.Bytes()
	} else {
		content = node.Content
	}

	out := []byte{tag}
	out = append(out, encodeLength(len(content))...)
	out = append(out, content...)
	return out
}

func encodeLength(length int) []byte {
	if length < 0 {
		return nil
	}
	if length < 0x80 {
		return []byte{byte(length)}
	}
	var tmp [4]byte
	n := 0
	for length > 0 {
		tmp[len(tmp)-1-n] = byte(length & 0xff)
		length >>= 8
		n++
	}
	out := []byte{0x80 | byte(n)}
	out = append(out, tmp[len(tmp)-n:]...)
	return out
}
