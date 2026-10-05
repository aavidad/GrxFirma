// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

func vfTestXML(kind, number, previous, date string) []byte {
	id := `<IDFactura><IDEmisorFactura>89890001K</IDEmisorFactura><NumSerieFactura>` + number + `</NumSerieFactura><FechaExpedicionFactura>01-01-2024</FechaExpedicionFactura></IDFactura>`
	chain := `<Encadenamiento><PrimerRegistro>S</PrimerRegistro></Encadenamiento>`
	if previous != "" {
		chain = `<Encadenamiento><RegistroAnterior><IDEmisorFactura>89890001K</IDEmisorFactura><NumSerieFactura>12345678/G33</NumSerieFactura><FechaExpedicionFactura>01-01-2024</FechaExpedicionFactura><Huella>` + previous + `</Huella></RegistroAnterior></Encadenamiento>`
	}
	system := `<SistemaInformatico><NombreRazon>Prueba</NombreRazon><NIF>89890001K</NIF><NombreSistemaInformatico>Prueba</NombreSistemaInformatico><IdSistemaInformatico>01</IdSistemaInformatico><Version>1</Version><NumeroInstalacion>1</NumeroInstalacion><TipoUsoPosibleSoloVerifactu>N</TipoUsoPosibleSoloVerifactu><TipoUsoPosibleMultiOT>N</TipoUsoPosibleMultiOT><IndicadorMultiplesOT>N</IndicadorMultiplesOT></SistemaInformatico>`
	middle := `<NombreRazonEmisor>Prueba</NombreRazonEmisor><TipoFactura>F1</TipoFactura><DescripcionOperacion>Prueba</DescripcionOperacion><Desglose><DetalleDesglose><CalificacionOperacion>S1</CalificacionOperacion><BaseImponibleOimporteNoSujeto>100</BaseImponibleOimporteNoSujeto></DetalleDesglose></Desglose><CuotaTotal>12.35</CuotaTotal><ImporteTotal>123.45</ImporteTotal>`
	if kind == "RegistroAnulacion" {
		middle = ""
		id = strings.NewReplacer("IDEmisorFactura", "IDEmisorFacturaAnulada", "NumSerieFactura", "NumSerieFacturaAnulada", "FechaExpedicionFactura", "FechaExpedicionFacturaAnulada").Replace(id)
	}
	data := []byte(`<` + kind + ` xmlns="` + VeriFactuNamespace + `"><IDVersion>1.0</IDVersion>` + id + middle + chain + system + `<FechaHoraHusoGenRegistro>` + date + `</FechaHoraHusoGenRegistro><TipoHuella>01</TipoHuella><Huella>HASH</Huella></` + kind + `>`)
	hash, _ := RecalcularHuellaVeriFactu(data)
	return bytes.Replace(data, []byte("HASH"), []byte(hash), 1)
}

func TestVeriFactuOfficialHashVectors(t *testing.T) {
	first := vfTestXML("RegistroAlta", "12345678/G33", "", "2024-01-01T19:20:30+01:00")
	h, e := RecalcularHuellaVeriFactu(first)
	if e != nil || h != "3C464DAF61ACB827C65FDA19F352A4E3BDC2C640E9E9FC4CC058073F38F12F60" {
		t.Fatalf("%s %v", h, e)
	}
	second := vfTestXML("RegistroAlta", "12345679/G34", h, "2024-01-01T19:20:35+01:00")
	h2, _ := RecalcularHuellaVeriFactu(second)
	if h2 != "F7B94CFD8924EDFF273501B01EE5153E4CE8F259766F88CF6ACB8935802A2B97" {
		t.Fatal(h2)
	}
	third := vfTestXML("RegistroAnulacion", "12345679/G34", h2, "2024-01-01T19:20:40+01:00")
	h3, _ := RecalcularHuellaVeriFactu(third)
	if h3 != "177547C0D57AC74748561D054A9CEC14B4C4EA23D1BEFD6F2E69E3A388F90C68" {
		t.Fatal(h3)
	}
	// Espacios exteriores se recortan; interiores y decimales se conservan.
	spaced := bytes.Replace(first, []byte("12345678/G33"), []byte(" 12345678/G33 "), 1)
	hs, _ := RecalcularHuellaVeriFactu(spaced)
	if hs != h {
		t.Fatal(hs)
	}
	result := ValidarRegistrosVeriFactu(context.Background(), map[string][]byte{"z.xml": first, "a.xml": second})
	if !result.Valid || result.Errors != 0 {
		t.Fatalf("%+v", result)
	}
	partial := ValidarRegistrosVeriFactu(context.Background(), map[string][]byte{"a.xml": second})
	if partial.Warnings < 2 {
		t.Fatalf("%+v", partial)
	}
	broken := vfTestXML("RegistroAlta", "12345679/G34", strings.Repeat("A", 64), "2024-01-01T19:20:35+01:00")
	if ValidarRegistrosVeriFactu(context.Background(), map[string][]byte{"first.xml": first, "broken.xml": broken}).Valid {
		t.Fatal("accepted a chain with an unreachable predecessor")
	}
	// Manipular un valor conserva el formato, pero rompe la huella.
	bad := bytes.Replace(second, []byte("123.45"), []byte("123.46"), 1)
	if ValidarRegistrosVeriFactu(context.Background(), map[string][]byte{"bad.xml": bad}).Valid {
		t.Fatal("accepted modified hash")
	}
}

