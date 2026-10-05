// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/signingpolicy"
)

const FormatVeriFactu domain.SignatureFormat = "VeriFactu"

type VeriFactuSigner struct{}

func NewVeriFactuSigner() *VeriFactuSigner { return &VeriFactuSigner{} }

func (s *VeriFactuSigner) Sign(ctx context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error) {
	if e := ctx.Err(); e != nil {
		return domain.SignatureResult{}, e
	}
	if job.Format != FormatVeriFactu || job.Action != domain.ActionSign {
		return domain.SignatureResult{}, vfError("profile")
	}
	data := sanitizeXMLDocument(job.Document.Content)
	n, e := vfParse(data)
	if e != nil {
		return domain.SignatureResult{}, e
	}
	if !vfRoot(n) {
		return domain.SignatureResult{}, vfError("root")
	}
	if len(vfStructure(n, true)) > 0 {
		return domain.SignatureResult{}, vfError("structure")
	}
	if looksLikeSignedXML(data) {
		return domain.SignatureResult{}, vfError("signed")
	}
	if conv, ok := key.(interface{ ToLocalSigningKey() *LocalSigningKey }); ok {
		key = conv.ToLocalSigningKey()
	}
	k, e := requireLocalSigningKeyXAdES(key)
	if e != nil {
		return domain.SignatureResult{}, vfError("key")
	}
	if e = signingpolicy.ValidateCertificateDER(k.Certificate.Raw, time.Now()); e != nil {
		return domain.SignatureResult{}, vfError("key")
	}
	date := n.value("FechaHoraHusoGenRegistro")
	if n.name.Local == "RegistroEvento" {
		date = n.value("Evento", "FechaHoraHusoGenEvento")
	}
	at, err := time.Parse(time.RFC3339Nano, date)
	if err != nil || at.Before(k.Certificate.NotBefore) || at.After(k.Certificate.NotAfter) {
		return domain.SignatureResult{}, vfError("certificate")
	}
	// Se ignoran opciones del llamante: no se puede degradar el perfil ni firmar otro nodo.
	alg, e := resolveXAdESAlgorithmOptions(map[string]string{"algorithm": "SHA256withRSA"})
	if e != nil {
		return domain.SignatureResult{}, e
	}
	opts := resolveXAdESBuildOptions(map[string]string{"expPolicy": "firmaage19"}, alg)
	opts.DocumentTransforms = []string{algEnveloped, algExcC14N}
	canon, e := canonicalizeXML(data, algExcC14N)
	if e != nil {
		return domain.SignatureResult{}, vfError("xml")
	}
	digest, e := digestBytes(alg.Hash, canon)
	if e != nil {
		return domain.SignatureResult{}, e
	}
	ids := idsXAdESNuevos()
	info, value, keyInfo, props, e := firmarPartesXAdES(k, alg, opts, ids, "", base64.StdEncoding.EncodeToString(digest), "text/xml")
	if e != nil {
		return domain.SignatureResult{}, e
	}
	sig := ensamblarFirmaXAdES(ids, opts.Namespace, info, value, keyInfo, "", props)
	var out []byte
	if n.name.Local == "RegistroEvento" {
		event := n.child(n.name.Space, "Evento")
		close := strings.LastIndex(string(data[:event.end]), "</")
		out = append(out, data[:close]...)
		out = append(out, sig...)
		out = append(out, data[close:]...)
	} else {
		out, e = insertarFirmaEnRaiz(data, sig)
	}
	if e != nil {
		return domain.SignatureResult{}, e
	}
	return domain.SignatureResult{Format: FormatVeriFactu, Data: out, Algorithm: "RSA-SHA256"}, nil
}

