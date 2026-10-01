// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Revisión del contenido de una FacturaE antes de firmarla: cuadres de
// importes, identificadores fiscales y datos que FACe exige. AutoFirma Java no
// revisa el contenido; aquí se avisa al usuario de lo que el registro de
// facturas rechazaría, sin impedir la firma, porque la factura es suya.

// NivelIncidenciaFactura clasifica una incidencia de la revisión.
type NivelIncidenciaFactura string

const (
	// IncidenciaError: FACe o el receptor rechazarán la factura.
	IncidenciaError NivelIncidenciaFactura = "error"
	// IncidenciaAviso: conviene revisarlo, aunque puede ser correcto.
	IncidenciaAviso NivelIncidenciaFactura = "aviso"
)

// IncidenciaFactura es un problema detectado en la factura.
type IncidenciaFactura struct {
	Nivel   NivelIncidenciaFactura
	Campo   string
	Mensaje string
}

func (i IncidenciaFactura) String() string {
	return fmt.Sprintf("[%s] %s: %s", i.Nivel, i.Campo, i.Mensaje)
}

// toleranciaImporte admite las diferencias de redondeo a céntimos.
const toleranciaImporte = 0.011

type facturaeImporte struct {
	TotalAmount string `xml:"TotalAmount"`
}

type facturaeImpuesto struct {
	TaxTypeCode string          `xml:"TaxTypeCode"`
	TaxRate     string          `xml:"TaxRate"`
	TaxableBase facturaeImporte `xml:"TaxableBase"`
	TaxAmount   facturaeImporte `xml:"TaxAmount"`
}

type facturaeDescuento struct {
	DiscountAmount string `xml:"DiscountAmount"`
}

type facturaeCargo struct {
	ChargeAmount string `xml:"ChargeAmount"`
}

type facturaeIdentificacion struct {
	PersonTypeCode          string `xml:"PersonTypeCode"`
	ResidenceTypeCode       string `xml:"ResidenceTypeCode"`
	TaxIdentificationNumber string `xml:"TaxIdentificationNumber"`
}

type facturaeCentro struct {
	CentreCode   string `xml:"CentreCode"`
	RoleTypeCode string `xml:"RoleTypeCode"`
}

type facturaeParte struct {
	TaxIdentification     facturaeIdentificacion `xml:"TaxIdentification"`
	AdministrativeCentres []facturaeCentro       `xml:"AdministrativeCentres>AdministrativeCentre"`
}

type facturaeLinea struct {
	Quantity            string              `xml:"Quantity"`
	UnitPriceWithoutTax string              `xml:"UnitPriceWithoutTax"`
	TotalCost           string              `xml:"TotalCost"`
	Discounts           []facturaeDescuento `xml:"DiscountsAndRebates>Discount"`
	Charges             []facturaeCargo     `xml:"Charges>Charge"`
	GrossAmount         string              `xml:"GrossAmount"`
}

type facturaeFactura struct {
	Number        string             `xml:"InvoiceHeader>InvoiceNumber"`
	TaxesOutputs  []facturaeImpuesto `xml:"TaxesOutputs>Tax"`
	TaxesWithheld []facturaeImpuesto `xml:"TaxesWithheld>Tax"`
	Totals        struct {
		TotalGrossAmount            string              `xml:"TotalGrossAmount"`
		GeneralDiscounts            []facturaeDescuento `xml:"GeneralDiscounts>Discount"`
		GeneralSurcharges           []facturaeCargo     `xml:"GeneralSurcharges>Charge"`
		TotalGrossAmountBeforeTaxes string              `xml:"TotalGrossAmountBeforeTaxes"`
		TotalTaxOutputs             string              `xml:"TotalTaxOutputs"`
		TotalTaxesWithheld          string              `xml:"TotalTaxesWithheld"`
		InvoiceTotal                string              `xml:"InvoiceTotal"`
		TotalOutstandingAmount      string              `xml:"TotalOutstandingAmount"`
		TotalExecutableAmount       string              `xml:"TotalExecutableAmount"`
	} `xml:"InvoiceTotals"`
	Lines []facturaeLinea `xml:"Items>InvoiceLine"`
}

