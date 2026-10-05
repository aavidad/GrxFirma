// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const facturaeNamespace = "http://www.facturae.gob.es/formato/Versiones/Facturaev3_2_2.xml"
const facturaeSchema = "https://www.facturae.gob.es/content/dam/facturae/formato/versiones/Facturaev3_2_2.xml"

type facturaeAddress struct {
	Address  string `json:"address"`
	PostCode string `json:"postCode"`
	Town     string `json:"town"`
	Province string `json:"province"`
}
type facturaeParty struct {
	PersonTypeCode          string          `json:"personTypeCode"`
	TaxIdentificationNumber string          `json:"taxIdentificationNumber"`
	Name                    string          `json:"name"`
	FirstSurname            string          `json:"firstSurname"`
	SecondSurname           string          `json:"secondSurname"`
	Address                 facturaeAddress `json:"address"`
	ElectronicMail          string          `json:"electronicMail"`
}
type facturaeLine struct {
	Description         string `json:"description"`
	Quantity            string `json:"quantity"`
	UnitPriceWithoutTax string `json:"unitPriceWithoutTax"`
	VatRate             string `json:"vatRate"`
}
type facturaeDraft struct {
	InvoiceNumber             string         `json:"invoiceNumber"`
	InvoiceSeriesCode         string         `json:"invoiceSeriesCode"`
	IssueDate                 string         `json:"issueDate"`
	Seller                    facturaeParty  `json:"seller"`
	Buyer                     facturaeParty  `json:"buyer"`
	AccountingOfficeDir3      string         `json:"accountingOfficeDir3"`
	ManagingBodyDir3          string         `json:"managingBodyDir3"`
	ProcessingUnitDir3        string         `json:"processingUnitDir3"`
	Lines                     []facturaeLine `json:"lines"`
	InstallmentDueDate        string         `json:"installmentDueDate"`
	Iban                      string         `json:"iban"`
	InvoiceDescription        string         `json:"invoiceDescription"`
	FileReference             string         `json:"fileReference"`
	ReceiverContractReference string         `json:"receiverContractReference"`
}
type facturaeCreateParams struct {
	Draft      facturaeDraft `json:"draft"`
	OutputPath string        `json:"outputPath"`
	// OverwriteConfirmed: la persona confirmó el reemplazo en un diálogo de
	// guardar del sistema. Sin él se aplica la preferencia de sobrescritura.
	OverwriteConfirmed bool `json:"overwriteConfirmed,omitempty"`
}
type facturaeComputedLine struct {
	line      facturaeLine
	base, tax *big.Int
	rate      string
}
type facturaeComputedTax struct {
	rate      string
	base, tax *big.Int
}