func vfHash(n *vfNode) (string, string) {
	base := n
	var paths [][]string
	switch n.name.Local {
	case "RegistroAlta":
		paths = [][]string{{"IDFactura", "IDEmisorFactura"}, {"IDFactura", "NumSerieFactura"}, {"IDFactura", "FechaExpedicionFactura"}, {"TipoFactura"}, {"CuotaTotal"}, {"ImporteTotal"}, {"Encadenamiento", "RegistroAnterior", "Huella"}, {"FechaHoraHusoGenRegistro"}}
	case "RegistroAnulacion":
		paths = [][]string{{"IDFactura", "IDEmisorFacturaAnulada"}, {"IDFactura", "NumSerieFacturaAnulada"}, {"IDFactura", "FechaExpedicionFacturaAnulada"}, {"Encadenamiento", "RegistroAnterior", "Huella"}, {"FechaHoraHusoGenRegistro"}}
	case "RegistroEvento":
		base = n.child(n.name.Space, "Evento")
		paths = [][]string{{"SistemaInformatico", "NIF"}, {"SistemaInformatico", "IDOtro", "ID"}, {"SistemaInformatico", "IdSistemaInformatico"}, {"SistemaInformatico", "Version"}, {"SistemaInformatico", "NumeroInstalacion"}, {"ObligadoEmision", "NIF"}, {"TipoEvento"}, {"Encadenamiento", "EventoAnterior", "HuellaEvento"}, {"FechaHoraHusoGenEvento"}}
	}
	parts := make([]string, 0, len(paths))
	for _, p := range paths {
		parts = append(parts, p[len(p)-1]+"="+base.value(p...))
	}
	input := strings.Join(parts, "&")
	sum := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%X", sum), input
}

// RecalcularHuellaVeriFactu solo calcula la huella de un registro aportado; no genera XML.
func RecalcularHuellaVeriFactu(data []byte) (string, error) {
	n, e := vfParse(data)
	if e != nil {
		return "", e
	}
	if !vfRoot(n) {
		return "", vfError("root")
	}
	h, _ := vfHash(n)
	return h, nil
}

type VeriFactuRecordResult struct {
	File           string      `json:"file"`
	Type           string      `json:"type"`
	Hash           string      `json:"hash"`
	CalculatedHash string      `json:"calculatedHash"`
	PreviousHash   string      `json:"previousHash"`
	Signed         bool        `json:"signed"`
	Valid          bool        `json:"valid"`
	Issues         []vfProblem `json:"issues"`
	root           *vfNode
	chain          string
	date           time.Time
}
type VeriFactuValidationResult struct {
	Format   string                  `json:"format"`
	Valid    bool                    `json:"valid"`
	Errors   int                     `json:"errors"`
	Warnings int                     `json:"warnings"`
	Records  []VeriFactuRecordResult `json:"records"`
	Report   string                  `json:"report"`
}

func (r *VeriFactuRecordResult) add(field, key, level string) {
	if len(r.Issues) < 64 {
		r.Issues = append(r.Issues, vfProblem{field, "verifactu." + key, level})
	}
}

