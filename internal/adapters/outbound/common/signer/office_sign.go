// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/common/officecontainer"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

const (
	formatODF   = domain.SignatureFormat("ODF")
	formatOOXML = domain.SignatureFormat("OOXML")

	odfSignatureNamespace = "urn:oasis:names:tc:opendocument:xmlns:digitalsignature:1.0"
	odfSignatureEntry     = "META-INF/documentsignatures.xml"
	odfSignatureRootOpen  = `<?xml version="1.0" encoding="UTF-8"?><document-signatures xmlns="` + odfSignatureNamespace + `">`
	odfSignatureRootClose = `</document-signatures>`

	ooxmlSigContentType    = "application/vnd.openxmlformats-package.digital-signature-xmlsignature+xml"
	ooxmlOriginContentType = "application/vnd.openxmlformats-package.digital-signature-origin"
	ooxmlOriginRelType     = "http://schemas.openxmlformats.org/package/2006/relationships/digital-signature/origin"
	ooxmlSignatureRelType  = "http://schemas.openxmlformats.org/package/2006/relationships/digital-signature/signature"
	ooxmlRelsSchema        = "http://schemas.openxmlformats.org/package/2006/relationships"
	ooxmlTypesSchema       = "http://schemas.openxmlformats.org/package/2006/content-types"
)

type ODFDetached struct{}
type OOXMLDetached struct{}

func NewODFDetached() *ODFDetached     { return &ODFDetached{} }
func NewOOXMLDetached() *OOXMLDetached { return &OOXMLDetached{} }

func (e *ODFDetached) Sign(ctx context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.SignatureResult{}, err
	}
	if err := validarTrabajoOffice(job, formatODF); err != nil {
		return domain.SignatureResult{}, err
	}
	clave, err := requireLocalSigningKeyXAdES(key)
	if err != nil {
		return domain.SignatureResult{}, err
	}
	if officecontainer.Detect(job.Document.Content) != officecontainer.KindODF {
		return domain.SignatureResult{}, errors.New("los datos introducidos no se corresponden con un documento ODF")
	}

	archive, err := openMutableZip(job.Document.Content)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("abriendo contenedor ODF: %w", err)
	}
	refs, err := buildODFReferences(archive)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("construyendo referencias ODF: %w", err)
	}
	signatureXML, err := buildOfficeSignatureXML(clave, refs, time.Now().UTC(), officeSignatureOptions{
		DateNamespaceURI: "http://purl.org/dc/elements/1.1/",
		DateElementName:  "dc:date",
	})
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("construyendo firma ODF: %w", err)
	}
	if current, ok := archive.Get(odfSignatureEntry); ok && len(bytes.TrimSpace(current)) > 0 {
		archive.Put(odfSignatureEntry, appendSignatureToODFEnvelope(current, signatureXML))
	} else {
		archive.Put(odfSignatureEntry, []byte(odfSignatureRootOpen+signatureXML+odfSignatureRootClose))
	}

	signed, err := archive.Bytes()
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("empaquetando ODF firmado: %w", err)
	}
	return domain.SignatureResult{
		Format:    formatODF,
		Data:      signed,
		Algorithm: "ODF-XMLDSig-RSA-SHA256",
	}, nil
}