func TestVeriFactuStructureAndXMLLimits(t *testing.T) {
	good := vfTestXML("RegistroAlta", "12345678/G33", "", "2024-01-01T19:20:30+01:00")
	for name, data := range map[string][]byte{
		"unknown namespace": bytes.ReplaceAll(good, []byte(VeriFactuNamespace), []byte("urn:fake")),
		"duplicate field":   bytes.Replace(good, []byte("<IDVersion>1.0</IDVersion>"), []byte("<IDVersion>1.0</IDVersion><IDVersion>1.0</IDVersion>"), 1),
		"nested leaf":       bytes.Replace(good, []byte("<CuotaTotal>12.35</CuotaTotal>"), []byte("<CuotaTotal><v>12.35</v></CuotaTotal>"), 1),
		"wrong order":       bytes.Replace(good, []byte("<TipoHuella>01</TipoHuella>"), []byte("<ImporteTotal>123.45</ImporteTotal><TipoHuella>01</TipoHuella>"), 1),
		"enum":              bytes.Replace(good, []byte("<TipoFactura>F1</TipoFactura>"), []byte("<TipoFactura>BAD</TipoFactura>"), 1),
		"length":            bytes.Replace(good, []byte("12345678/G33"), []byte(strings.Repeat("x", 61)), 1),
		"dtd":               append([]byte(`<!DOCTYPE r [<!ENTITY x SYSTEM "file:///etc/passwd">]>`), good...),
		"depth":             []byte(strings.Repeat("<n>", 65) + strings.Repeat("</n>", 65)),
		"nodes":             []byte("<root>" + strings.Repeat("<n/>", 20001) + "</root>"),
		"empty event":       []byte(`<RegistroEvento xmlns="` + VeriFactuEventNamespace + `"/>`),
	} {
		t.Run(name, func(t *testing.T) {
			if ValidarRegistrosVeriFactu(context.Background(), map[string][]byte{"bad.xml": data}).Valid {
				t.Fatal("invalid XML accepted")
			}
		})
	}
}

func TestVeriFactuSignProfileAndTampering(t *testing.T) {
	priv, cert := certForTest(t, "VeriFactu")
	data := vfTestXML("RegistroAlta", "12345678/G33", "", time.Now().UTC().Format(time.RFC3339))
	doc, _ := domain.NewDocument("record.xml", data, "application/xml")
	job := domain.SignatureJob{Document: doc, Format: FormatVeriFactu, Action: domain.ActionSign, Options: map[string]string{"nodeToSign": "other", "algorithm": "SHA1withRSA", "policyIdentifier": "urn:bad", "format": "XAdES Detached"}}
	signed, e := NewVeriFactuSigner().Sign(context.Background(), job, &LocalSigningKey{ID: "key", Signer: priv, Certificate: cert})
	if e != nil {
		t.Fatal(e)
	}
	n, e := vfParse(signed.Data)
	if e != nil {
		t.Fatal(e)
	}
	problems := vfCheckSignature(signed.Data, n, time.Now())
	if len(problems) != 1 || problems[0].Key != "verifactu.trust" {
		t.Fatalf("%+v", problems)
	}
	result := ValidarRegistrosVeriFactu(context.Background(), map[string][]byte{"record.xml": signed.Data})
	if !result.Valid {
		t.Fatalf("%+v", result.Records)
	}
	tampered := bytes.Replace(signed.Data, []byte("123.45"), []byte("123.46"), 1)
	n, _ = vfParse(tampered)
	if vfCheckSignature(tampered, n, time.Now())[0].Level != "error" {
		t.Fatal("accepted altered signature")
	}
	// El perfil no firma envolturas superiores aunque contengan un registro válido.
	job.Document.Content = append(append([]byte("<RegistroFactura>"), data...), []byte("</RegistroFactura>")...)
	if _, e = NewVeriFactuSigner().Sign(context.Background(), job, &LocalSigningKey{Signer: priv, Certificate: cert}); e == nil {
		t.Fatal("signed wrapper")
	}
}

