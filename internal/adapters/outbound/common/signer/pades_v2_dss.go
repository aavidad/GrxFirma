// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"crypto/x509"
	"encoding/asn1"
	"io"
	"strings"

	pdf "github.com/digitorus/pdf"
	"golang.org/x/crypto/ocsp"
)

const maxDSSV2Streams = 128
const maxDSSV2StreamBytes = 1 << 20
const maxDSSV2TotalBytes = 8 << 20
const maxDSSV2VRI = 128

// comprobarDSSV2 solo admite una subred de objetos de validación alcanzable
// desde el catálogo nuevo. Cada referencia y cada byte decodificado tiene cota.
func comprobarDSSV2(data []byte, before, after pdf.XRefSnapshot, changed []uint32) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	if before.RootObject == 0 || after.RootObject == 0 ||
		before.RootGeneration != after.RootGeneration && before.RootObject == after.RootObject {
		return false
	}
	oldCatalog, oldOK := parsePDFObjectDict(cuerpoActivoPDF(data, before, before.RootObject))
	newCatalog, newOK := parsePDFObjectDict(cuerpoActivoPDF(data, after, after.RootObject))
	if !oldOK || !newOK || oldCatalog["/DSS"] == newCatalog["/DSS"] ||
		changedKeysOutside(oldCatalog, newCatalog, "/DSS", "/Extensions", "/Version") != "" {
		return false
	}
	r, err := pdf.NewReader(bytes.NewReader(data[:after.Length]), int64(after.Length))
	if err != nil {
		return false
	}
	dss := r.Trailer().Key("Root").Key("DSS")
	dssNumber, _ := dss.ObjectReference()
	if dss.Kind() != pdf.Dict {
		return false
	}
	allowed := map[uint32]bool{after.RootObject: true}
	var dssDict map[string]string
	var valid bool
	if ref, indirect := pdfSingleRef(newCatalog["/DSS"]); indirect {
		if dssNumber == 0 || !mismoObjetoPDF(ref, dssNumber) {
			return false
		}
		allowed[dssNumber] = true
		dssDict, valid = parsePDFObjectDict(cuerpoActivoPDF(data, after, dssNumber))
	} else if dssNumber == after.RootObject {
		dssDict, valid = parsePDFDictValue(newCatalog["/DSS"])
	} else {
		return false
	}
	if !valid || !onlyDSSKeys(dssDict, "/Type", "/Certs", "/CRLs", "/OCSPs", "/VRI") {
		return false
	}
	if kind := pdfName(dssDict["/Type"]); kind != "" && kind != "/DSS" {
		return false
	}
	streams, total := 0, 0
	for _, key := range []string{"Certs", "CRLs", "OCSPs"} {
		if !checkDSSArray(dss.Key(key), key, after, allowed, &streams, &total) {
			return false
		}
	}
	vri := dss.Key("VRI")
	if !vri.IsNull() {
		if vri.Kind() != pdf.Dict || len(vri.Keys()) > maxDSSV2VRI {
			return false
		}
		vriNumber, _ := vri.ObjectReference()
		if vriNumber != 0 {
			allowed[vriNumber] = true
		}
		for _, key := range vri.Keys() {
			item := vri.Key(key)
			n, _ := item.ObjectReference()
			if item.Kind() != pdf.Dict {
				return false
			}
			var dict map[string]string
			var good bool
			if n != 0 && n != vriNumber {
				allowed[n] = true
				dict, good = parsePDFObjectDict(cuerpoActivoPDF(data, after, n))
			} else {
				dict, good = parsePDFDictValue(item.String())
			}
			if !good || !onlyDSSKeys(dict, "/Type", "/Cert", "/CRL", "/OCSP", "/Certs", "/CRLs", "/OCSPs", "/TU", "/TS") {
				return false
			}
			if kind := pdfName(dict["/Type"]); kind != "" && kind != "/VRI" {
				return false
			}
			for _, category := range []string{"Certs", "CRLs", "OCSPs"} {
				if !checkDSSArray(item.Key(category), category, after, allowed, &streams, &total) ||
					!checkDSSArray(item.Key(strings.TrimSuffix(category, "s")), category, after, allowed, &streams, &total) {
					return false
				}
			}
		}
	}
	protectedDSSV2 := objetosDePaginaPDF(data[:before.Length])
	if !addDSSFieldObjects(data[:before.Length], protectedDSSV2) {
		return false
	}
	oldDSSV2, oldOK := oldDSSObjectsV2(data[:before.Length])
	if !oldOK {
		return false
	}
	for _, n := range changed {
		if protectedDSSV2[n] {
			return false
		}
		if old := before.Entries[n]; old.Active && n != after.RootObject && !oldDSSV2[n] {
			return false
		}
		if allowed[n] {
			continue
		}
		body := cuerpoActivoPDF(data, after, n)
		dict, valid := parsePDFObjectDict(body)
		if !valid || pdfName(dict["/Type"]) != "/XRef" {
			return false
		}
	}
	return true
}

