// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"strings"
	"testing"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/avisos"
)

func facturaUBLPrueba(reemplazos ...string) []byte {
	x := `<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
 xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
 xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
 <cbc:CustomizationID>urn:cen.eu:en16931:2017</cbc:CustomizationID>
 <cbc:ID>F-2026-1</cbc:ID>
 <cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
 <cac:AccountingSupplierParty><cac:Party><cac:PartyTaxScheme><cbc:CompanyID>ESB99999997</cbc:CompanyID></cac:PartyTaxScheme></cac:Party></cac:AccountingSupplierParty>
 <cac:AccountingCustomerParty><cac:Party><cac:PartyTaxScheme><cbc:CompanyID>ESA58818501</cbc:CompanyID></cac:PartyTaxScheme></cac:Party></cac:AccountingCustomerParty>
 <cac:TaxTotal><cbc:TaxAmount currencyID="EUR">31.50</cbc:TaxAmount>
  <cac:TaxSubtotal><cbc:TaxableAmount currencyID="EUR">150.00</cbc:TaxableAmount><cbc:TaxAmount currencyID="EUR">31.50</cbc:TaxAmount>
   <cac:TaxCategory><cbc:ID>S</cbc:ID><cbc:Percent>21</cbc:Percent></cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>
 <cac:LegalMonetaryTotal>
  <cbc:LineExtensionAmount currencyID="EUR">160.00</cbc:LineExtensionAmount>
  <cbc:TaxExclusiveAmount currencyID="EUR">150.00</cbc:TaxExclusiveAmount>
  <cbc:TaxInclusiveAmount currencyID="EUR">181.50</cbc:TaxInclusiveAmount>
  <cbc:AllowanceTotalAmount currencyID="EUR">10.00</cbc:AllowanceTotalAmount>
  <cbc:PrepaidAmount currencyID="EUR">50.00</cbc:PrepaidAmount>
  <cbc:PayableAmount currencyID="EUR">131.50</cbc:PayableAmount>
 </cac:LegalMonetaryTotal>
 <cac:InvoiceLine><cbc:ID>1</cbc:ID><cbc:LineExtensionAmount currencyID="EUR">100.00</cbc:LineExtensionAmount></cac:InvoiceLine>
 <cac:InvoiceLine><cbc:ID>2</cbc:ID><cbc:LineExtensionAmount currencyID="EUR">60.00</cbc:LineExtensionAmount></cac:InvoiceLine>
</Invoice>`
	for i := 0; i+1 < len(reemplazos); i += 2 {
		x = strings.Replace(x, reemplazos[i], reemplazos[i+1], 1)
	}
	return []byte(x)
}

func facturaCIIPrueba(reemplazos ...string) []byte {
	x := `<?xml version="1.0" encoding="UTF-8"?>
<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"
 xmlns:ram="urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100">
 <rsm:SupplyChainTradeTransaction>
  <ram:IncludedSupplyChainTradeLineItem><ram:SpecifiedLineTradeSettlement><ram:SpecifiedTradeSettlementLineMonetarySummation><ram:LineTotalAmount>200.00</ram:LineTotalAmount></ram:SpecifiedTradeSettlementLineMonetarySummation></ram:SpecifiedLineTradeSettlement></ram:IncludedSupplyChainTradeLineItem>
  <ram:ApplicableHeaderTradeAgreement>
   <ram:SellerTradeParty><ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">ESB99999997</ram:ID></ram:SpecifiedTaxRegistration></ram:SellerTradeParty>
   <ram:BuyerTradeParty><ram:SpecifiedTaxRegistration><ram:ID schemeID="VA">DE123456789</ram:ID></ram:SpecifiedTaxRegistration></ram:BuyerTradeParty>
  </ram:ApplicableHeaderTradeAgreement>
  <ram:ApplicableHeaderTradeSettlement>
   <ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>
   <ram:ApplicableTradeTax><ram:CalculatedAmount>42.00</ram:CalculatedAmount><ram:BasisAmount>200.00</ram:BasisAmount><ram:RateApplicablePercent>21</ram:RateApplicablePercent></ram:ApplicableTradeTax>
   <ram:SpecifiedTradeSettlementHeaderMonetarySummation>
    <ram:LineTotalAmount>200.00</ram:LineTotalAmount>
    <ram:TaxBasisTotalAmount>200.00</ram:TaxBasisTotalAmount>
    <ram:TaxTotalAmount currencyID="EUR">42.00</ram:TaxTotalAmount>
    <ram:GrandTotalAmount>242.00</ram:GrandTotalAmount>
    <ram:DuePayableAmount>242.00</ram:DuePayableAmount>
   </ram:SpecifiedTradeSettlementHeaderMonetarySummation>
  </ram:ApplicableHeaderTradeSettlement>
 </rsm:SupplyChainTradeTransaction>
</rsm:CrossIndustryInvoice>`
	for i := 0; i+1 < len(reemplazos); i += 2 {
		x = strings.Replace(x, reemplazos[i], reemplazos[i+1], 1)
	}
	return []byte(x)
}