func TestVeriFactuEventAndCancellationSigning(t *testing.T) {
	priv, cert := certForTest(t, "VeriFactuEvent")
	date := time.Now().UTC().Format(time.RFC3339)
	base := vfTestXML("RegistroAlta", "12345678/G33", "", date)
	n, _ := vfParse(base)
	system := n.child(VeriFactuNamespace, "SistemaInformatico")
	systemXML := string(base[system.start:system.end])
	event := []byte(`<RegistroEvento xmlns="` + VeriFactuEventNamespace + `"><IDVersion>1.0</IDVersion><Evento>` + systemXML + `<ObligadoEmision><NombreRazon>Prueba</NombreRazon><NIF>89890001K</NIF></ObligadoEmision><FechaHoraHusoGenEvento>` + date + `</FechaHoraHusoGenEvento><TipoEvento>01</TipoEvento><Encadenamiento><PrimerEvento>S</PrimerEvento></Encadenamiento><TipoHuella>01</TipoHuella><HuellaEvento>HASH</HuellaEvento></Evento></RegistroEvento>`)
	hash, _ := RecalcularHuellaVeriFactu(event)
	event = bytes.Replace(event, []byte("HASH"), []byte(hash), 1)
	for _, data := range [][]byte{event, vfTestXML("RegistroAnulacion", "12345678/G33", "", date)} {
		doc, _ := domain.NewDocument("record.xml", data, "application/xml")
		signed, err := NewVeriFactuSigner().Sign(context.Background(), domain.SignatureJob{Document: doc, Format: FormatVeriFactu, Action: domain.ActionSign}, &LocalSigningKey{Signer: priv, Certificate: cert})
		if err != nil {
			t.Fatal(err)
		}
		result := ValidarRegistrosVeriFactu(context.Background(), map[string][]byte{"record.xml": signed.Data})
		if !result.Valid {
			t.Fatalf("%+v", result.Records)
		}
	}
}

func TestXAdESSigningCertificateUsesFirstEntry(t *testing.T) {
	_, cert := certForTest(t, "XAdES-first-cert")
	hash, _ := digestXML(algSHA256, cert.Raw)
	valid := base64.StdEncoding.EncodeToString(hash)
	for _, kind := range []string{"SigningCertificate", "SigningCertificateV2"} {
		entry := func(value string) string {
			return `<x:Cert><x:CertDigest><ds:DigestMethod Algorithm="` + algSHA256 + `"/><ds:DigestValue>` + value + `</ds:DigestValue></x:CertDigest></x:Cert>`
		}
		info := []byte(`<ds:SignedInfo xmlns:ds="` + nsXMLDSig + `"><ds:Reference URI="#props" Type="` + typeSignedProps + `"/></ds:SignedInfo>`)
		build := func(first, last string) []byte {
			return []byte(`<root xmlns:ds="` + nsXMLDSig + `" xmlns:x="` + nsXAdES + `"><x:SignedProperties Id="props"><x:SignedSignatureProperties><x:` + kind + `>` + entry(first) + entry(last) + `</x:` + kind + `></x:SignedSignatureProperties></x:SignedProperties></root>`)
		}
		if err := verifySigningCertificate(build(valid, "AAAA"), info, cert); err != nil {
			t.Fatal(err)
		}
		if err := verifySigningCertificate(build("AAAA", valid), info, cert); err == nil {
			t.Fatal("accepted a matching later certificate")
		}
	}
}