func vfCheckSignature(data []byte, n *vfNode, at time.Time) []vfProblem {
	fail := func(key string) []vfProblem { return []vfProblem{{"Signature", "verifactu." + key, "error"}} }
	parts, e := extractContextualXMLSignatures(data)
	if e != nil || len(parts) != 1 {
		return fail("signature")
	}
	signatureRoot := n
	if n.name.Local == "RegistroEvento" {
		signatureRoot = n.child(n.name.Space, "Evento")
	}
	sig := signatureRoot.child(nsXMLDSig, "Signature")
	if sig == nil || !vfSignatureEnvelopeClean(sig) {
		return fail("profile")
	}
	info := sig.child(nsXMLDSig, "SignedInfo")
	if info == nil {
		return fail("profile")
	}
	method := info.child(nsXMLDSig, "SignatureMethod")
	if method == nil {
		return fail("profile")
	}
	algo := vfAttribute(method, "Algorithm")
	if algo != algRSASHA256 && algo != algRSASHA512 {
		return fail("profile")
	}
	rootRefs := 0
	propsRefs := 0
	for _, ref := range info.children {
		if ref.name.Local != "Reference" || ref.name.Space != nsXMLDSig {
			continue
		}
		digestMethod := vfAttribute(ref.child(nsXMLDSig, "DigestMethod"), "Algorithm")
		if digestMethod != algSHA256 && digestMethod != "http://www.w3.org/2001/04/xmlenc#sha512" {
			return fail("profile")
		}
		uri := vfAttribute(ref, "URI")
		if uri == "" {
			rootRefs++
			transforms := ref.child(nsXMLDSig, "Transforms")
			if transforms == nil || len(transforms.children) < 1 || len(transforms.children) > 2 || vfAttribute(transforms.children[0], "Algorithm") != algEnveloped {
				return fail("profile")
			}
			if len(transforms.children) == 2 {
				algorithm := vfAttribute(transforms.children[1], "Algorithm")
				if algorithm != algC14N && algorithm != algExcC14N {
					return fail("profile")
				}
			}
		}
		if vfAttribute(ref, "Type") == typeSignedProps || vfAttribute(ref, "Type") == typeSignedPropsV122 {
			propsRefs++
		}
	}
	if rootRefs != 1 || propsRefs != 1 {
		return fail("profile")
	}
	props := sig.child(nsXMLDSig, "Object")
	if props == nil {
		return fail("profile")
	}
	qp := props.child(nsXAdES, "QualifyingProperties")
	sp := qp.child(nsXAdES, "SignedProperties")
	// La política leída tiene que pertenecer a las propiedades efectivamente firmadas.
	bound := false
	for _, ref := range info.children {
		if ref.name.Space == nsXMLDSig && ref.name.Local == "Reference" && vfAttribute(ref, "Type") == typeSignedProps && vfAttribute(ref, "URI") == "#"+vfAttribute(sp, "Id") && vfAttribute(sp, "Id") != "" {
			bound = true
		}
	}
	if !bound || vfAttribute(qp, "Target") != "#"+vfAttribute(sig, "Id") {
		return fail("profile")
	}
	ss := sp.child(nsXAdES, "SignedSignatureProperties")
	pi := ss.child(nsXAdES, "SignaturePolicyIdentifier").child(nsXAdES, "SignaturePolicyId")
	if pi.value("SigPolicyId", "Identifier") != "urn:oid:2.16.724.1.3.1.1.2.1.9" {
		return fail("profile")
	}
	ph := pi.child(nsXAdES, "SigPolicyHash")
	if ph == nil || ph.child(nsXMLDSig, "DigestValue").value() != "G7roucf600+f03r/o0bAOQ6WAs0=" || vfAttribute(ph.child(nsXMLDSig, "DigestMethod"), "Algorithm") != algSHA1 {
		return fail("profile")
	}
	verification, e := verifyXMLSignatureDocument(data, true, nil, domain.CertificateChain{}, string(FormatVeriFactu), "", nil)
	if e != nil || len(xmlCompatibilityDetails(verification.result)) > 0 {
		return fail("signature")
	}
	for _, c := range verification.signerCertificates {
		if at.IsZero() || at.Before(c.NotBefore) || at.After(c.NotAfter) {
			return fail("certificate")
		}
	}
	// La integridad no acredita representación, cualificación TSL ni revocación histórica.
	return []vfProblem{{"Signature", "verifactu.trust", "warning"}}
}

// vfAEATNamespacePrefix agrupa los espacios de nombres de la AEAT para
// Veri*Factu (SuministroInformacion, EventosSIF y los demás de tike/cont/ws).
const vfAEATNamespacePrefix = "https://www2.agenciatributaria.gob.es/static_files/common/internet/dep/aplicaciones/es/aeat/tike/cont/ws/"