func TestRevisarFactura_UBLYCIICorrectas(t *testing.T) {
	for nombre, data := range map[string][]byte{"UBL": facturaUBLPrueba(), "CII": facturaCIIPrueba()} {
		formato, inc, err := RevisarFactura(data)
		if err != nil || string(formato) != nombre || len(inc) != 0 {
			t.Errorf("%s: formato=%s err=%v incidencias=%v", nombre, formato, err, inc)
		}
	}
}

func TestRevisarFactura_DetectaReglasEN16931(t *testing.T) {
	casos := []struct {
		nombre, campo string
		data          []byte
	}{
		{"UBL líneas", "BR-CO-10", facturaUBLPrueba(`<cbc:LineExtensionAmount currencyID="EUR">60.00`, `<cbc:LineExtensionAmount currencyID="EUR">61.00`)},
		{"UBL base", "BR-CO-13", facturaUBLPrueba(`<cbc:TaxExclusiveAmount currencyID="EUR">150.00`, `<cbc:TaxExclusiveAmount currencyID="EUR">160.00`)},
		{"UBL desglose", "BR-CO-14", facturaUBLPrueba(`<cbc:TaxAmount currencyID="EUR">31.50</cbc:TaxAmount>`, `<cbc:TaxAmount currencyID="EUR">30.00</cbc:TaxAmount>`)},
		{"UBL total", "BR-CO-15", facturaUBLPrueba(`<cbc:TaxInclusiveAmount currencyID="EUR">181.50`, `<cbc:TaxInclusiveAmount currencyID="EUR">180.00`)},
		{"UBL a pagar", "BR-CO-16", facturaUBLPrueba(`<cbc:PayableAmount currencyID="EUR">131.50`, `<cbc:PayableAmount currencyID="EUR">181.50`)},
		{"UBL NIF", "Vendedor/NIF-IVA", facturaUBLPrueba("ESB99999997", "ESB99999998")},
		{"CII total", "BR-CO-15", facturaCIIPrueba("<ram:GrandTotalAmount>242.00", "<ram:GrandTotalAmount>240.00")},
		{"CII cuota", "ApplicableTradeTax 1", facturaCIIPrueba("<ram:CalculatedAmount>42.00", "<ram:CalculatedAmount>40.00")},
		{"CII NIF", "Vendedor/NIF-IVA", facturaCIIPrueba("ESB99999997", "ESB12345678")},
	}
	for _, c := range casos {
		_, inc, err := RevisarFactura(c.data)
		if err != nil {
			t.Fatalf("%s: %v", c.nombre, err)
		}
		encontrado := false
		for _, i := range inc {
			if strings.Contains(i.Campo, c.campo) {
				encontrado = true
			}
		}
		if !encontrado {
			t.Errorf("%s: no se detectó %s; incidencias=%v", c.nombre, c.campo, inc)
		}
	}
}

func TestDetectarFormatoFactura(t *testing.T) {
	if _, err := DetectarFormatoFactura([]byte(`<pedido/>`)); err == nil {
		t.Error("un XML que no es factura debe rechazarse")
	}
	if f, err := DetectarFormatoFactura(facturaPrueba()); err != nil || f != FormatoFacturaE {
		t.Errorf("FacturaE: %s %v", f, err)
	}
}

func TestXAdES_FacturaUBLDescuadradaAvisaAlFirmar(t *testing.T) {
	avisos.Reiniciar()
	defer avisos.Reiniciar()
	priv, cert := certForTest(t, "Factura UBL")
	doc, _ := domain.NewDocument("factura.xml", facturaUBLPrueba(`<cbc:PayableAmount currencyID="EUR">131.50`, `<cbc:PayableAmount currencyID="EUR">181.50`), "application/xml")
	_, err := NewXAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatXAdES, Action: domain.ActionSign,
		Options: map[string]string{"format": "XAdES Enveloped"},
	}, &LocalSigningKey{ID: "k", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(avisos.Texto(), "BR-CO-16") {
		t.Fatalf("se esperaba el aviso de la revisión de la factura, avisos=%q", avisos.Texto())
	}
}