func TestVeriFactuEventHashFields(t *testing.T) {
	data := []byte(`<RegistroEvento xmlns="` + VeriFactuEventNamespace + `"><IDVersion>1.0</IDVersion><Evento><SistemaInformatico><NIF>89890001K</NIF><IdSistemaInformatico>77</IdSistemaInformatico><Version>1.0</Version><NumeroInstalacion>383</NumeroInstalacion></SistemaInformatico><ObligadoEmision><NIF>89890002E</NIF></ObligadoEmision><TipoEvento>01</TipoEvento><Encadenamiento><PrimerEvento>S</PrimerEvento></Encadenamiento><FechaHoraHusoGenEvento>2024-01-01T19:20:30+01:00</FechaHoraHusoGenEvento></Evento></RegistroEvento>`)
	n, e := vfParse(data)
	if e != nil {
		t.Fatal(e)
	}
	_, input := vfHash(n)
	expected := "NIF=89890001K&ID=&IdSistemaInformatico=77&Version=1.0&NumeroInstalacion=383&NIF=89890002E&TipoEvento=01&HuellaEvento=&FechaHoraHusoGenEvento=2024-01-01T19:20:30+01:00"
	if input != expected {
		t.Fatalf("%s", input)
	}
}

func TestVeriFactuAEATOptional(t *testing.T) {
	root := os.Getenv("GRXFIRMA_VERIFACTU_ANEXOS")
	if root == "" {
		t.Skip("GRXFIRMA_VERIFACTU_ANEXOS")
	}
	for _, name := range []string{"ejemploRegistro.xml", "ejemploRegistro-firmado-epes-xades4j.xml"} {
		data, e := os.ReadFile(filepath.Join(root, name))
		if os.IsNotExist(e) {
			t.Skip(name)
		}
		if e != nil {
			t.Fatal(e)
		}

		result := ValidarRegistrosVeriFactu(context.Background(), map[string][]byte{name: data})
		if !result.Valid {
			t.Fatalf("%s: %+v", name, result.Records)
		}
		if name == "ejemploRegistro.xml" {
			priv, baseCert := certForTest(t, "VeriFactu-AEAT-synthetic")
			template := *baseCert
			template.NotBefore = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			template.NotAfter = time.Now().AddDate(1, 0, 0)
			der, err := x509.CreateCertificate(rand.Reader, &template, &template, priv.Public(), priv)
			if err != nil {
				t.Fatal(err)
			}
			cert, err := x509.ParseCertificate(der)
			if err != nil {
				t.Fatal(err)
			}
			doc, _ := domain.NewDocument(name, data, "application/xml")
			signed, err := NewVeriFactuSigner().Sign(context.Background(), domain.SignatureJob{Document: doc, Format: FormatVeriFactu, Action: domain.ActionSign}, &LocalSigningKey{Signer: priv, Certificate: cert})
			if err != nil {
				t.Fatal(err)
			}
			check := ValidarRegistrosVeriFactu(context.Background(), map[string][]byte{name: signed.Data})
			if !check.Valid {
				t.Fatalf("signed AEAT example: %+v", check.Records)
			}
		}
	}
}