// vfSignatureEnvelopeClean comprueba que la firma no transporta registros.
// La transformación enveloped excluye todo el subárbol de ds:Signature del
// resumen del registro, así que un RegistroAlta inyectado en ds:Object no
// invalidaría la firma y otro lector podría tomarlo por el registro firmado.
// Se exige un único ds:Object con solo xades:QualifyingProperties y ningún
// elemento de los espacios de nombres de la AEAT en todo el subárbol.
func vfSignatureEnvelopeClean(sig *vfNode) bool {
	objects := 0
	for _, c := range sig.children {
		if c.name.Space != nsXMLDSig || c.name.Local != "Object" {
			continue
		}
		objects++
		if len(c.children) != 1 || c.children[0].name.Space != nsXAdES ||
			c.children[0].name.Local != "QualifyingProperties" || strings.TrimSpace(c.text.String()) != "" {
			return false
		}
	}
	if objects != 1 {
		return false
	}
	pending := []*vfNode{sig}
	for len(pending) > 0 {
		n := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if n.name.Space == VeriFactuNamespace || n.name.Space == VeriFactuEventNamespace ||
			strings.HasPrefix(n.name.Space, vfAEATNamespacePrefix) {
			return false
		}
		pending = append(pending, n.children...)
	}
	return true
}

func vfAttribute(n *vfNode, name string) string {
	if n != nil {
		for _, a := range n.attrs {
			if a.Name.Space == "" && a.Name.Local == name {
				return a.Value
			}
		}
	}
	return ""
}