var facturaeDir3Pattern = regexp.MustCompile(`^[A-Za-z0-9]{9}$`)
var facturaePostCodePattern = regexp.MustCompile(`^[0-9]{5}$`)
var facturaeEmailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
var facturaeIBANPattern = regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[A-Z0-9]{11,30}$`)

func facturaeDate(value string) (time.Time, error) {
	day, err := time.Parse("2006-01-02", value)
	if err != nil || day.Format("2006-01-02") != value {
		return time.Time{}, errors.New("facturae.error.date")
	}
	return day, nil
}
func facturaeRequired(value string, max int) bool {
	n := len([]rune(strings.TrimSpace(value)))
	return n > 0 && n <= max
}
func facturaeOptional(value string, max int) bool {
	return len([]rune(strings.TrimSpace(value))) <= max
}
func facturaeValidateParty(p facturaeParty) error {
	kind := strings.ToUpper(strings.TrimSpace(p.PersonTypeCode))
	if kind != "F" && kind != "J" {
		return errors.New("facturae.error.party")
	}
	if n := len([]rune(strings.TrimSpace(p.TaxIdentificationNumber))); n < 3 || n > 30 {
		return errors.New("facturae.error.party")
	}
	maxName := 80
	if kind == "F" {
		maxName = 40
	}
	if !facturaeRequired(p.Name, maxName) || (kind == "F" && !facturaeRequired(p.FirstSurname, 40)) || !facturaeOptional(p.SecondSurname, 40) {
		return errors.New("facturae.error.party")
	}
	if !facturaeRequired(p.Address.Address, 80) || !facturaePostCodePattern.MatchString(strings.TrimSpace(p.Address.PostCode)) || !facturaeRequired(p.Address.Town, 50) || !facturaeRequired(p.Address.Province, 20) {
		return errors.New("facturae.error.address")
	}
	if !facturaeOptional(p.ElectronicMail, 60) || (strings.TrimSpace(p.ElectronicMail) != "" && !facturaeEmailPattern.MatchString(strings.TrimSpace(p.ElectronicMail))) {
		return errors.New("facturae.error.email")
	}
	return nil
}
func facturaeValidIBAN(value string) bool {
	if !facturaeIBANPattern.MatchString(value) {
		return false
	}
	rearranged := value[4:] + value[:4]
	mod := 0
	for _, r := range rearranged {
		if r >= '0' && r <= '9' {
			mod = (mod*10 + int(r-'0')) % 97
		} else {
			n := int(r-'A') + 10
			mod = (mod*10 + n/10) % 97
			mod = (mod*10 + n%10) % 97
		}
	}
	return mod == 1
}
func facturaeDecimal(value string) (*big.Rat, error) {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, ".")
	if len(value) > 30 || len(parts) > 2 || len(parts[0]) == 0 || (len(parts) == 2 && len(parts[1]) > 8) {
		return nil, errors.New("facturae.error.amount")
	}
	if len(parts) == 2 && len(parts[1]) == 0 {
		return nil, errors.New("facturae.error.amount")
	}
	for _, part := range parts {
		for _, c := range part {
			if c < '0' || c > '9' {
				return nil, errors.New("facturae.error.amount")
			}
		}
	}
	r := new(big.Rat)
	if _, ok := r.SetString(value); !ok {
		return nil, errors.New("facturae.error.amount")
	}
	return r, nil
}
func facturaeRoundInteger(value *big.Rat) *big.Int {
	n := new(big.Int).Mul(value.Num(), big.NewInt(2))
	d := new(big.Int).Set(value.Denom())
	n.Add(n, d)
	return n.Quo(n, new(big.Int).Mul(d, big.NewInt(2)))
}
func facturaeRoundCents(value *big.Rat) *big.Int {
	return facturaeRoundInteger(new(big.Rat).Mul(value, big.NewRat(100, 1)))
}
func facturaeMoney(cents *big.Int) string {
	whole, fraction := new(big.Int).QuoRem(cents, big.NewInt(100), new(big.Int))
	return fmt.Sprintf("%s.%02d", whole.String(), fraction.Int64())
}
func facturaeFormatDecimal(value string) string {
	if strings.Contains(value, ".") {
		value = strings.TrimRight(value, "0")
		value = strings.TrimRight(value, ".")
	}
	if value == "" {
		return "0"
	}
	return value
}
func facturaeNormalizeIBAN(s string) string {
	return strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(s))
}
func facturaeNormalizeTaxID(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), " ", "")
}

func generateFacturae(d facturaeDraft, now time.Time) ([]byte, string, error) {
	if !facturaeRequired(d.InvoiceNumber, 20) || !facturaeOptional(d.InvoiceSeriesCode, 20) {
		return nil, "", errors.New("facturae.error.number")
	}
	issue, err := facturaeDate(d.IssueDate)
	if err != nil || d.IssueDate > now.In(time.Local).Format("2006-01-02") {
		return nil, "", errors.New("facturae.error.date")
	}
	if err := facturaeValidateParty(d.Seller); err != nil {
		return nil, "", err
	}
	if err := facturaeValidateParty(d.Buyer); err != nil {
		return nil, "", err
	}
	for _, code := range []string{d.AccountingOfficeDir3, d.ManagingBodyDir3, d.ProcessingUnitDir3} {
		if !facturaeDir3Pattern.MatchString(strings.TrimSpace(code)) {
			return nil, "", errors.New("facturae.error.dir3")
		}
	}
	if len(d.Lines) < 1 || len(d.Lines) > 100 {
		return nil, "", errors.New("facturae.error.lines")
	}
	if !facturaeOptional(d.InvoiceDescription, 2500) || !facturaeOptional(d.FileReference, 20) || !facturaeOptional(d.ReceiverContractReference, 20) {
		return nil, "", errors.New("facturae.error.description")
	}
	iban := facturaeNormalizeIBAN(d.Iban)
	if (d.InstallmentDueDate == "") != (iban == "") {
		return nil, "", errors.New("facturae.error.payment")
	}
	if iban != "" {
		due, err := facturaeDate(d.InstallmentDueDate)
		if err != nil || due.Before(issue) || !facturaeValidIBAN(iban) {
			return nil, "", errors.New("facturae.error.payment")
		}
	}
	calculated := make([]facturaeComputedLine, 0, len(d.Lines))
	taxes := make([]facturaeComputedTax, 0)
	baseTotal := big.NewInt(0)
	taxTotal := big.NewInt(0)
	for _, line := range d.Lines {
		if !facturaeRequired(line.Description, 2500) {
			return nil, "", errors.New("facturae.error.lines")
		}
		quantity, errQ := facturaeDecimal(line.Quantity)
		price, errP := facturaeDecimal(line.UnitPriceWithoutTax)
		rate, errR := facturaeDecimal(line.VatRate)
		if errQ != nil || errP != nil || errR != nil || quantity.Sign() <= 0 || rate.Cmp(big.NewRat(100, 1)) > 0 {
			return nil, "", errors.New("facturae.error.amount")
		}
		base := facturaeRoundCents(new(big.Rat).Mul(quantity, price))
		tax := facturaeRoundInteger(new(big.Rat).Quo(new(big.Rat).Mul(new(big.Rat).SetInt(base), rate), big.NewRat(100, 1)))
		if base.Cmp(big.NewInt(99999999999900)) > 0 {
			return nil, "", errors.New("facturae.error.amount")
		}
		rateKey := rate.RatString()
		calculated = append(calculated, facturaeComputedLine{line, base, tax, rateKey})
		baseTotal.Add(baseTotal, base)
		taxTotal.Add(taxTotal, tax)
		found := false
		for i := range taxes {
			if taxes[i].rate == rateKey {
				taxes[i].base.Add(taxes[i].base, base)
				taxes[i].tax.Add(taxes[i].tax, tax)
				found = true
				break
			}
		}
		if !found {
			taxes = append(taxes, facturaeComputedTax{rateKey, new(big.Int).Set(base), new(big.Int).Set(tax)})
		}
	}
	sort.Slice(taxes, func(i, j int) bool {
		a, _ := new(big.Rat).SetString(taxes[i].rate)
		b, _ := new(big.Rat).SetString(taxes[j].rate)
		return a.Cmp(b) < 0
	})
	total := new(big.Int).Add(baseTotal, taxTotal)
	if total.Cmp(big.NewInt(99999999999900)) > 0 {
		return nil, "", errors.New("facturae.error.amount")
	}
	xmlData, err := facturaeXML(d, calculated, taxes, baseTotal, taxTotal, total, iban)
	if err != nil {
		return nil, "", err
	}
	return xmlData, facturaeMoney(total), nil
}

// XML is assembled with encoding/xml, so all user fields are escaped as text.
func facturaeXML(d facturaeDraft, lines []facturaeComputedLine, taxes []facturaeComputedTax, base, tax, total *big.Int, iban string) ([]byte, error) {
	var out bytes.Buffer
	out.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	e := xml.NewEncoder(&out)
	e.Indent("", "  ")
	var stack []xml.StartElement
	start := func(name string, attrs ...xml.Attr) error {
		token := xml.StartElement{Name: xml.Name{Local: name}, Attr: attrs}
		stack = append(stack, token)
		return e.EncodeToken(token)
	}
	end := func() error {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return e.EncodeToken(top.End())
	}
	elem := func(name, value string) error {
		if err := start(name); err != nil {
			return err
		}
		if err := e.EncodeToken(xml.CharData([]byte(strings.TrimSpace(value)))); err != nil {
			return err
		}
		return end()
	}
	amount := func(name string, value *big.Int) error {
		if err := start(name); err != nil {
			return err
		}
		if err := elem("TotalAmount", facturaeMoney(value)); err != nil {
			return err
		}
		return end()
	}
	address := func(a facturaeAddress) error {
		if err := start("AddressInSpain"); err != nil {
			return err
		}
		for _, item := range [][2]string{{"Address", a.Address}, {"PostCode", a.PostCode}, {"Town", a.Town}, {"Province", a.Province}, {"CountryCode", "ESP"}} {
			if err := elem(item[0], item[1]); err != nil {
				return err
			}
		}
		return end()
	}
	party := func(name string, p facturaeParty, centres bool) error {
		if err := start(name); err != nil {
			return err
		}
		if err := start("TaxIdentification"); err != nil {
			return err
		}
		for _, item := range [][2]string{{"PersonTypeCode", strings.ToUpper(p.PersonTypeCode)}, {"ResidenceTypeCode", "R"}, {"TaxIdentificationNumber", facturaeNormalizeTaxID(p.TaxIdentificationNumber)}} {
			if err := elem(item[0], item[1]); err != nil {
				return err
			}
		}
		if err := end(); err != nil {
			return err
		}
		if centres {
			if err := start("AdministrativeCentres"); err != nil {
				return err
			}
			for _, centre := range [][3]string{{d.AccountingOfficeDir3, "01", "Oficina contable"}, {d.ManagingBodyDir3, "02", "Órgano gestor"}, {d.ProcessingUnitDir3, "03", "Unidad tramitadora"}} {
				if err := start("AdministrativeCentre"); err != nil {
					return err
				}
				for _, item := range [][2]string{{"CentreCode", strings.ToUpper(centre[0])}, {"RoleTypeCode", centre[1]}, {"Name", centre[2]}} {
					if err := elem(item[0], item[1]); err != nil {
						return err
					}
				}
				if err := address(p.Address); err != nil {
					return err
				}
				if err := end(); err != nil {
					return err
				}
			}
			if err := end(); err != nil {
				return err
			}
		}
		if strings.ToUpper(p.PersonTypeCode) == "F" {
			if err := start("Individual"); err != nil {
				return err
			}
			for _, item := range [][2]string{{"Name", p.Name}, {"FirstSurname", p.FirstSurname}} {
				if err := elem(item[0], item[1]); err != nil {
					return err
				}
			}
			if p.SecondSurname != "" {
				if err := elem("SecondSurname", p.SecondSurname); err != nil {
					return err
				}
			}
		} else {
			if err := start("LegalEntity"); err != nil {
				return err
			}
			if err := elem("CorporateName", p.Name); err != nil {
				return err
			}
		}
		if err := address(p.Address); err != nil {
			return err
		}
		if p.ElectronicMail != "" {
			if err := start("ContactDetails"); err != nil {
				return err
			}
			if err := elem("ElectronicMail", p.ElectronicMail); err != nil {
				return err
			}
			if err := end(); err != nil {
				return err
			}
		}
		if err := end(); err != nil {
			return err
		}
		return end()
	}
	writeTax := func(rate string, taxBase, taxAmount *big.Int) error {
		if err := start("Tax"); err != nil {
			return err
		}
		r := new(big.Rat)
		r.SetString(rate)
		display := r.FloatString(8)
		if err := elem("TaxTypeCode", "01"); err != nil {
			return err
		}
		if err := elem("TaxRate", facturaeFormatDecimal(display)); err != nil {
			return err
		}
		if err := amount("TaxableBase", taxBase); err != nil {
			return err
		}
		if err := amount("TaxAmount", taxAmount); err != nil {
			return err
		}
		return end()
	}
	attrs := []xml.Attr{{Name: xml.Name{Local: "xmlns"}, Value: facturaeNamespace}, {Name: xml.Name{Local: "xmlns:xsi"}, Value: "http://www.w3.org/2001/XMLSchema-instance"}, {Name: xml.Name{Local: "xsi:schemaLocation"}, Value: facturaeNamespace + " " + facturaeSchema}}
	if err := start("Facturae", attrs...); err != nil {
		return nil, err
	}
	if err := start("FileHeader"); err != nil {
		return nil, err
	}
	for _, item := range [][2]string{{"SchemaVersion", "3.2.2"}, {"Modality", "I"}, {"InvoiceIssuerType", "EM"}} {
		if err := elem(item[0], item[1]); err != nil {
			return nil, err
		}
	}
	if err := start("Batch"); err != nil {
		return nil, err
	}
	if err := elem("BatchIdentifier", facturaeNormalizeTaxID(d.Seller.TaxIdentificationNumber)+strings.TrimSpace(d.InvoiceNumber)+strings.TrimSpace(d.InvoiceSeriesCode)); err != nil {
		return nil, err
	}
	if err := elem("InvoicesCount", "1"); err != nil {
		return nil, err
	}
	for _, name := range []string{"TotalInvoicesAmount", "TotalOutstandingAmount", "TotalExecutableAmount"} {
		if err := amount(name, total); err != nil {
			return nil, err
		}
	}
	if err := elem("InvoiceCurrencyCode", "EUR"); err != nil {
		return nil, err
	}
	end()
	end()
	start("Parties")
	if err := party("SellerParty", d.Seller, false); err != nil {
		return nil, err
	}
	if err := party("BuyerParty", d.Buyer, true); err != nil {
		return nil, err
	}
	end()
	start("Invoices")
	start("Invoice")
	start("InvoiceHeader")
	elem("InvoiceNumber", d.InvoiceNumber)
	if d.InvoiceSeriesCode != "" {
		elem("InvoiceSeriesCode", d.InvoiceSeriesCode)
	}
	elem("InvoiceDocumentType", "FC")
	elem("InvoiceClass", "OO")
	end()
	start("InvoiceIssueData")
	for _, item := range [][2]string{{"IssueDate", d.IssueDate}, {"InvoiceCurrencyCode", "EUR"}, {"TaxCurrencyCode", "EUR"}, {"LanguageName", "es"}, {"InvoiceDescription", d.InvoiceDescription}, {"FileReference", d.FileReference}, {"ReceiverContractReference", d.ReceiverContractReference}} {
		if item[1] != "" {
			if err := elem(item[0], item[1]); err != nil {
				return nil, err
			}
		}
	}
	end()
	start("TaxesOutputs")
	for _, group := range taxes {
		if err := writeTax(group.rate, group.base, group.tax); err != nil {
			return nil, err
		}
	}
	end()
	start("InvoiceTotals")
	for _, item := range []struct {
		name  string
		value *big.Int
	}{{"TotalGrossAmount", base}, {"TotalGrossAmountBeforeTaxes", base}, {"TotalTaxOutputs", tax}, {"TotalTaxesWithheld", big.NewInt(0)}, {"InvoiceTotal", total}, {"TotalOutstandingAmount", total}, {"TotalExecutableAmount", total}} {
		if err := elem(item.name, facturaeMoney(item.value)); err != nil {
			return nil, err
		}
	}
	end()
	start("Items")
	for _, item := range lines {
		start("InvoiceLine")
		elem("ItemDescription", item.line.Description)
		elem("Quantity", facturaeFormatDecimal(item.line.Quantity))
		elem("UnitPriceWithoutTax", facturaeFormatDecimal(item.line.UnitPriceWithoutTax))
		elem("TotalCost", facturaeMoney(item.base))
		elem("GrossAmount", facturaeMoney(item.base))
		start("TaxesOutputs")
		if err := writeTax(item.rate, item.base, item.tax); err != nil {
			return nil, err
		}
		end()
		end()
	}
	end()
	if iban != "" {
		start("PaymentDetails")
		start("Installment")
		elem("InstallmentDueDate", d.InstallmentDueDate)
		elem("InstallmentAmount", facturaeMoney(total))
		elem("PaymentMeans", "04")
		start("AccountToBeCredited")
		elem("IBAN", iban)
		end()
		end()
		end()
	}
	end()
	end()
	end()
	if err := e.Flush(); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

func (m *Manejador) handleFacturaeCreate(ctx context.Context, raw json.RawMessage) respuesta {
	const action = "facturae_create"
	var params facturaeCreateParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{Action: action, Error: "facturae.error.input"}
	}
	if err := ctx.Err(); err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	if params.OutputPath != "" {
		if !strings.EqualFold(filepath.Ext(params.OutputPath), ".xml") {
			return respuesta{Action: action, Error: "facturae.error.output"}
		}
		if err := validarRutaEscritura(params.OutputPath); err != nil {
			return respuesta{Action: action, Error: "facturae.error.output"}
		}
	}
	data, total, err := generateFacturae(params.Draft, time.Now())
	if err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	if err := ctx.Err(); err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	if params.OutputPath == "" {
		return respuesta{OK: true, Action: action, Data: map[string]string{"xml": string(data), "total": total}}
	}
	finalPath, err := m.guardarSalidaIPC(ctx, params.OutputPath, "", params.OverwriteConfirmed, data)
	if err != nil {
		return respuesta{Action: action, Error: m.mensajeErrorSalidaIPC(err, params.OutputPath, "facturae.error.output")}
	}
	return respuesta{OK: true, Action: action, Data: map[string]string{"outputPath": finalPath, "total": total}}
}
