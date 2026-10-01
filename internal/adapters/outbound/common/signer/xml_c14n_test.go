// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"strings"
	"testing"
)

func TestCanonicalizeXML_C14N10W3C(t *testing.T) {
	input := `<doc>
   <e1   />
   <e2   ></e2>
   <e3   name = "elem3"   id="elem3"   />
   <e4   name="elem4"   id="elem4"   ></e4>
   <e5 a:attr="out" b:attr="sorted" attr2="all" attr="I'm"
      xmlns:b="http://www.ietf.org"
      xmlns:a="http://www.w3.org"
      xmlns="http://example.org"/>
   <e6 xmlns="" xmlns:a="http://www.w3.org">
      <e7 xmlns="http://www.ietf.org">
         <e8 xmlns="" xmlns:a="http://www.w3.org">
            <e9 xmlns="" xmlns:a="http://www.ietf.org"/>
         </e8>
      </e7>
   </e6>
</doc>`
	expected := `<doc>
   <e1></e1>
   <e2></e2>
   <e3 id="elem3" name="elem3"></e3>
   <e4 id="elem4" name="elem4"></e4>
   <e5 xmlns="http://example.org" xmlns:a="http://www.w3.org" xmlns:b="http://www.ietf.org" attr="I'm" attr2="all" b:attr="sorted" a:attr="out"></e5>
   <e6 xmlns:a="http://www.w3.org">
      <e7 xmlns="http://www.ietf.org">
         <e8 xmlns="">
            <e9 xmlns:a="http://www.ietf.org"></e9>
         </e8>
      </e7>
   </e6>
</doc>`

	got, err := canonicalizeXML([]byte(input), algC14N)
	if err != nil {
		t.Fatalf("canonicalizeXML(C14N 1.0) error = %v", err)
	}
	if string(got) != expected {
		t.Fatalf("canonicalización distinta:\nobtenido: %s\nesperado: %s", got, expected)
	}
}

func TestCanonicalizeElementInDocument_InheritsNamespaces(t *testing.T) {
	input := `<RootElement xmlns="urn:root" xmlns:ds="` + nsXMLDSig + `" xmlns:xades="` + nsXAdES + `" xml:lang="es">` +
		`<ds:Signature><ds:SignedInfo Id="signed"><ds:Reference URI="#data"/></ds:SignedInfo></ds:Signature>` +
		`</RootElement>`

	inclusive, err := canonicalizeElementInDocument([]byte(input), nsXMLDSig, "SignedInfo", algC14N)
	if err != nil {
		t.Fatalf("canonicalizeElementInDocument(C14N) error = %v", err)
	}
	for _, inherited := range []string{
		`xmlns="urn:root"`,
		`xmlns:ds="` + nsXMLDSig + `"`,
		`xmlns:xades="` + nsXAdES + `"`,
		`xml:lang="es"`,
	} {
		if !strings.Contains(string(inclusive), inherited) {
			t.Fatalf("C14N inclusiva no heredó %s: %s", inherited, inclusive)
		}
	}

	exclusive, err := canonicalizeElementInDocument([]byte(input), nsXMLDSig, "SignedInfo", algExcC14N)
	if err != nil {
		t.Fatalf("canonicalizeElementInDocument(Exclusive C14N) error = %v", err)
	}
	if strings.Contains(string(exclusive), `xmlns:xades=`) || strings.Contains(string(exclusive), `xmlns="urn:root"`) || strings.Contains(string(exclusive), `xml:lang=`) {
		t.Fatalf("C14N exclusiva conservó espacios no utilizados: %s", exclusive)
	}
	if !strings.Contains(string(exclusive), `xmlns:ds="`+nsXMLDSig+`"`) {
		t.Fatalf("C14N exclusiva perdió el espacio ds utilizado: %s", exclusive)
	}
}
