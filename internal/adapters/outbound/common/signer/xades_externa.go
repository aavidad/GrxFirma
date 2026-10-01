// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"crypto"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"grxfirma/internal/domain"
)

// XAdES Externally Detached de AutoFirma Java: la firma referencia datos que
// no contiene (parámetro "uri") mediante su huella. Con useManifest=true las
// referencias (uri o uri1/md1, uri2/md2...) van dentro de un ds:Manifest y la
// firma cubre ese Manifest (Navarra la usa así). Si se indica
// precalculatedHashAlgorithm, los datos recibidos son ya la huella.

const typeXMLDSigManifest = "http://www.w3.org/2000/09/xmldsig#Manifest"

type referenciaExterna struct {
	uri    string
	digest []byte
}

func buildXAdESExterna(job domain.SignatureJob, key *LocalSigningKey, ids xadesIDs, algOpts xadesAlgorithmOptions, buildOpts xadesBuildOptions, mimeType string) ([]byte, string, error) {
	hashRef, digestMethodRef, err := hashReferenciaExterna(job.Options, algOpts)
	if err != nil {
		return nil, "", err
	}
	refs, err := referenciasExternas(job, hashRef)
	if err != nil {
		return nil, "", err
	}
	usarManifest := strings.EqualFold(strings.TrimSpace(valorOpcion(job.Options, "useManifest")), "true")
	if !usarManifest && len(refs) != 1 {
		return nil, "", errors.New("XAdES Externally Detached sin manifest admite una única referencia (uri)")
	}

	var (
		documentURI string
		docDigest   []byte
		objectXML   string
	)
	if usarManifest {
		manifestID := ids.sig + "-Manifest"
		var b strings.Builder
		fmt.Fprintf(&b, `<ds:Manifest xmlns:ds="%s" Id="%s">`, nsXMLDSig, manifestID)
		for i, r := range refs {
			fmt.Fprintf(&b, `<ds:Reference Id="%s-%d" URI="%s"><ds:DigestMethod Algorithm="%s"/><ds:DigestValue>%s</ds:DigestValue></ds:Reference>`,
				ids.reference, i+1, escapeXMLAttr(r.uri), digestMethodRef, base64.StdEncoding.EncodeToString(r.digest))
		}
		b.WriteString(`</ds:Manifest>`)
		manifest := b.String()
		canonical, err := canonicalizeXML([]byte(manifest), algC14N)
		if err != nil {
			return nil, "", fmt.Errorf("error canonicalizando el Manifest: %w", err)
		}
		if docDigest, err = digestBytes(algOpts.Hash, canonical); err != nil {
			return nil, "", err
		}
		objectXML = fmt.Sprintf(`<ds:Object Id="%s-ManifestObject">%s</ds:Object>`, ids.sig, manifest)
		documentURI = "#" + manifestID
		buildOpts.DocumentTransforms = []string{algC14N}
		buildOpts.DocumentReferenceType = typeXMLDSigManifest
	} else {
		// Referencia externa directa: sin transformaciones ni tipo, como Java.
		if hashRef != algOpts.Hash {
			return nil, "", errors.New("la huella externa debe usar el algoritmo de la firma")
		}
		documentURI = refs[0].uri
		docDigest = refs[0].digest
		buildOpts.OmitDocumentTransform = true
	}

	signedInfoXML, sigValueB64, keyInfoXML, signedPropsXML, err := firmarPartesXAdES(
		key, algOpts, buildOpts, ids, documentURI, base64.StdEncoding.EncodeToString(docDigest), mimeType)
	if err != nil {
		return nil, "", err
	}
	firma := ensamblarFirmaXAdES(ids, buildOpts.Namespace, signedInfoXML, sigValueB64, keyInfoXML, objectXML, signedPropsXML)
	out := `<?xml version="1.0" encoding="UTF-8"?><AFIRMA Id="` + nuevoIDXAdES("AfirmaRoot") + `">` + firma + `</AFIRMA>`
	return []byte(out), buildOpts.Algorithm.Label, nil
}

// hashReferenciaExterna resuelve el algoritmo de las huellas externas:
// precalculatedHashAlgorithm si se indica o, si no, el de la firma.
func hashReferenciaExterna(options map[string]string, algOpts xadesAlgorithmOptions) (crypto.Hash, string, error) {
	raw := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(valorOpcion(options, "precalculatedHashAlgorithm")), "-", ""))
	switch raw {
	case "":
		return algOpts.Hash, algOpts.DigestMethod, nil
	case "SHA256":
		return crypto.SHA256, algSHA256, nil
	case "SHA384":
		return crypto.SHA384, algSHA384, nil
	case "SHA512":
		return crypto.SHA512, algSHA512, nil
	default:
		return 0, "", fmt.Errorf("algoritmo de huella precalculada no admitido: %s", raw)
	}
}

func referenciasExternas(job domain.SignatureJob, hash crypto.Hash) ([]referenciaExterna, error) {
	precalculada := strings.TrimSpace(valorOpcion(job.Options, "precalculatedHashAlgorithm")) != ""
	if uri := strings.TrimSpace(valorOpcion(job.Options, "uri")); uri != "" {
		digest := job.Document.Content
		if !precalculada {
			var err error
			if digest, err = digestBytes(hash, digest); err != nil {
				return nil, err
			}
		}
		if len(digest) != hash.Size() {
			return nil, fmt.Errorf("la huella recibida no corresponde a %s", hash)
		}
		return []referenciaExterna{{uri: uri, digest: digest}}, nil
	}
	var refs []referenciaExterna
	for i := 1; i <= 1000; i++ {
		uri := strings.TrimSpace(valorOpcion(job.Options, "uri"+strconv.Itoa(i)))
		if uri == "" {
			break
		}
		md, err := base64.StdEncoding.DecodeString(strings.TrimSpace(valorOpcion(job.Options, "md"+strconv.Itoa(i))))
		if err != nil || len(md) != hash.Size() {
			return nil, fmt.Errorf("huella de la referencia %d del manifest ausente o inválida", i)
		}
		refs = append(refs, referenciaExterna{uri: uri, digest: md})
	}
	if len(refs) == 0 {
		return nil, errors.New("XAdES Externally Detached requiere el parámetro uri (o uri1/md1...)")
	}
	return refs, nil
}
