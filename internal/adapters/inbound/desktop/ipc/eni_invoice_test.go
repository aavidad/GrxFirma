// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateInvoiceIPCReportsEngineIssues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invoice.xml")
	invoice := `<Facturae><FileHeader><SchemaVersion>3.2.2</SchemaVersion><Batch><InvoicesCount>1</InvoicesCount><TotalInvoicesAmount><TotalAmount>120.00</TotalAmount></TotalInvoicesAmount></Batch></FileHeader><Parties><SellerParty><TaxIdentification><TaxIdentificationNumber>B99999997</TaxIdentificationNumber></TaxIdentification></SellerParty><BuyerParty><TaxIdentification><TaxIdentificationNumber>12345678Z</TaxIdentificationNumber></TaxIdentification></BuyerParty></Parties><Invoices><Invoice><InvoiceTotals><TotalGrossAmount>100.00</TotalGrossAmount><TotalGrossAmountBeforeTaxes>100.00</TotalGrossAmountBeforeTaxes><TotalTaxOutputs>21.00</TotalTaxOutputs><InvoiceTotal>120.00</InvoiceTotal></InvoiceTotals><Items><InvoiceLine><TotalCost>100.00</TotalCost><GrossAmount>100.00</GrossAmount></InvoiceLine></Items></Invoice></Invoices></Facturae>`
	if err := os.WriteFile(path, []byte(invoice), 0o600); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(invoiceValidationParams{InputPath: path})
	response := (&Manejador{}).despachar(context.Background(), peticion{Action: "validate_invoice", Params: raw})
	if !response.OK {
		t.Fatal(response.Error)
	}
	result, ok := response.Data.(invoiceValidationResult)
	if !ok || result.Format != "FacturaE" || result.Valid || result.Errors == 0 || !strings.Contains(result.Report, "InvoiceTotal") {
		t.Fatalf("unexpected validation result: %+v", response.Data)
	}
}

func TestENIDocumentIPCRejectsInvalidSignatureBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "signature.pdf")
	output := filepath.Join(dir, "eni.xml")
	if err := os.WriteFile(input, []byte("%PDF-1.7\nunsigned"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(eniDocumentParams{InputPath: input, OutputPath: output,
		Options: map[string]string{"eni.organo": "L01180877", "eni.origen": "ciudadano"}})
	response := (&Manejador{}).despachar(context.Background(), peticion{Action: "generate_eni_document", Params: raw})
	if response.OK || response.Error == "" {
		t.Fatalf("unexpected response: %+v", response)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("output created after failure: %v", err)
	}
}

func TestENIFileIPCRejectsOutputInsideInputFolder(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "documents")
	if err := os.Mkdir(input, 0o700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(eniFileParams{DirectoryPath: input,
		OutputPath: filepath.Join(input, "expediente.xml")})
	response := (&Manejador{}).despachar(context.Background(), peticion{Action: "generate_eni_file", Params: raw})
	if response.OK || !strings.Contains(response.Error, "fuera de la carpeta") {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestValidateENIIPC(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eni.xml")
	if err := os.WriteFile(path, []byte(`<documento xmlns="urn:otro"/>`), 0600); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"inputPath": path})
	response := (&Manejador{}).despachar(context.Background(), peticion{Action: "validate_eni", Params: raw})
	if !response.OK {
		t.Fatal(response.Error)
	}
	result, ok := response.Data.(invoiceValidationResult)
	if !ok || result.Valid || result.Errors == 0 {
		t.Fatalf("resultado: %+v", response)
	}
}
