// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/avisos"
)

// Revisión de facturas B2B en los formatos de la norma europea EN 16931
// (UBL 2.1 y UN/CEFACT CII), que la Ley 18/2022 (Crea y Crece) admite junto a
// FacturaE. Se comprueban las reglas de cuadre de la norma (BR-CO-10, 13, 14,
// 15 y 16), la cuota de cada tipo de IVA y los NIF españoles.

const (
	nsUBLInvoice    = "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
	nsUBLCreditNote = "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"
	nsCII           = "urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"
)

// FormatoFactura identifica el formato de una factura electrónica.
type FormatoFactura string

const (
	FormatoFacturaE FormatoFactura = "FacturaE"
	FormatoUBL      FormatoFactura = "UBL"
	FormatoCII      FormatoFactura = "CII"
)

// DetectarFormatoFactura reconoce FacturaE, UBL (factura o abono) y CII.
func DetectarFormatoFactura(data []byte) (FormatoFactura, error) {
	dec := xml.NewDecoder(bytes.NewReader(sanitizeXMLDocument(data)))
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", errors.New("el fichero no es un XML de factura legible")
		}
		if el, ok := tok.(xml.StartElement); ok {
			switch {
			case el.Name.Local == "Facturae":
				return FormatoFacturaE, nil
			case el.Name.Space == nsUBLInvoice && el.Name.Local == "Invoice",
				el.Name.Space == nsUBLCreditNote && el.Name.Local == "CreditNote":
				return FormatoUBL, nil
			case el.Name.Space == nsCII && el.Name.Local == "CrossIndustryInvoice":
				return FormatoCII, nil
			}
			return "", fmt.Errorf("formato de factura no reconocido (%s); se admiten FacturaE, UBL 2.1 y CII", el.Name.Local)
		}
	}
}

// RevisarFactura revisa una factura en cualquiera de los formatos admitidos.
func RevisarFactura(data []byte) (FormatoFactura, []IncidenciaFactura, error) {
	formato, err := DetectarFormatoFactura(data)
	if err != nil {
		return "", nil, err
	}
	var inc []IncidenciaFactura
	switch formato {
	case FormatoFacturaE:
		inc, err = RevisarFacturaE(data)
	case FormatoUBL:
		inc, err = revisarUBL(data)
	case FormatoCII:
		inc, err = revisarCII(data)
	}
	return formato, inc, err
}

type importeUBL struct {
	Valor  string `xml:",chardata"`
	Moneda string `xml:"currencyID,attr"`
}

type impuestoUBL struct {
	TaxAmount importeUBL `xml:"TaxAmount"`
	Subtotal  []struct {
		Base    importeUBL `xml:"TaxableAmount"`
		Cuota   importeUBL `xml:"TaxAmount"`
		Percent string     `xml:"TaxCategory>Percent"`
		Tipo    string     `xml:"TaxCategory>ID"`
	} `xml:"TaxSubtotal"`
}

type facturaUBL struct {
	Moneda   string        `xml:"DocumentCurrencyCode"`
	Vendedor []string      `xml:"AccountingSupplierParty>Party>PartyTaxScheme>CompanyID"`
	Comprad  []string      `xml:"AccountingCustomerParty>Party>PartyTaxScheme>CompanyID"`
	Lineas   []importeUBL  `xml:"InvoiceLine>LineExtensionAmount"`
	Abonos   []importeUBL  `xml:"CreditNoteLine>LineExtensionAmount"`
	Impuesto []impuestoUBL `xml:"TaxTotal"`
	Totales  struct {
		Lineas      string `xml:"LineExtensionAmount"`
		SinImpuesto string `xml:"TaxExclusiveAmount"`
		ConImpuesto string `xml:"TaxInclusiveAmount"`
		Descuentos  string `xml:"AllowanceTotalAmount"`
		Cargos      string `xml:"ChargeTotalAmount"`
		Anticipos   string `xml:"PrepaidAmount"`
		Redondeo    string `xml:"PayableRoundingAmount"`
		APagar      string `xml:"PayableAmount"`
	} `xml:"LegalMonetaryTotal"`
}