type facturaeDocumento struct {
	SchemaVersion string `xml:"FileHeader>SchemaVersion"`
	Batch         struct {
		InvoicesCount          string          `xml:"InvoicesCount"`
		TotalInvoicesAmount    facturaeImporte `xml:"TotalInvoicesAmount"`
		TotalOutstandingAmount facturaeImporte `xml:"TotalOutstandingAmount"`
		TotalExecutableAmount  facturaeImporte `xml:"TotalExecutableAmount"`
	} `xml:"FileHeader>Batch"`
	Seller   facturaeParte     `xml:"Parties>SellerParty"`
	Buyer    facturaeParte     `xml:"Parties>BuyerParty"`
	Invoices []facturaeFactura `xml:"Invoices>Invoice"`
}

// RevisarFacturaE revisa el contenido de una FacturaE. Devuelve error solo si
// el XML no puede leerse; las incidencias no impiden firmar.
func RevisarFacturaE(data []byte) ([]IncidenciaFactura, error) {
	var doc facturaeDocumento
	dec := xml.NewDecoder(bytes.NewReader(sanitizeXMLDocument(data)))
	dec.Strict = true
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("no se puede leer la factura: %w", err)
	}
	r := &revisionFactura{}
	r.cabecera(doc)
	r.parte("Emisor", doc.Seller)
	r.parte("Receptor", doc.Buyer)
	r.centrosFACe(doc.Buyer)
	var total, pendiente, ejecutable float64
	for i, f := range doc.Invoices {
		r.factura(i, f)
		total += r.num(f.Totals.InvoiceTotal)
		pendiente += r.num(f.Totals.TotalOutstandingAmount)
		ejecutable += r.num(f.Totals.TotalExecutableAmount)
	}
	r.cuadra("Lote/TotalInvoicesAmount", doc.Batch.TotalInvoicesAmount.TotalAmount, total, "la suma de los totales de las facturas")
	r.cuadra("Lote/TotalOutstandingAmount", doc.Batch.TotalOutstandingAmount.TotalAmount, pendiente, "la suma de los importes pendientes")
	r.cuadra("Lote/TotalExecutableAmount", doc.Batch.TotalExecutableAmount.TotalAmount, ejecutable, "la suma de los importes a ejecutar")
	return r.incidencias, nil
}

// ResumenIncidenciasFactura redacta las incidencias para el usuario.
func ResumenIncidenciasFactura(incidencias []IncidenciaFactura) string {
	if len(incidencias) == 0 {
		return ""
	}
	errores := 0
	lineas := make([]string, 0, len(incidencias))
	for _, i := range incidencias {
		if i.Nivel == IncidenciaError {
			errores++
		}
		lineas = append(lineas, i.String())
	}
	cabecera := fmt.Sprintf("La revisión de la factura ha encontrado %d incidencias (%d que FACe o el receptor rechazarían). Se ha firmado igualmente porque el contenido es responsabilidad del emisor:", len(incidencias), errores)
	return cabecera + "\n" + strings.Join(lineas, "\n")
}

type revisionFactura struct {
	incidencias []IncidenciaFactura
}

func (r *revisionFactura) add(nivel NivelIncidenciaFactura, campo, formato string, args ...any) {
	if len(r.incidencias) >= 50 {
		return
	}
	r.incidencias = append(r.incidencias, IncidenciaFactura{Nivel: nivel, Campo: campo, Mensaje: fmt.Sprintf(formato, args...)})
}