func oldDSSObjectsV2(data []byte) (map[uint32]bool, bool) {
	result := make(map[uint32]bool)
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, false
	}
	dss := r.Trailer().Key("Root").Key("DSS")
	if dss.IsNull() {
		return result, true
	}
	if dss.Kind() != pdf.Dict {
		return nil, false
	}
	mark := func(value pdf.Value) {
		if n, _ := value.ObjectReference(); n != 0 {
			result[n] = true
		}
	}
	mark(dss)
	visitArrays := func(v pdf.Value) bool {
		if v.IsNull() {
			return true
		}
		if v.Kind() != pdf.Array || v.Len() > maxDSSV2Streams {
			return false
		}
		mark(v)
		for i := 0; i < v.Len(); i++ {
			mark(v.Index(i))
		}
		return true
	}
	for _, category := range []string{"Certs", "CRLs", "OCSPs"} {
		if !visitArrays(dss.Key(category)) {
			return nil, false
		}
	}
	vri := dss.Key("VRI")
	if !vri.IsNull() {
		if vri.Kind() != pdf.Dict || len(vri.Keys()) > maxDSSV2VRI {
			return nil, false
		}
		mark(vri)
		for _, name := range vri.Keys() {
			item := vri.Key(name)
			if item.Kind() != pdf.Dict {
				return nil, false
			}
			mark(item)
			for _, category := range []string{"Certs", "CRLs", "OCSPs", "Cert", "CRL", "OCSP"} {
				if !visitArrays(item.Key(category)) {
					return nil, false
				}
			}
		}
	}
	return result, true
}

func addDSSFieldObjects(data []byte, protected map[uint32]bool) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	acro := r.Trailer().Key("Root").Key("AcroForm")
	if n, _ := acro.ObjectReference(); n != 0 {
		protected[n] = true
	}
	fields := acro.Key("Fields")
	var visit func(pdf.Value, int) bool
	visit = func(field pdf.Value, depth int) bool {
		if depth > 32 || len(protected) > 50000 {
			return false
		}
		n, _ := field.ObjectReference()
		if n == 0 {
			return false
		}
		if protected[n] {
			return true
		}
		protected[n] = true
		kids := field.Key("Kids")
		if kids.Len() > 10000 {
			return false
		}
		for i := 0; i < kids.Len(); i++ {
			if !visit(kids.Index(i), depth+1) {
				return false
			}
		}
		return true
	}
	if fields.Len() > 10000 {
		return false
	}
	for i := 0; i < fields.Len(); i++ {
		if !visit(fields.Index(i), 0) {
			return false
		}
	}
	return true
}

func onlyDSSKeys(dict map[string]string, keys ...string) bool {
	allowed := make(map[string]bool, len(keys))
	for _, key := range keys {
		allowed[key] = true
	}
	for key := range dict {
		if !allowed[key] {
			return false
		}
	}
	return true
}

func checkDSSArray(value pdf.Value, category string, snapshot pdf.XRefSnapshot, allowed map[uint32]bool, count, total *int) bool {
	if value.IsNull() {
		return true
	}
	if value.Kind() != pdf.Array || value.Len() > maxDSSV2Streams {
		return false
	}
	// Un array indirecto también pertenece exclusivamente al DSS.
	if n, _ := value.ObjectReference(); n != 0 {
		allowed[n] = true
	}
	for i := 0; i < value.Len(); i++ {
		if !checkDSSSingle(value.Index(i), category, snapshot, allowed, count, total) {
			return false
		}
	}
	return true
}

func checkDSSSingle(value pdf.Value, category string, snapshot pdf.XRefSnapshot, allowed map[uint32]bool, count, total *int) bool {
	if value.IsNull() {
		return true
	}
	n, _ := value.ObjectReference()
	entry := snapshot.Entries[n]
	if n == 0 || !entry.Active || entry.InStream || value.Kind() != pdf.Stream || *count >= maxDSSV2Streams {
		return false
	}
	if value.Key("Length").Int64() < 0 || value.Key("Length").Int64() > maxDSSV2StreamBytes {
		return false
	}
	reader := value.Reader()
	der, err := io.ReadAll(io.LimitReader(reader, maxDSSV2StreamBytes+1))
	_ = reader.Close()
	if err != nil || len(der) == 0 || len(der) > maxDSSV2StreamBytes || *total > maxDSSV2TotalBytes-len(der) {
		return false
	}
	var raw asn1.RawValue
	if rest, parseErr := asn1.Unmarshal(der, &raw); parseErr != nil || len(rest) != 0 || !bytes.Equal(raw.FullBytes, der) {
		return false
	}
	switch category {
	case "Certs":
		_, err = x509.ParseCertificate(der)
	case "CRLs":
		_, err = x509.ParseRevocationList(der)
	case "OCSPs":
		_, err = ocsp.ParseResponse(der, nil)
	default:
		return false
	}
	if err != nil {
		return false
	}
	allowed[n] = true
	*count++
	*total += len(der)
	return true
}