func revisarUBL(data []byte) ([]IncidenciaFactura, error) {
	var f facturaUBL
	if err := decodificarFactura(data, &f); err != nil {
		return nil, err
	}
	r := &revisionFactura{}
	lineas := append(f.Lineas, f.Abonos...)
	if len(lineas) == 0 {
		r.add(IncidenciaError, "InvoiceLine", "la factura no tiene líneas")
	}
	var suma float64
	for _, l := range lineas {
		suma += r.num(l.Valor)
	}
	t := f.Totales
	r.cuadra("LegalMonetaryTotal/LineExtensionAmount (BR-CO-10)", t.Lineas, suma, "la suma de las líneas")
	r.cuadra("LegalMonetaryTotal/TaxExclusiveAmount (BR-CO-13)", t.SinImpuesto, r.num(t.Lineas)-r.num(t.Descuentos)+r.num(t.Cargos), "las líneas menos descuentos más cargos")
	// El impuesto en la moneda del documento es el TaxTotal con desglose.
	var cuotaTotal float64
	for i, it := range f.Impuesto {
		if len(it.Subtotal) == 0 && len(f.Impuesto) > 1 {
			continue
		}
		var suma float64
		for j, s := range it.Subtotal {
			base, cuota := r.num(s.Base.Valor), r.num(s.Cuota.Valor)
			suma += cuota
			if s.Percent != "" {
				if calc := base * r.num(s.Percent) / 100; abs(calc-cuota) > toleranciaImporte {
					r.add(IncidenciaAviso, fmt.Sprintf("TaxTotal %d/TaxSubtotal %d", i+1, j+1), "la cuota %.2f no corresponde a la base %.2f al %s %% (%.2f)", cuota, base, strings.TrimSpace(s.Percent), calc)
				}
			}
		}
		if len(it.Subtotal) > 0 {
			r.cuadra(fmt.Sprintf("TaxTotal %d/TaxAmount (BR-CO-14)", i+1), it.TaxAmount.Valor, suma, "la suma de los desgloses de impuestos")
		}
		cuotaTotal = r.num(it.TaxAmount.Valor)
	}
	r.cuadra("LegalMonetaryTotal/TaxInclusiveAmount (BR-CO-15)", t.ConImpuesto, r.num(t.SinImpuesto)+cuotaTotal, "el importe sin impuestos más la cuota")
	r.cuadra("LegalMonetaryTotal/PayableAmount (BR-CO-16)", t.APagar, r.num(t.ConImpuesto)-r.num(t.Anticipos)+r.num(t.Redondeo), "el total menos anticipos más redondeo")
	r.nifIVA("Vendedor", f.Vendedor)
	r.nifIVA("Comprador", f.Comprad)
	return r.incidencias, nil
}

type facturaCII struct {
	Lineas  []string `xml:"SupplyChainTradeTransaction>IncludedSupplyChainTradeLineItem>SpecifiedLineTradeSettlement>SpecifiedTradeSettlementLineMonetarySummation>LineTotalAmount"`
	Vendedr []struct {
		ID     string `xml:",chardata"`
		Scheme string `xml:"schemeID,attr"`
	} `xml:"SupplyChainTradeTransaction>ApplicableHeaderTradeAgreement>SellerTradeParty>SpecifiedTaxRegistration>ID"`
	Comprad []struct {
		ID     string `xml:",chardata"`
		Scheme string `xml:"schemeID,attr"`
	} `xml:"SupplyChainTradeTransaction>ApplicableHeaderTradeAgreement>BuyerTradeParty>SpecifiedTaxRegistration>ID"`
	Liquidacion struct {
		Moneda    string `xml:"InvoiceCurrencyCode"`
		Impuestos []struct {
			Cuota string `xml:"CalculatedAmount"`
			Base  string `xml:"BasisAmount"`
			Tipo  string `xml:"RateApplicablePercent"`
		} `xml:"ApplicableTradeTax"`
		Totales struct {
			Lineas     string       `xml:"LineTotalAmount"`
			Cargos     string       `xml:"ChargeTotalAmount"`
			Descuentos string       `xml:"AllowanceTotalAmount"`
			Base       string       `xml:"TaxBasisTotalAmount"`
			Cuota      []importeUBL `xml:"TaxTotalAmount"`
			Redondeo   string       `xml:"RoundingAmount"`
			Total      string       `xml:"GrandTotalAmount"`
			Anticipos  string       `xml:"TotalPrepaidAmount"`
			APagar     string       `xml:"DuePayableAmount"`
		} `xml:"SpecifiedTradeSettlementHeaderMonetarySummation"`
	} `xml:"SupplyChainTradeTransaction>ApplicableHeaderTradeSettlement"`
}