// ValidarRegistrosVeriFactu revisa documentos independientes, sin acceder a la red.
func ValidarRegistrosVeriFactu(ctx context.Context, files map[string][]byte) VeriFactuValidationResult {
	result := VeriFactuValidationResult{Format: string(FormatVeriFactu), Valid: true, Records: []VeriFactuRecordResult{}}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	total := 0
	for _, name := range names {
		r := VeriFactuRecordResult{File: name, Issues: []vfProblem{}}
		data := files[name]
		total += len(data)
		if ctx.Err() != nil || len(result.Records) >= 256 || total > 32*1024*1024 {
			r.add("XML", "limit", "error")
			result.Records = append(result.Records, r)
			break
		}
		n, e := vfParse(data)
		if e != nil || !vfRoot(n) {
			if p, ok := e.(vfProblem); ok {
				p.Field, p.Level = "XML", "error"
				r.Issues = append(r.Issues, p)
			} else {
				r.add("XML", "root", "error")
			}
			result.Records = append(result.Records, r)
			continue
		}
		r.root = n
		r.Type = n.name.Local
		r.Issues = vfStructure(n, false)
		if len(r.Issues) > 64 {
			r.Issues = r.Issues[:64]
		}
		b := n
		hashField := "Huella"
		dateField := "FechaHoraHusoGenRegistro"
		prev := "RegistroAnterior"
		first := "PrimerRegistro"
		if r.Type == "RegistroEvento" {
			b = n.child(n.name.Space, "Evento")
			hashField = "HuellaEvento"
			dateField = "FechaHoraHusoGenEvento"
			prev = "EventoAnterior"
			first = "PrimerEvento"
		}
		r.Hash = b.value(hashField)
		r.CalculatedHash, _ = vfHash(n)
		r.PreviousHash = b.value("Encadenamiento", prev, hashField)
		if !vfDigest.MatchString(r.Hash) {
			r.add(hashField, "value", "error")
		}
		if r.PreviousHash != "" && !vfDigest.MatchString(r.PreviousHash) {
			r.add("Encadenamiento/"+prev+"/"+hashField, "value", "error")
		}
		r.date, _ = time.Parse(time.RFC3339Nano, b.value(dateField))
		if !vfDateTime(b.value(dateField)) {
			r.add(dateField, "value", "error")
		}
		if r.Hash != r.CalculatedHash {
			r.add(hashField, "hash", "error")
		}
		if b.value("TipoHuella") != "01" {
			r.add("TipoHuella", "value", "error")
		}
		if r.PreviousHash == "" && b.value("Encadenamiento", first) != "S" {
			r.add("Encadenamiento", "chain", "error")
		}
		obligated := n.value("IDFactura", "IDEmisorFactura")
		if r.Type == "RegistroAnulacion" {
			obligated = n.value("IDFactura", "IDEmisorFacturaAnulada")
		}
		if r.Type == "RegistroEvento" {
			obligated = b.value("ObligadoEmision", "NIF")
		}
		system := b.child(n.name.Space, "SistemaInformatico")
		r.chain = strings.Join([]string{obligated, system.value("NIF"), system.value("IDOtro", "ID"), system.value("IdSistemaInformatico"), system.value("NumeroInstalacion")}, "\x00")
		if r.Type == "RegistroEvento" {
			r.chain += "\x00event"
		}
		r.Signed = looksLikeSignedXML(data)
		if r.Signed {
			r.Issues = append(r.Issues, vfCheckSignature(data, n, r.date)...)
		} else {
			r.add("Signature", "unsigned", "warning")
		}
		result.Records = append(result.Records, r)
	}
	byHash := map[string]int{}
	children := map[string]int{}
	starts := map[string]int{}
	for i := range result.Records {
		r := &result.Records[i]
		if r.root == nil {
			continue
		}
		key := r.chain + "\x00" + r.Hash
		if _, exists := byHash[key]; exists {
			r.add("Huella", "duplicate", "error")
		}
		byHash[key] = i
		if r.PreviousHash == "" {
			starts[r.chain]++
			if starts[r.chain] > 1 {
				r.add("Encadenamiento", "chain", "error")
			}
		} else {
			p := r.chain + "\x00" + r.PreviousHash
			children[p]++
			if children[p] > 1 {
				r.add("Encadenamiento", "chain", "error")
			}
		}
	}
	for i := range result.Records {
		r := &result.Records[i]
		if r.root == nil || r.PreviousHash == "" {
			continue
		}
		visited := map[int]bool{i: true}
		cursor := r
		for cursor.PreviousHash != "" {
			index, found := byHash[cursor.chain+"\x00"+cursor.PreviousHash]
			if !found {
				break
			}
			if visited[index] {
				r.add("Encadenamiento", "chain", "error")
				break
			}
			visited[index] = true
			cursor = &result.Records[index]
		}
		j, ok := byHash[r.chain+"\x00"+r.PreviousHash]
		if !ok {
			// Con un inicio declarado en la selección, un enlace que no llega
			// a él deja la cadena aportada incompleta. Sin inicio puede tratarse
			// de un fragmento exportado: se informa sin afirmar su continuidad.
			level := "warning"
			if starts[r.chain] > 0 {
				level = "error"
			}
			r.add("Encadenamiento", "previous_missing", level)
			continue
		}
		prev := &result.Records[j]
		if j == i || r.date.Before(prev.date) {
			r.add("Encadenamiento", "chain", "error")
		}
		b := r.root
		if r.Type == "RegistroEvento" {
			b = b.child(b.name.Space, "Evento")
			if b.value("Encadenamiento", "EventoAnterior", "TipoEvento") != prev.root.value("Evento", "TipoEvento") || b.value("Encadenamiento", "EventoAnterior", "FechaHoraHusoGenEvento") != prev.root.value("Evento", "FechaHoraHusoGenEvento") {
				r.add("EventoAnterior", "chain", "error")
			}
		} else {
			suffix := ""
			if prev.Type == "RegistroAnulacion" {
				suffix = "Anulada"
			}
			for _, field := range []string{"IDEmisorFactura", "NumSerieFactura", "FechaExpedicionFactura"} {
				if b.value("Encadenamiento", "RegistroAnterior", field) != prev.root.value("IDFactura", field+suffix) {
					r.add("RegistroAnterior/"+field, "chain", "error")
				}
			}
		}
	}
	for i := range result.Records {
		r := &result.Records[i]
		// Ya validados y encadenados, los valores que se muestran pierden los
		// caracteres de control y de formato (Bidi, anchura cero...).
		r.File = vfTextoVisible(r.File, 260)
		r.Type = vfTextoVisible(r.Type, 64)
		r.Hash = vfTextoVisible(r.Hash, 128)
		r.PreviousHash = vfTextoVisible(r.PreviousHash, 128)
		r.Valid = true
		for _, p := range r.Issues {
			if p.Level == "error" {
				result.Errors++
				r.Valid = false
				result.Valid = false
			} else {
				result.Warnings++
			}
		}
	}
	if len(result.Records) == 0 {
		result.Valid = false
		result.Errors++
	}
	return result
}