func (e *OOXMLDetached) Sign(ctx context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.SignatureResult{}, err
	}
	if err := validarTrabajoOffice(job, formatOOXML); err != nil {
		return domain.SignatureResult{}, err
	}
	clave, err := requireLocalSigningKeyXAdES(key)
	if err != nil {
		return domain.SignatureResult{}, err
	}
	if officecontainer.Detect(job.Document.Content) != officecontainer.KindOOXML {
		return domain.SignatureResult{}, errors.New("los datos introducidos no se corresponden con un documento OOXML")
	}

	archive, err := openMutableZip(job.Document.Content)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("abriendo contenedor OOXML: %w", err)
	}
	signatureEntry := nextOOXMLSignatureEntry(archive)
	if err := updateOOXMLScaffolding(archive, signatureEntry); err != nil {
		return domain.SignatureResult{}, fmt.Errorf("preparando andamiaje OOXML: %w", err)
	}
	refs, err := buildOOXMLReferences(archive)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("construyendo referencias OOXML: %w", err)
	}
	signatureXML, err := buildOfficeSignatureXML(clave, refs, time.Now().UTC(), officeSignatureOptions{
		DateWrapperXML: `<mdssi:SignatureTime xmlns:mdssi="http://schemas.openxmlformats.org/package/2006/digital-signature"><mdssi:Format>YYYY-MM-DDThh:mm:ssTZD</mdssi:Format><mdssi:Value>%s</mdssi:Value></mdssi:SignatureTime>`,
	})
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("construyendo firma OOXML: %w", err)
	}
	archive.Put(signatureEntry, []byte(signatureXML))

	signed, err := archive.Bytes()
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("empaquetando OOXML firmado: %w", err)
	}
	return domain.SignatureResult{
		Format:    formatOOXML,
		Data:      signed,
		Algorithm: "OOXML-XMLDSig-RSA-SHA256",
	}, nil
}

func validarTrabajoOffice(job domain.SignatureJob, expected domain.SignatureFormat) error {
	if err := job.Validate(); err != nil {
		return err
	}
	if job.Format != expected {
		return fmt.Errorf("este motor solo soporta formato %s", expected)
	}
	switch job.Action {
	case domain.ActionSign, domain.ActionCoSign:
		return nil
	default:
		return errors.New("este motor solo soporta sign y cosign")
	}
}

type mutableZip struct {
	order   []string
	entries map[string][]byte
}

func openMutableZip(data []byte) (*mutableZip, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	out := &mutableZip{
		order:   make([]string, 0, len(reader.File)),
		entries: make(map[string][]byte, len(reader.File)),
	}
	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		name := strings.ReplaceAll(f.Name, "\\", "/")
		out.order = append(out.order, name)
		out.entries[name] = content
	}
	return out, nil
}

func (m *mutableZip) Get(name string) ([]byte, bool) {
	if m == nil {
		return nil, false
	}
	data, ok := m.entries[strings.ReplaceAll(name, "\\", "/")]
	return data, ok
}

func (m *mutableZip) Put(name string, data []byte) {
	if m == nil {
		return
	}
	name = strings.ReplaceAll(name, "\\", "/")
	if _, ok := m.entries[name]; !ok {
		m.order = append(m.order, name)
	}
	m.entries[name] = append([]byte(nil), data...)
}