func revisarCII(data []byte) ([]IncidenciaFactura, error) {
	var f facturaCII
	if err := decodificarFactura(data, &f); err != nil {
		return nil, err
	}
	r := &revisionFactura{}
	if len(f.Lineas) == 0 {
		r.add(IncidenciaError, "IncludedSupplyChainTradeLineItem", "la factura no tiene líneas")
	}
	var suma float64
	for _, l := range f.Lineas {
		suma += r.num(l)
	}
	liq := f.Liquidacion
	t := liq.Totales
	r.cuadra("LineTotalAmount (BR-CO-10)", t.Lineas, suma, "la suma de las líneas")
	r.cuadra("TaxBasisTotalAmount (BR-CO-13)", t.Base, r.num(t.Lineas)-r.num(t.Descuentos)+r.num(t.Cargos), "las líneas menos descuentos más cargos")
	var cuotas float64
	for i, it := range liq.Impuestos {
		base, cuota := r.num(it.Base), r.num(it.Cuota)
		cuotas += cuota
		if it.Tipo != "" {
			if calc := base * r.num(it.Tipo) / 100; abs(calc-cuota) > toleranciaImporte {
				r.add(IncidenciaAviso, fmt.Sprintf("ApplicableTradeTax %d", i+1), "la cuota %.2f no corresponde a la base %.2f al %s %% (%.2f)", cuota, base, strings.TrimSpace(it.Tipo), calc)
			}
		}
	}
	// TaxTotalAmount puede repetirse por moneda: vale el de la moneda de la factura.
	cuotaTotal := ""
	for _, c := range t.Cuota {
		if c.Moneda == "" || c.Moneda == liq.Moneda || cuotaTotal == "" {
			cuotaTotal = c.Valor
		}
	}
	r.cuadra("TaxTotalAmount (BR-CO-14)", cuotaTotal, cuotas, "la suma de los impuestos por tipo")
	r.cuadra("GrandTotalAmount (BR-CO-15)", t.Total, r.num(t.Base)+r.num(cuotaTotal), "la base más la cuota")
	r.cuadra("DuePayableAmount (BR-CO-16)", t.APagar, r.num(t.Total)-r.num(t.Anticipos)+r.num(t.Redondeo), "el total menos anticipos más redondeo")
	var vendedor, comprador []string
	for _, v := range f.Vendedr {
		if v.Scheme == "VA" {
			vendedor = append(vendedor, v.ID)
		}
	}
	for _, v := range f.Comprad {
		if v.Scheme == "VA" {
			comprador = append(comprador, v.ID)
		}
	}
	r.nifIVA("Vendedor", vendedor)
	r.nifIVA("Comprador", comprador)
	return r.incidencias, nil
}

// avisarIncidenciasFacturaB2B revisa una factura UBL o CII que se va a firmar
// en XAdES y explica al usuario lo que el receptor rechazaría, sin impedir
// la firma, como con FacturaE.
func avisarIncidenciasFacturaB2B(job domain.SignatureJob) {
	if job.Action != domain.ActionSign || !bytes.HasPrefix(bytes.TrimLeft(job.Document.Content, " \t\r\n\ufeff"), []byte("<")) {
		return
	}
	formato, err := DetectarFormatoFactura(job.Document.Content)
	if err != nil || (formato != FormatoUBL && formato != FormatoCII) {
		return
	}
	if _, incidencias, err := RevisarFactura(job.Document.Content); err == nil && len(incidencias) > 0 {
		avisos.Registrar("Revisión de la factura", ResumenIncidenciasFactura(incidencias))
	}
}

func decodificarFactura(data []byte, destino any) error {
	dec := xml.NewDecoder(bytes.NewReader(sanitizeXMLDocument(data)))
	dec.Strict = true
	if err := dec.Decode(destino); err != nil {
		return fmt.Errorf("no se puede leer la factura: %w", err)
	}
	return nil
}

// nifIVA comprueba los identificadores de IVA españoles (prefijo ES).
func (r *revisionFactura) nifIVA(parte string, ids []string) {
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if strings.HasPrefix(strings.ToUpper(id), "ES") && !IdentificadorFiscalEspanolValido(id) {
			r.add(IncidenciaAviso, parte+"/NIF-IVA", "%q no es un NIF español con dígito de control válido", id)
		}
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