// Si se aportan los XSD descargados, exigir que las reglas nativas sigan todas
// sus secuencias, alternativas y restricciones para los tipos alcanzables.
func TestVeriFactuXSDRulesOptional(t *testing.T) {
	root := os.Getenv("GRXFIRMA_VERIFACTU_ANEXOS")
	if root == "" {
		t.Skip("GRXFIRMA_VERIFACTU_ANEXOS")
	}
	tested := 0
	for name, expected := range vfSchemas {
		data, err := os.ReadFile(filepath.Join(root, name+".xsd"))
		if os.IsNotExist(err) {
			t.Logf("%s.xsd unavailable", name)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		xsd, err := vfParse(data)
		if err != nil {
			t.Fatal(err)
		}
		types := map[string]*vfNode{}
		for _, n := range xsd.children {
			if n.name.Local == "simpleType" || n.name.Local == "complexType" {
				types[vfAttribute(n, "name")] = n
			}
		}
		actual := vfSchema{Roots: map[string]vfRule{}, Types: map[string]vfRule{}}
		var compile func(*vfNode) vfRule
		compile = func(n *vfNode) vfRule {
			r := vfRule{Kind: n.name.Local, Name: vfAttribute(n, "name"), Type: strings.TrimPrefix(vfAttribute(n, "type"), "sf:"), Ref: vfAttribute(n, "ref"), Value: vfAttribute(n, "value"), Base: vfAttribute(n, "base")}
			if n.name.Local == "element" || n.name.Local == "sequence" || n.name.Local == "choice" {
				r.Min, r.Max = 1, 1
				if v := vfAttribute(n, "minOccurs"); v != "" {
					r.Min, _ = strconv.Atoi(v)
				}
				if v := vfAttribute(n, "maxOccurs"); v != "" {
					r.Max, _ = strconv.Atoi(v)
				}
			}
			if target, found := types[r.Type]; found {
				if _, done := actual.Types[r.Type]; !done {
					actual.Types[r.Type] = vfRule{}
					actual.Types[r.Type] = compile(target)
				}
			}
			for _, c := range n.children {
				if c.name.Local != "annotation" {
					r.Children = append(r.Children, compile(c))
				}
			}
			return r
		}
		for _, n := range xsd.children {
			if _, needed := expected.Roots[vfAttribute(n, "name")]; needed && n.name.Local == "element" {
				actual.Roots[vfAttribute(n, "name")] = compile(n)
			}
		}
		tested++
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("native rules differ from %s.xsd", name)
		}
	}
	if tested == 0 {
		t.Skip("XSD unavailable in GRXFIRMA_VERIFACTU_ANEXOS")
	}
}

type vfRoundTrip func(*http.Request) (*http.Response, error)

func (f vfRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestVeriFactuQRStrictAndQuery(t *testing.T) {
	raw := "https://www2.agenciatributaria.gob.es/wlpl/TIKE-CONT/ValidarQR?nif=89890001K&numserie=ABC%26G33&fecha=01-01-2025&importe=241.4"
	qr, e := LeerQRVeriFactu(raw)
	if e != nil || qr.Number != "ABC&G33" {
		t.Fatalf("%+v %v", qr, e)
	}
	for _, bad := range []string{
		strings.Replace(raw, "https:", "http:", 1), strings.Replace(raw, "www2.agenciatributaria.gob.es", "www2.agenciatributaria.gob.es.evil.test", 1), strings.Replace(raw, "www2.agenciatributaria.gob.es", "www2.agenciatributaria.gob.es:443", 1), strings.Replace(raw, "www2.agenciatributaria.gob.es", "ｗｗｗ2.agenciatributaria.gob.es", 1), strings.Replace(raw, "www2.agenciatributaria.gob.es", "www2.agenciatributaria.gob.es@evil.test", 1), raw + "#", raw + "&formato=json", raw + "&nif=89890001K", strings.Replace(raw, "241.4", "241,4", 1), strings.Replace(raw, "01-01-2025", "31-02-2025", 1), strings.Replace(raw, "89890001K", "89890001J", 1), strings.Replace(raw, "ABC%26G33", "ABC%0AG33", 1), strings.Replace(raw, "ABC%26G33", "%ZZ", 1), strings.Replace(raw, "ValidarQR", "%56alidarQR", 1),
	} {
		if _, e := LeerQRVeriFactu(bad); e == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	calls := 0
	client := &http.Client{Transport: vfRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Host != "www2.agenciatributaria.gob.es" || len(r.URL.Query()) != 5 || r.URL.Query().Get("formato") != "json" || r.Body != nil || r.Header.Get("Authorization") != "" {
			t.Fatalf("%+v", r)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"resultado":"ok"}`)), Header: make(http.Header)}, nil
	})}
	data, e := vfQuery(context.Background(), client, qr)
	if e != nil || !json.Valid(data) || calls != 1 {
		t.Fatalf("%s %v %d", data, e, calls)
	}
	client.Transport = vfRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 302, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{"Location": []string{"https://evil.test"}}}, nil
	})
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if _, e := vfQuery(context.Background(), client, qr); e == nil {
		t.Fatal("accepted redirect")
	}
}
