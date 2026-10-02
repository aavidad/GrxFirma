// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build ignore

// Regenera el corpus sintético local: go run scripts/generar_dictamen_v2.go
// Las claves son exclusivamente de prueba, públicas y no aptas para producción.
package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	pdf "github.com/digitorus/pdf"
	pdfsign "github.com/digitorus/pdfsign/sign"
	restin "grxfirma/internal/adapters/inbound/common/rest"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/common/verificacionlocal"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/pdffixture"
)

const corpus = "testdata/dictamen-v2"

var fixedDate = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
var notBefore = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
var notAfter = time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC)

type identity struct {
	key  *rsa.PrivateKey
	cert *x509.Certificate
}

func main() {
	root := issue("raiz", 1, nil, true)
	intermediate := issue("intermedia", 2, &root, true)
	alice := issue("firmante-a", 3, &intermediate, false)
	bob := issue("firmante-b", 4, &intermediate, false)
	write("pki/raiz.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: root.cert.Raw}))
	write("pki/intermedia.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: intermediate.cert.Raw}))
	write("pki/firmante-a.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: alice.cert.Raw}))
	write("pki/firmante-b.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: bob.cert.Raw}))
	write("pki/crl-buena/raiz.crl", createCRL(root, nil))
	write("pki/crl-buena/intermedia.crl", createCRL(intermediate, nil))
	write("pki/crl-b-revocado/raiz.crl", createCRL(root, nil))
	write("pki/crl-b-revocado/intermedia.crl", createCRL(intermediate, bob.cert.SerialNumber))

	original := pdffixture.Minimal()
	one := signPDF(original, alice, intermediate, pdfsign.ApprovalSignature, 0, false)
	two := signPDF(one, bob, intermediate, pdfsign.ApprovalSignature, 0, false)
	writeCase("01_una_firma", original, one, "pki/crl-buena", "valida")
	writeCase("02_dos_firmas", original, two, "pki/crl-buena", "valida")
	between := appendPageContent(one, "TEXTO CAMBIADO ENTRE FIRMAS")
	betweenSigned := signPDF(between, bob, intermediate, pdfsign.ApprovalSignature, 0, false)
	writeCase("03_pagina_entre_firmas", original, betweenSigned, "pki/crl-buena", "no_valida")
	writeCase("04_pagina_posterior", original, appendPageContent(two, "TEXTO AÑADIDO DESPUÉS"), "pki/crl-buena", "no_valida")
	certified := signPDF(original, alice, intermediate, pdfsign.CertificationSignature, pdfsign.DoNotAllowAnyChangesPerms, false)
	certifiedThenSigned := signPDF(certified, bob, intermediate, pdfsign.ApprovalSignature, 0, false)
	writeCase("05_docmdp_1", original, certifiedThenSigned, "pki/crl-buena", "no_valida")
	wrongOriginal := bytes.Replace(original, []byte("valid PDF fixture"), []byte("other PDF fixture"), 1)
	writeCase("06_original_ajeno", wrongOriginal, two, "pki/crl-buena", "indeterminada")
	writeCase("07_byterange_incompleto", original, truncateByteRange(one, alice, intermediate), "pki/crl-buena", "no_valida")
	writeCase("08_certificado_revocado", original, two, "pki/crl-b-revocado", "no_valida")
	writeCase("09_xref_stream", original, appendXRefStream(two, false), "pki/crl-buena", "indeterminada")
	writeCase("10_xref_hibrido", original, appendXRefStream(two, true), "pki/crl-buena", "indeterminada")
	locked := signPDF(original, alice, intermediate, pdfsign.ApprovalSignature, 0, true)
	writeCase("11_fieldmdp_all", original, signPDF(locked, bob, intermediate, pdfsign.ApprovalSignature, 0, false), "pki/crl-buena", "no_valida")
	writeCase("12_dss", original, appendDSS(two), "pki/crl-buena", "indeterminada")
	writeCase("13_doctimestamp", original, appendDocumentTimestamp(two), "pki/crl-buena", "indeterminada")
	certifiedP2 := signPDF(original, alice, intermediate, pdfsign.CertificationSignature, pdfsign.AllowFillingExistingFormFieldsAndSignaturesPerms, false)
	writeCase("14_docmdp_2", original, signPDF(certifiedP2, bob, intermediate, pdfsign.ApprovalSignature, 0, false), "pki/crl-buena", "valida")
	certifiedP3 := signPDF(original, alice, intermediate, pdfsign.CertificationSignature, pdfsign.AllowFillingExistingFormFieldsAndSignaturesAndCRUDAnnotationsPerms, false)
	writeCase("15_docmdp_3", original, signPDF(certifiedP3, bob, intermediate, pdfsign.ApprovalSignature, 0, false), "pki/crl-buena", "valida")
	fmt.Println("Corpus v2 generado en", corpus)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func write(relative string, data []byte) {
	path := filepath.Join(corpus, relative)
	must(os.MkdirAll(filepath.Dir(path), 0o755))
	must(os.WriteFile(path, data, 0o644))
}
func loadKey(name string) *rsa.PrivateKey {
	data, err := os.ReadFile(filepath.Join(corpus, "pki", name+".key.pem"))
	must(err)
	block, _ := pem.Decode(data)
	if block == nil {
		panic("clave PEM inválida")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	must(err)
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		panic("clave RSA requerida")
	}
	return key
}
func issue(name string, serial int64, parent *identity, ca bool) identity {
	key := loadKey(name)
	template := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "GrxFirma v2 sintética " + name}, NotBefore: notBefore, NotAfter: notAfter,
		BasicConstraintsValid: true, IsCA: ca, SubjectKeyId: []byte{byte(serial), 0x56, 0x32},
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment}
	if ca {
		template.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	}
	if !ca {
		template.CRLDistributionPoints = []string{"http://crl.invalid/intermedia.crl"}
	}
	issuer, signer := template, crypto.Signer(key)
	if parent != nil {
		issuer, signer = parent.cert, parent.key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, issuer, &key.PublicKey, signer)
	must(err)
	cert, err := x509.ParseCertificate(der)
	must(err)
	return identity{key, cert}
}
func createCRL(issuer identity, revoked *big.Int) []byte {
	list := &x509.RevocationList{Number: big.NewInt(1), ThisUpdate: notBefore, NextUpdate: notAfter}
	if revoked != nil {
		list.RevokedCertificateEntries = []x509.RevocationListEntry{{SerialNumber: revoked, RevocationTime: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}}
	}
	der, err := x509.CreateRevocationList(rand.Reader, list, issuer.cert, issuer.key)
	must(err)
	return der
}
func signPDF(input []byte, signer, issuer identity, kind pdfsign.CertType, permission pdfsign.DocMDPPerm, lock bool) []byte {
	dir, err := os.MkdirTemp("", "grxfirma-v2-sign-")
	must(err)
	defer os.RemoveAll(dir)
	in, out := filepath.Join(dir, "in.pdf"), filepath.Join(dir, "out.pdf")
	must(os.WriteFile(in, input, 0o600))
	must(pdfsign.SignFile(in, out, pdfsign.SignData{Signature: pdfsign.SignDataSignature{Info: pdfsign.SignDataSignatureInfo{Name: signer.cert.Subject.CommonName, Date: fixedDate}, CertType: kind, DocMDPPerm: permission, FieldMDPAll: lock, SubFilter: pdfsign.SignatureSubFilterETSICAdESDetached}, Signer: signer.key, DigestAlgorithm: crypto.SHA256, Certificate: signer.cert, CertificateChains: [][]*x509.Certificate{{signer.cert, issuer.cert}}}))
	data, err := os.ReadFile(out)
	must(err)
	return data
}
func xrefInfo(data []byte) (root uint32, generation uint16, size int64, previous int64) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	must(err)
	root, generation = r.Trailer().Key("Root").ObjectReference()
	return root, generation, r.Trailer().Key("Size").Int64(), r.XrefInformation.StartPos
}
func appendPageContent(input []byte, message string) []byte {
	root, generation, size, prev := xrefInfo(input)
	var out bytes.Buffer
	out.Write(input)
	offset := out.Len()
	stream := "BT /F1 12 Tf 72 700 Td (" + message + ") Tj ET"
	fmt.Fprintf(&out, "4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n4 1\n%010d 00000 n \ntrailer\n<< /Size %d /Root %d %d R /Prev %d >>\nstartxref\n%d\n%%%%EOF\n", offset, size, root, generation, prev, xref)
	return out.Bytes()
}
func appendXRefStream(input []byte, hybrid bool) []byte {
	root, generation, size, prev := xrefInfo(input)
	var out bytes.Buffer
	out.Write(input)
	offset := out.Len()
	object := size
	entry := make([]byte, 7)
	entry[0] = 1
	binary.BigEndian.PutUint32(entry[1:5], uint32(offset))
	fmt.Fprintf(&out, "%d 0 obj\n<< /Type /XRef /Size %d /Root %d %d R /W [1 4 2] /Index [%d 1] /Length 7", object, size+1, root, generation, object)
	if !hybrid {
		fmt.Fprintf(&out, " /Prev %d", prev)
	}
	out.WriteString(" >>\nstream\n")
	out.Write(entry)
	out.WriteString("\nendstream\nendobj\n")
	start := offset
	if hybrid {
		start = out.Len()
		fmt.Fprintf(&out, "xref\n0 1\n0000000000 65535 f \ntrailer\n<< /Size %d /Root %d %d R /Prev %d /XRefStm %d >>\n", size+1, root, generation, prev, offset)
	}
	fmt.Fprintf(&out, "startxref\n%d\n%%%%EOF\n", start)
	return out.Bytes()
}
func appendDSS(input []byte) []byte {
	root, generation, size, prev := xrefInfo(input)
	r, err := pdf.NewReader(bytes.NewReader(input), int64(len(input)))
	must(err)
	catalog := r.Trailer().Key("Root").String()
	index := strings.LastIndex(catalog, ">>")
	if index < 0 {
		panic("catalogo inválido")
	}
	catalog = catalog[:index] + fmt.Sprintf(" /DSS %d 0 R ", size) + catalog[index:]
	var out bytes.Buffer
	out.Write(input)
	dssOffset := out.Len()
	fmt.Fprintf(&out, "%d 0 obj\n<< /Type /DSS /Certs [] /CRLs [] /OCSPs [] >>\nendobj\n", size)
	catOffset := out.Len()
	fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", size+1, catalog)
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n%d 2\n%010d 00000 n \n%010d 00000 n \ntrailer\n<< /Size %d /Root %d 0 R /Prev %d >>\nstartxref\n%d\n%%%%EOF\n", size, dssOffset, catOffset, size+2, size+1, prev, xref)
	_ = root
	_ = generation
	return out.Bytes()
}
func appendDocumentTimestamp(input []byte) []byte {
	root, generation, size, prev := xrefInfo(input)
	var out bytes.Buffer
	out.Write(input)
	offset := out.Len()
	fmt.Fprintf(&out, "%d 0 obj\n<< /Type /DocTimeStamp /SubFilter /ETSI.RFC3161 /ByteRange [0 0 0 0] /Contents <3000> >>\nendobj\n", size)
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n%d 1\n%010d 00000 n \ntrailer\n<< /Size %d /Root %d %d R /Prev %d >>\nstartxref\n%d\n%%%%EOF\n", size, offset, size+1, root, generation, prev, xref)
	return out.Bytes()
}

var byteRangeFinal = regexp.MustCompile(`/ByteRange\s*\[\s*0\s+([0-9]+)\s+([0-9]+)\s+([0-9]+)\s*\]`)
var contentsHex = regexp.MustCompile(`/Contents\s*<([0-9A-Fa-f]+)>`)

func truncateByteRange(input []byte, signer, issuer identity) []byte {
	output := append([]byte(nil), input...)
	br := byteRangeFinal.FindSubmatchIndex(output)
	if br == nil {
		panic("ByteRange ausente")
	}
	first, _ := strconv.Atoi(string(output[br[2]:br[3]]))
	second, _ := strconv.Atoi(string(output[br[4]:br[5]]))
	last, _ := strconv.Atoi(string(output[br[6]:br[7]]))
	if last < 20 {
		panic("ByteRange demasiado corto")
	}
	replacement := fmt.Sprintf("%0*d", br[7]-br[6], last-10)
	copy(output[br[6]:br[7]], replacement)
	content := append(append([]byte(nil), output[:first]...), output[second:second+last-10]...)
	doc, err := domain.NewDocument("contenido.bin", content, "application/octet-stream")
	must(err)
	res, err := commonsigner.NewCAdESBESDetached().Sign(context.Background(), domain.SignatureJob{Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign}, &commonsigner.LocalSigningKey{ID: "corpus", Signer: signer.key, Certificate: signer.cert, Chain: []*x509.Certificate{issuer.cert}})
	must(err)
	hexCMS := hex.EncodeToString(res.Data)
	match := contentsHex.FindSubmatchIndex(output)
	if match == nil {
		panic("Contents ausente")
	}
	if len(hexCMS) > match[3]-match[2] {
		panic("CMS no cabe en Contents")
	}
	copy(output[match[2]:match[3]], hexCMS+strings.Repeat("0", match[3]-match[2]-len(hexCMS)))
	return output
}
func writeCase(name string, original, signed []byte, crlDir, expectedState string) {
	write(name+"/original.pdf", original)
	write(name+"/firmado.pdf", signed)
	anchors, err := verificacionlocal.CargarAnclas(filepath.Join(corpus, "pki/raiz.pem"))
	must(err)
	crl, err := verificacionlocal.NuevoAlmacenCRL(filepath.Join(corpus, crlDir))
	must(err)
	uc := application.NuevoVerifySignatureUseCase(anchors, commonsigner.NewMultiVerifierOffline(), nil).ConEvaluador(verificacionlocal.Nuevo(verificacionlocal.Configuracion{CRL: crl}))
	adapter := restin.New(nil, uc, nil).WithBearerToken("token-sintetico-de-corpus")
	request, _ := json.Marshal(map[string]string{"content_base64": base64.StdEncoding.EncodeToString(signed), "original_content_base64": base64.StdEncoding.EncodeToString(original), "contrato_solicitado": "autofirmav2.dictamen-verificacion.v2"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v2/verify", bytes.NewReader(request))
	req.Header.Set("Authorization", "Bearer token-sintetico-de-corpus")
	adapter.RoutesSoloVerificacion().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		panic(fmt.Sprintf("%s: HTTP %d %s", name, rec.Code, rec.Body.String()))
	}
	var response struct {
		Dictamen json.RawMessage `json:"dictamen"`
	}
	must(json.Unmarshal(rec.Body.Bytes(), &response))
	var state struct {
		Estado string `json:"estado"`
		Motivo string `json:"motivo"`
	}
	must(json.Unmarshal(response.Dictamen, &state))
	if state.Estado != expectedState {
		panic(fmt.Sprintf("%s: esperado %s, obtenido %s/%s", name, expectedState, state.Estado, state.Motivo))
	}
	var formatted bytes.Buffer
	must(json.Indent(&formatted, response.Dictamen, "", "  "))
	formatted.WriteByte('\n')
	write(name+"/dictamen-esperado.json", formatted.Bytes())
}