func (m *mutableZip) Bytes() ([]byte, error) {
	if m == nil {
		return nil, errors.New("zip mutable nulo")
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	written := make(map[string]struct{}, len(m.entries))
	writeEntry := func(name string, data []byte) error {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		if name == "mimetype" {
			header.Method = zip.Store
			header.UncompressedSize64 = uint64(len(data))
			header.CRC32 = crc32.ChecksumIEEE(data)
		}
		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err = w.Write(data); err != nil {
			return err
		}
		written[name] = struct{}{}
		return nil
	}
	for _, name := range m.order {
		data, ok := m.entries[name]
		if !ok {
			continue
		}
		if err := writeEntry(name, data); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	extras := make([]string, 0, len(m.entries))
	for name := range m.entries {
		if _, ok := written[name]; !ok {
			extras = append(extras, name)
		}
	}
	sort.Strings(extras)
	for _, name := range extras {
		if err := writeEntry(name, m.entries[name]); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type officeReference struct {
	ID         string
	URI        string
	DigestB64  string
	Transforms []string
	Type       string
}

func buildODFReferences(archive *mutableZip) ([]officeReference, error) {
	names := make([]string, 0, len(archive.entries))
	for name := range archive.entries {
		if strings.EqualFold(name, odfSignatureEntry) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	refs := make([]officeReference, 0, len(names))
	for idx, name := range names {
		digestB64, err := digestOfficeEntry(archive.entries[name])
		if err != nil {
			return nil, fmt.Errorf("entrada %s: %w", name, err)
		}
		ref := officeReference{
			ID:        fmt.Sprintf("Reference-ODF-%d", idx+1),
			URI:       strings.ReplaceAll(name, " ", "%20"),
			DigestB64: digestB64,
		}
		if isXMLLike(name, archive.entries[name]) {
			ref.Transforms = []string{algExcC14N}
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func buildOOXMLReferences(archive *mutableZip) ([]officeReference, error) {
	contentTypes, _ := parseOOXMLContentTypes(archive.entries["[Content_Types].xml"])
	names := make([]string, 0, len(archive.entries))
	for name := range archive.entries {
		if strings.EqualFold(name, "[Content_Types].xml") ||
			strings.HasPrefix(strings.ToLower(name), "_xmlsignatures/") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	refs := make([]officeReference, 0, len(names))
	for idx, name := range names {
		digestB64, err := digestOfficeEntry(archive.entries[name])
		if err != nil {
			return nil, fmt.Errorf("entrada %s: %w", name, err)
		}
		uri := "/" + name
		if ct := contentTypes.ContentTypeFor(name); ct != "" {
			uri += "?ContentType=" + ct
		}
		ref := officeReference{
			ID:        fmt.Sprintf("Reference-OOXML-%d", idx+1),
			URI:       uri,
			DigestB64: digestB64,
		}
		if isXMLLike(name, archive.entries[name]) {
			ref.Transforms = []string{algExcC14N}
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func nextOOXMLSignatureEntry(archive *mutableZip) string {
	occupied := make(map[string]struct{}, len(archive.entries))
	for name := range archive.entries {
		normalized := strings.ToLower(strings.ReplaceAll(name, "\\", "/"))
		occupied[normalized] = struct{}{}
	}
	for index := 1; ; index++ {
		candidate := fmt.Sprintf("_xmlsignatures/sig%d.xml", index)
		if _, exists := occupied[candidate]; !exists {
			return candidate
		}
	}
}

func digestOfficeEntry(data []byte) (string, error) {
	payload := data
	if c14n, err := exclusiveC14N(string(data)); err == nil {
		payload = c14n
	}
	sum := sha256.Sum256(payload)
	return base64.StdEncoding.EncodeToString(sum[:]), nil
}

func isXMLLike(name string, data []byte) bool {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".xml") || strings.HasSuffix(lower, ".rels") {
		return true
	}
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) > 0 && trimmed[0] == '<'
}

type officeSignatureOptions struct {
	DateNamespaceURI string
	DateElementName  string
	DateWrapperXML   string
}

func buildOfficeSignatureXML(key *LocalSigningKey, refs []officeReference, now time.Time, opts officeSignatureOptions) (string, error) {
	sigID := fmt.Sprintf("Signature-%d", now.UnixNano())
	keyInfoID := fmt.Sprintf("KeyInfo-%d", now.UnixNano())
	propertyID := fmt.Sprintf("SignatureProperty-%d", now.UnixNano())

	propertyXML, err := buildOfficeSignaturePropertyXML(sigID, propertyID, now, opts)
	if err != nil {
		return "", err
	}

	signedInfoXML := buildOfficeSignedInfoXML(refs)
	signedInfoC14N, err := exclusiveC14N(signedInfoXML)
	if err != nil {
		return "", err
	}
	sigDigest := sha256.Sum256(signedInfoC14N)
	sigBytes, err := key.Signer.Sign(rand.Reader, sigDigest[:], crypto.SHA256)
	if err != nil {
		return "", err
	}
	keyInfoXML := buildKeyInfoXML(keyInfoID, base64.StdEncoding.EncodeToString(key.Certificate.Raw))
	return fmt.Sprintf(`<ds:Signature xmlns:ds="%s" Id="%s">%s<ds:SignatureValue>%s</ds:SignatureValue>%s<ds:Object>%s</ds:Object></ds:Signature>`,
		nsXMLDSig,
		escapeXMLAttr(sigID),
		signedInfoXML,
		base64.StdEncoding.EncodeToString(sigBytes),
		keyInfoXML,
		propertyXML,
	), nil
}

func buildOfficeSignedInfoXML(refs []officeReference) string {
	var b strings.Builder
	b.WriteString(`<ds:SignedInfo xmlns:ds="` + nsXMLDSig + `">`)
	b.WriteString(`<ds:CanonicalizationMethod Algorithm="` + algExcC14N + `"/>`)
	b.WriteString(`<ds:SignatureMethod Algorithm="` + algRSASHA256 + `"/>`)
	for _, ref := range refs {
		b.WriteString(`<ds:Reference`)
		if strings.TrimSpace(ref.ID) != "" {
			b.WriteString(` Id="` + escapeXMLAttr(ref.ID) + `"`)
		}
		b.WriteString(` URI="` + escapeXMLAttr(ref.URI) + `"`)
		if strings.TrimSpace(ref.Type) != "" {
			b.WriteString(` Type="` + escapeXMLAttr(ref.Type) + `"`)
		}
		b.WriteString(`>`)
		if len(ref.Transforms) > 0 {
			b.WriteString(`<ds:Transforms>`)
			for _, transform := range ref.Transforms {
				b.WriteString(`<ds:Transform Algorithm="` + escapeXMLAttr(transform) + `"/>`)
			}
			b.WriteString(`</ds:Transforms>`)
		}
		b.WriteString(`<ds:DigestMethod Algorithm="` + algSHA256 + `"/>`)
		b.WriteString(`<ds:DigestValue>` + ref.DigestB64 + `</ds:DigestValue>`)
		b.WriteString(`</ds:Reference>`)
	}
	b.WriteString(`</ds:SignedInfo>`)
	return b.String()
}

func buildOfficeSignaturePropertyXML(sigID, propertyID string, now time.Time, opts officeSignatureOptions) (string, error) {
	dateValue := escapeXMLText(now.Format("2006-01-02T15:04:05Z"))
	var content string
	switch {
	case strings.TrimSpace(opts.DateWrapperXML) != "":
		content = fmt.Sprintf(opts.DateWrapperXML, dateValue)
	case strings.TrimSpace(opts.DateElementName) != "":
		content = `<` + opts.DateElementName
		if ns := strings.TrimSpace(opts.DateNamespaceURI); ns != "" {
			prefix := ""
			if idx := strings.IndexByte(opts.DateElementName, ':'); idx >= 0 {
				prefix = opts.DateElementName[:idx]
			}
			if prefix != "" {
				content += ` xmlns:` + prefix + `="` + escapeXMLAttr(ns) + `"`
			} else {
				content += ` xmlns="` + escapeXMLAttr(ns) + `"`
			}
		}
		content += `>` + dateValue + `</` + opts.DateElementName + `>`
	default:
		content = dateValue
	}
	return fmt.Sprintf(`<ds:SignatureProperties xmlns:ds="%s"><ds:SignatureProperty Target="#%s" Id="%s">%s</ds:SignatureProperty></ds:SignatureProperties>`,
		nsXMLDSig,
		escapeXMLAttr(sigID),
		escapeXMLAttr(propertyID),
		content,
	), nil
}

func appendSignatureToODFEnvelope(existing []byte, signatureXML string) []byte {
	trimmed := strings.TrimSpace(string(existing))
	if idx := strings.LastIndex(strings.ToLower(trimmed), strings.ToLower(odfSignatureRootClose)); idx >= 0 {
		return []byte(trimmed[:idx] + signatureXML + trimmed[idx:])
	}
	return []byte(odfSignatureRootOpen + trimmed + signatureXML + odfSignatureRootClose)
}

type ooxmlContentTypes struct {
	XMLName   xml.Name                   `xml:"Types"`
	XMLNS     string                     `xml:"xmlns,attr,omitempty"`
	Defaults  []ooxmlContentTypeDefault  `xml:"Default"`
	Overrides []ooxmlContentTypeOverride `xml:"Override"`
}

type ooxmlContentTypeDefault struct {
	Extension   string `xml:"Extension,attr"`
	ContentType string `xml:"ContentType,attr"`
}

type ooxmlContentTypeOverride struct {
	PartName    string `xml:"PartName,attr"`
	ContentType string `xml:"ContentType,attr"`
}

func parseOOXMLContentTypes(data []byte) (ooxmlContentTypes, error) {
	out := ooxmlContentTypes{}
	if len(bytes.TrimSpace(data)) == 0 {
		return out, errors.New("content types vacío")
	}
	if err := xml.Unmarshal(data, &out); err != nil {
		return out, err
	}
	if strings.TrimSpace(out.XMLNS) == "" {
		out.XMLNS = ooxmlTypesSchema
	}
	return out, nil
}

func (ct ooxmlContentTypes) ContentTypeFor(name string) string {
	partName := "/" + strings.ReplaceAll(name, "\\", "/")
	for _, override := range ct.Overrides {
		if strings.EqualFold(strings.TrimSpace(override.PartName), partName) {
			return strings.TrimSpace(override.ContentType)
		}
	}
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(name)), ".")
	for _, def := range ct.Defaults {
		if strings.EqualFold(strings.TrimSpace(def.Extension), ext) {
			return strings.TrimSpace(def.ContentType)
		}
	}
	return ""
}

func updateOOXMLScaffolding(archive *mutableZip, signatureEntry string) error {
	contentTypesData, ok := archive.Get("[Content_Types].xml")
	if !ok {
		return errors.New("documento OOXML sin [Content_Types].xml")
	}
	ct, err := parseOOXMLContentTypes(contentTypesData)
	if err != nil {
		return err
	}
	if !containsDefaultContentType(ct.Defaults, "sigs", ooxmlOriginContentType) {
		ct.Defaults = append(ct.Defaults, ooxmlContentTypeDefault{Extension: "sigs", ContentType: ooxmlOriginContentType})
	}
	if !containsOverrideContentType(ct.Overrides, "/"+signatureEntry, ooxmlSigContentType) {
		ct.Overrides = append(ct.Overrides, ooxmlContentTypeOverride{PartName: "/" + signatureEntry, ContentType: ooxmlSigContentType})
	}
	sort.Slice(ct.Defaults, func(i, j int) bool {
		return strings.ToLower(ct.Defaults[i].Extension) < strings.ToLower(ct.Defaults[j].Extension)
	})
	sort.Slice(ct.Overrides, func(i, j int) bool {
		return strings.ToLower(ct.Overrides[i].PartName) < strings.ToLower(ct.Overrides[j].PartName)
	})
	contentTypesOut, err := xml.Marshal(ct)
	if err != nil {
		return err
	}
	archive.Put("[Content_Types].xml", append([]byte(xml.Header), contentTypesOut...))

	rootRelsData, ok := archive.Get("_rels/.rels")
	if !ok {
		return errors.New("documento OOXML sin _rels/.rels")
	}
	rootRels, err := parseOOXMLRelationships(rootRelsData)
	if err != nil {
		return err
	}
	if !hasRelationshipType(rootRels.Relationships, ooxmlOriginRelType) {
		rootRels.Relationships = append(rootRels.Relationships, ooxmlRelationship{
			ID:     nextOOXMLRelationshipID(rootRels.Relationships, "rel-id-"),
			Type:   ooxmlOriginRelType,
			Target: "_xmlsignatures/origin.sigs",
		})
	}
	rootRelsOut, err := xml.Marshal(rootRels)
	if err != nil {
		return err
	}
	archive.Put("_rels/.rels", append([]byte(xml.Header), rootRelsOut...))

	if _, ok := archive.Get("_xmlsignatures/origin.sigs"); !ok {
		archive.Put("_xmlsignatures/origin.sigs", []byte{})
	}
	originRelsEntry := "_xmlsignatures/_rels/origin.sigs.rels"
	originRelsData, ok := archive.Get(originRelsEntry)
	var originRels ooxmlRelationships
	if ok && len(bytes.TrimSpace(originRelsData)) > 0 {
		originRels, err = parseOOXMLRelationships(originRelsData)
		if err != nil {
			return err
		}
	} else {
		originRels = ooxmlRelationships{XMLNS: ooxmlRelsSchema}
	}
	target := path.Base(signatureEntry)
	if !hasRelationshipTargetAndType(originRels.Relationships, target, ooxmlSignatureRelType) {
		originRels.Relationships = append(originRels.Relationships, ooxmlRelationship{
			ID:     nextOOXMLRelationshipID(originRels.Relationships, "rel-"),
			Type:   ooxmlSignatureRelType,
			Target: target,
		})
	}
	originRelsOut, err := xml.Marshal(originRels)
	if err != nil {
		return err
	}
	archive.Put(originRelsEntry, append([]byte(xml.Header), originRelsOut...))
	return nil
}

type ooxmlRelationships struct {
	XMLName       xml.Name            `xml:"Relationships"`
	XMLNS         string              `xml:"xmlns,attr,omitempty"`
	Relationships []ooxmlRelationship `xml:"Relationship"`
}

type ooxmlRelationship struct {
	ID     string `xml:"Id,attr"`
	Type   string `xml:"Type,attr"`
	Target string `xml:"Target,attr"`
}

func nextOOXMLRelationshipID(items []ooxmlRelationship, prefix string) string {
	occupied := make(map[string]struct{}, len(items))
	for _, item := range items {
		occupied[strings.ToLower(strings.TrimSpace(item.ID))] = struct{}{}
	}
	for index := 1; ; index++ {
		candidate := fmt.Sprintf("%s%d", prefix, index)
		if _, exists := occupied[strings.ToLower(candidate)]; !exists {
			return candidate
		}
	}
}

func parseOOXMLRelationships(data []byte) (ooxmlRelationships, error) {
	rels := ooxmlRelationships{}
	if err := xml.Unmarshal(data, &rels); err != nil {
		return rels, err
	}
	if strings.TrimSpace(rels.XMLNS) == "" {
		rels.XMLNS = ooxmlRelsSchema
	}
	return rels, nil
}

func containsDefaultContentType(items []ooxmlContentTypeDefault, ext, contentType string) bool {
	for _, item := range items {
		if strings.EqualFold(item.Extension, ext) && strings.EqualFold(item.ContentType, contentType) {
			return true
		}
	}
	return false
}

func containsOverrideContentType(items []ooxmlContentTypeOverride, partName, contentType string) bool {
	for _, item := range items {
		if strings.EqualFold(item.PartName, partName) && strings.EqualFold(item.ContentType, contentType) {
			return true
		}
	}
	return false
}

func hasRelationshipType(items []ooxmlRelationship, relType string) bool {
	for _, item := range items {
		if strings.EqualFold(item.Type, relType) {
			return true
		}
	}
	return false
}

func hasRelationshipTargetAndType(items []ooxmlRelationship, target, relType string) bool {
	for _, item := range items {
		if strings.EqualFold(item.Target, target) && strings.EqualFold(item.Type, relType) {
			return true
		}
	}
	return false
}

var _ ports.SignerEngine = (*ODFDetached)(nil)
var _ ports.SignerEngine = (*OOXMLDetached)(nil)