// vfTextoVisible quita los caracteres de control (Cc) y de formato (Cf) y
// recorta el texto a maxRunas para mostrarlo en un informe.
func vfTextoVisible(s string, maxRunas int) string {
	limpio := strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
	if runas := []rune(limpio); len(runas) > maxRunas {
		limpio = string(runas[:maxRunas])
	}
	return limpio
}

// vfDetalleTecnico acompaña en el informe a los mensajes en lenguaje llano con
// el detalle técnico (elementos, perfil de firma, huella o encadenamiento).
var vfDetalleTecnico = map[string]string{
	"verifactu.root":      "verifactu.root_detail",
	"verifactu.profile":   "verifactu.profile_detail",
	"verifactu.hash":      "verifactu.hash_detail",
	"verifactu.chain":     "verifactu.chain_detail",
	"verifactu.signature": "verifactu.signature_detail",
}

func (r *VeriFactuValidationResult) Localize(t func(string) string) {
	var b strings.Builder
	fmt.Fprintln(&b, t("verifactu.scope"))
	if len(r.Records) == 0 {
		fmt.Fprintln(&b, t("verifactu.empty"))
	}
	for _, record := range r.Records {
		fmt.Fprintf(&b, "\n%s [%s]\n", vfTextoVisible(filepath.Base(record.File), 260), vfTextoVisible(record.Type, 64))
		fmt.Fprintf(&b, "%s: %s\n", t("verifactu.hash_label"), record.CalculatedHash)
		for _, p := range record.Issues {
			fmt.Fprintf(&b, "%s: %s\n", p.Field, t(p.Key))
			if detalle, ok := vfDetalleTecnico[p.Key]; ok {
				fmt.Fprintf(&b, "    %s\n", t(detalle))
			}
		}
		if len(record.Issues) == 0 {
			fmt.Fprintln(&b, t("verifactu.valid"))
		}
	}
	r.Report = b.String()
}

// ValidarRutaVeriFactu acota la carpeta, los ficheros y el volumen agregado.
func ValidarRutaVeriFactu(ctx context.Context, path string) (VeriFactuValidationResult, error) {
	info, e := os.Lstat(path)
	if e != nil {
		return VeriFactuValidationResult{}, vfError("input")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return VeriFactuValidationResult{}, vfError("input")
	}
	paths := []string{path}
	if info.IsDir() {
		d, e := securefile.OpenDir(path)
		if e != nil {
			return VeriFactuValidationResult{}, vfError("input")
		}
		entries, e := d.ReadDir(257)
		d.Close()
		if e != nil && len(entries) == 0 {
			return VeriFactuValidationResult{}, vfError("input")
		}
		if len(entries) > 256 {
			return VeriFactuValidationResult{}, vfError("limit")
		}
		paths = nil
		for _, entry := range entries {
			if entry.Type().IsRegular() && strings.EqualFold(filepath.Ext(entry.Name()), ".xml") {
				paths = append(paths, filepath.Join(path, entry.Name()))
			}
		}
	}
	files := map[string][]byte{}
	total := 0
	for _, p := range paths {
		if e = ctx.Err(); e != nil {
			return VeriFactuValidationResult{}, e
		}
		data, e := securefile.ReadFileLimit(p, VeriFactuMaxXMLBytes)
		if e != nil {
			return VeriFactuValidationResult{}, vfError("input")
		}
		total += len(data)
		if total > 32*1024*1024 {
			return VeriFactuValidationResult{}, vfError("limit")
		}
		files[p] = data
	}
	return ValidarRegistrosVeriFactu(ctx, files), nil
}