// num interpreta un importe; los vacíos cuentan como cero.
func (r *revisionFactura) num(v string) float64 {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

func (r *revisionFactura) numValido(campo, v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	if _, err := strconv.ParseFloat(v, 64); err != nil {
		r.add(IncidenciaError, campo, "el importe %q no es un número válido", v)
		return false
	}
	return true
}

func (r *revisionFactura) cuadra(campo, declarado string, calculado float64, descripcion string) {
	if !r.numValido(campo, declarado) {
		if strings.TrimSpace(declarado) == "" {
			r.add(IncidenciaError, campo, "falta el importe")
		}
		return
	}
	if d := r.num(declarado); math.Abs(d-calculado) > toleranciaImporte {
		r.add(IncidenciaError, campo, "vale %s pero %s da %.2f", strings.TrimSpace(declarado), descripcion, calculado)
	}
}

func (r *revisionFactura) cabecera(doc facturaeDocumento) {
	switch strings.TrimSpace(doc.SchemaVersion) {
	case "3.2", "3.2.1", "3.2.2":
	case "":
		r.add(IncidenciaError, "SchemaVersion", "falta la versión del formato Facturae")
	default:
		r.add(IncidenciaAviso, "SchemaVersion", "la versión %s no la admite FACe, que exige Facturae 3.2, 3.2.1 o 3.2.2", strings.TrimSpace(doc.SchemaVersion))
	}
	n, err := strconv.Atoi(strings.TrimSpace(doc.Batch.InvoicesCount))
	if err != nil {
		r.add(IncidenciaError, "Lote/InvoicesCount", "el número de facturas del lote no es válido")
	} else if n != len(doc.Invoices) {
		r.add(IncidenciaError, "Lote/InvoicesCount", "declara %d facturas y el fichero contiene %d", n, len(doc.Invoices))
	}
	if len(doc.Invoices) == 0 {
		r.add(IncidenciaError, "Invoices", "el fichero no contiene ninguna factura")
	}
}

func (r *revisionFactura) parte(nombre string, p facturaeParte) {
	id := p.TaxIdentification
	nif := strings.TrimSpace(id.TaxIdentificationNumber)
	campo := nombre + "/TaxIdentificationNumber"
	if nif == "" {
		r.add(IncidenciaError, campo, "falta el identificador fiscal")
		return
	}
	if strings.TrimSpace(id.ResidenceTypeCode) != "R" {
		return // no residente o residente en la UE: el formato depende del país
	}
	if !IdentificadorFiscalEspanolValido(nif) {
		r.add(IncidenciaAviso, campo, "%q no es un NIF, NIE o CIF español con dígito de control válido", nif)
	}
}

// centrosFACe comprueba los códigos DIR3 que FACe exige para facturar a la
// Administración: oficina contable (01), órgano gestor (02) y unidad
// tramitadora (03). Sin centros se entiende que no es una factura para FACe.
func (r *revisionFactura) centrosFACe(p facturaeParte) {
	if len(p.AdministrativeCentres) == 0 {
		return
	}
	roles := map[string]bool{}
	for _, c := range p.AdministrativeCentres {
		rol := strings.TrimSpace(c.RoleTypeCode)
		if strings.TrimSpace(c.CentreCode) == "" {
			r.add(IncidenciaError, "Receptor/AdministrativeCentre", "hay un centro administrativo con rol %q sin código DIR3", rol)
			continue
		}
		roles[rol] = true
	}
	for rol, nombre := range map[string]string{"01": "oficina contable", "02": "órgano gestor", "03": "unidad tramitadora"} {
		if !roles[rol] {
			r.add(IncidenciaError, "Receptor/AdministrativeCentres", "falta el centro DIR3 de %s (rol %s) que FACe exige", nombre, rol)
		}
	}
}

func (r *revisionFactura) factura(indice int, f facturaeFactura) {
	pref := fmt.Sprintf("Factura %d", indice+1)
	if n := strings.TrimSpace(f.Number); n != "" {
		pref = "Factura " + n
	}
	var bruto float64
	for j, l := range f.Lines {
		campo := fmt.Sprintf("%s/línea %d", pref, j+1)
		coste := r.num(l.TotalCost)
		if l.Quantity != "" && l.UnitPriceWithoutTax != "" {
			if calc := r.num(l.Quantity) * r.num(l.UnitPriceWithoutTax); math.Abs(calc-coste) > toleranciaImporte {
				r.add(IncidenciaAviso, campo+"/TotalCost", "vale %.2f pero cantidad × precio da %.2f", coste, calc)
			}
		}
		esperado := coste
		for _, d := range l.Discounts {
			esperado -= r.num(d.DiscountAmount)
		}
		for _, c := range l.Charges {
			esperado += r.num(c.ChargeAmount)
		}
		r.cuadra(campo+"/GrossAmount", l.GrossAmount, esperado, "el coste menos descuentos más cargos")
		bruto += r.num(l.GrossAmount)
	}
	t := f.Totals
	if len(f.Lines) == 0 {
		r.add(IncidenciaError, pref+"/Items", "la factura no tiene líneas")
	} else {
		r.cuadra(pref+"/TotalGrossAmount", t.TotalGrossAmount, bruto, "la suma de las líneas")
	}
	antes := r.num(t.TotalGrossAmount)
	for _, d := range t.GeneralDiscounts {
		antes -= r.num(d.DiscountAmount)
	}
	for _, c := range t.GeneralSurcharges {
		antes += r.num(c.ChargeAmount)
	}
	r.cuadra(pref+"/TotalGrossAmountBeforeTaxes", t.TotalGrossAmountBeforeTaxes, antes, "el bruto menos descuentos más recargos generales")

	repercutidos := r.impuestos(pref+"/TaxesOutputs", f.TaxesOutputs)
	r.cuadra(pref+"/TotalTaxOutputs", t.TotalTaxOutputs, repercutidos, "la suma de los impuestos repercutidos")
	retenidos := r.impuestos(pref+"/TaxesWithheld", f.TaxesWithheld)
	if strings.TrimSpace(t.TotalTaxesWithheld) != "" || len(f.TaxesWithheld) > 0 {
		r.cuadra(pref+"/TotalTaxesWithheld", t.TotalTaxesWithheld, retenidos, "la suma de las retenciones")
	}
	total := r.num(t.TotalGrossAmountBeforeTaxes) + r.num(t.TotalTaxOutputs) - r.num(t.TotalTaxesWithheld)
	r.cuadra(pref+"/InvoiceTotal", t.InvoiceTotal, total, "base + impuestos − retenciones")
}

func (r *revisionFactura) impuestos(campo string, impuestos []facturaeImpuesto) float64 {
	var suma float64
	for i, tx := range impuestos {
		base := r.num(tx.TaxableBase.TotalAmount)
		cuota := r.num(tx.TaxAmount.TotalAmount)
		if tx.TaxRate != "" {
			if calc := base * r.num(tx.TaxRate) / 100; math.Abs(calc-cuota) > toleranciaImporte {
				r.add(IncidenciaAviso, fmt.Sprintf("%s/%d", campo, i+1), "la cuota %.2f no corresponde a la base %.2f al %s %% (%.2f)", cuota, base, strings.TrimSpace(tx.TaxRate), calc)
			}
		}
		suma += cuota
	}
	return suma
}

// IdentificadorFiscalEspanolValido comprueba el dígito de control de un NIF
// de persona física (DNI), NIE o CIF/NIF de persona jurídica.
func IdentificadorFiscalEspanolValido(v string) bool {
	v = strings.ToUpper(strings.NewReplacer("-", "", " ", "", ".", "").Replace(strings.TrimSpace(v)))
	v = strings.TrimPrefix(v, "ES")
	if len(v) != 9 {
		return false
	}
	const letrasDNI = "TRWAGMYFPDXBNJZSQVHLCKE"
	numero := func(s string) (int, bool) {
		n, err := strconv.Atoi(s)
		return n, err == nil
	}
	switch c := v[0]; {
	case c >= '0' && c <= '9':
		n, ok := numero(v[:8])
		return ok && letrasDNI[n%23] == v[8]
	case c == 'X' || c == 'Y' || c == 'Z':
		n, ok := numero(strconv.Itoa(strings.IndexByte("XYZ", c)) + v[1:8])
		return ok && letrasDNI[n%23] == v[8]
	case c == 'K' || c == 'L' || c == 'M':
		n, ok := numero(v[1:8])
		return ok && letrasDNI[n%23] == v[8]
	case strings.IndexByte("ABCDEFGHJNPQRSUVW", c) >= 0:
		if _, ok := numero(v[1:8]); !ok {
			return false
		}
		suma := 0
		for i := 1; i <= 7; i++ {
			d := int(v[i] - '0')
			if i%2 == 1 {
				d *= 2
				d = d/10 + d%10
			}
			suma += d
		}
		control := (10 - suma%10) % 10
		letra := "JABCDEFGHI"[control]
		switch {
		case strings.IndexByte("PQRSNW", c) >= 0:
			return v[8] == letra
		case strings.IndexByte("ABEH", c) >= 0:
			return v[8] == byte('0'+control)
		default:
			return v[8] == byte('0'+control) || v[8] == letra
		}
	}
	return false
}
