// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"os"
	"strings"
	"testing"
)

// facturaPrueba genera una FacturaE 3.2.2 sintética; los reemplazos permiten
// introducir errores concretos.
func facturaPrueba(reemplazos ...string) []byte {
	x := `<?xml version="1.0" encoding="UTF-8"?>
<fe:Facturae xmlns:fe="http://www.facturae.gob.es/formato/Versiones/Facturaev3_2_2.xml">
<FileHeader><SchemaVersion>3.2.2</SchemaVersion><Modality>I</Modality><InvoiceIssuerType>EM</InvoiceIssuerType>
<Batch><BatchIdentifier>LOTE-2026-1</BatchIdentifier><InvoicesCount>1</InvoicesCount>
<TotalInvoicesAmount><TotalAmount>114.00</TotalAmount></TotalInvoicesAmount>
<TotalOutstandingAmount><TotalAmount>114.00</TotalAmount></TotalOutstandingAmount>
<TotalExecutableAmount><TotalAmount>114.00</TotalAmount></TotalExecutableAmount>
<InvoiceCurrencyCode>EUR</InvoiceCurrencyCode></Batch></FileHeader>
<Parties>
<SellerParty><TaxIdentification><PersonTypeCode>J</PersonTypeCode><ResidenceTypeCode>R</ResidenceTypeCode><TaxIdentificationNumber>B99999997</TaxIdentificationNumber></TaxIdentification></SellerParty>
<BuyerParty><TaxIdentification><PersonTypeCode>J</PersonTypeCode><ResidenceTypeCode>R</ResidenceTypeCode><TaxIdentificationNumber>Q2826000H</TaxIdentificationNumber></TaxIdentification>
<AdministrativeCentres>
<AdministrativeCentre><CentreCode>EA0000001</CentreCode><RoleTypeCode>01</RoleTypeCode></AdministrativeCentre>
<AdministrativeCentre><CentreCode>EA0000002</CentreCode><RoleTypeCode>02</RoleTypeCode></AdministrativeCentre>
<AdministrativeCentre><CentreCode>EA0000003</CentreCode><RoleTypeCode>03</RoleTypeCode></AdministrativeCentre>
</AdministrativeCentres></BuyerParty>
</Parties>
<Invoices><Invoice><InvoiceHeader><InvoiceNumber>1</InvoiceNumber></InvoiceHeader>
<TaxesOutputs><Tax><TaxTypeCode>01</TaxTypeCode><TaxRate>21.00</TaxRate><TaxableBase><TotalAmount>100.00</TotalAmount></TaxableBase><TaxAmount><TotalAmount>21.00</TotalAmount></TaxAmount></Tax></TaxesOutputs>
<TaxesWithheld><Tax><TaxTypeCode>04</TaxTypeCode><TaxRate>7.00</TaxRate><TaxableBase><TotalAmount>100.00</TotalAmount></TaxableBase><TaxAmount><TotalAmount>7.00</TotalAmount></TaxAmount></Tax></TaxesWithheld>
<InvoiceTotals><TotalGrossAmount>100.00</TotalGrossAmount><TotalGrossAmountBeforeTaxes>100.00</TotalGrossAmountBeforeTaxes>
<TotalTaxOutputs>21.00</TotalTaxOutputs><TotalTaxesWithheld>7.00</TotalTaxesWithheld><InvoiceTotal>114.00</InvoiceTotal>
<TotalOutstandingAmount>114.00</TotalOutstandingAmount><TotalExecutableAmount>114.00</TotalExecutableAmount></InvoiceTotals>
<Items><InvoiceLine><ItemDescription>Servicio</ItemDescription><Quantity>2</Quantity><UnitPriceWithoutTax>50.000000</UnitPriceWithoutTax>
<TotalCost>100.00</TotalCost><GrossAmount>100.00</GrossAmount></InvoiceLine></Items>
</Invoice></Invoices></fe:Facturae>`
	for i := 0; i+1 < len(reemplazos); i += 2 {
		x = strings.Replace(x, reemplazos[i], reemplazos[i+1], 1)
	}
	return []byte(x)
}

func TestRevisarFacturaE_CorrectaSinIncidencias(t *testing.T) {
	inc, err := RevisarFacturaE(facturaPrueba())
	if err != nil || len(inc) != 0 {
		t.Fatalf("err=%v incidencias=%v", err, inc)
	}
}

func TestRevisarFacturaE_DetectaDescuadresYDatosFACe(t *testing.T) {
	casos := map[string][]string{
		"InvoiceTotal":                   {"<InvoiceTotal>114.00", "<InvoiceTotal>121.00"},
		"TotalGrossAmount":               {"<GrossAmount>100.00", "<GrossAmount>90.00"},
		"Lote/InvoicesCount":             {"<InvoicesCount>1", "<InvoicesCount>2"},
		"Lote/TotalInvoicesAmount":       {"<TotalInvoicesAmount><TotalAmount>114.00", "<TotalInvoicesAmount><TotalAmount>115.00"},
		"TaxesOutputs/1":                 {"<TaxAmount><TotalAmount>21.00", "<TaxAmount><TotalAmount>20.00"},
		"Receptor/AdministrativeCentres": {"<RoleTypeCode>03</RoleTypeCode>", "<RoleTypeCode>04</RoleTypeCode>"},
		"Emisor/TaxIdentificationNumber": {"B99999997", "B99999998"},
		"SchemaVersion":                  {"<SchemaVersion>3.2.2", "<SchemaVersion>3.1"},
		"TotalCost":                      {"<Quantity>2", "<Quantity>3"},
	}
	for campo, r := range casos {
		inc, err := RevisarFacturaE(facturaPrueba(r...))
		if err != nil {
			t.Fatalf("%s: %v", campo, err)
		}
		encontrado := false
		for _, i := range inc {
			if strings.Contains(i.Campo, campo) {
				encontrado = true
			}
		}
		if !encontrado {
			t.Errorf("%s: no detectado; incidencias=%v", campo, inc)
		}
	}
}

func TestRevisarFacturaE_EjemploHistorico(t *testing.T) {
	data, err := os.ReadFile("testdata/sample-facturae.xml")
	if err != nil {
		t.Fatal(err)
	}
	inc, err := RevisarFacturaE(data)
	if err != nil {
		t.Fatal(err)
	}
	// Versión 3.1 y NIF de pruebas sin dígito de control válido.
	texto := ResumenIncidenciasFactura(inc)
	for _, esperado := range []string{"SchemaVersion", "Emisor/TaxIdentificationNumber", "Receptor/TaxIdentificationNumber"} {
		if !strings.Contains(texto, esperado) {
			t.Errorf("falta %s en:\n%s", esperado, texto)
		}
	}
	if strings.Contains(texto, "InvoiceTotal") {
		t.Errorf("los importes del ejemplo cuadran:\n%s", texto)
	}
}

func TestIdentificadorFiscalEspanolValido(t *testing.T) {
	validos := []string{"12345678Z", "X1234567L", "B99999997", "Q2826000H", "ES-B99999997", "A58818501"}
	invalidos := []string{"12345678A", "X1234567A", "B99999998", "Q2826000I", "", "123", "Z12345678"}
	for _, v := range validos {
		if !IdentificadorFiscalEspanolValido(v) {
			t.Errorf("%s debería ser válido", v)
		}
	}
	for _, v := range invalidos {
		if IdentificadorFiscalEspanolValido(v) {
			t.Errorf("%s debería ser inválido", v)
		}
	}
}
