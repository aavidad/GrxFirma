// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunValidarFactura(t *testing.T) {
	// Factura con el total descuadrado: 100 + 21 no son 120.
	factura := `<Facturae><FileHeader><SchemaVersion>3.2.2</SchemaVersion><Batch><InvoicesCount>1</InvoicesCount>
<TotalInvoicesAmount><TotalAmount>120.00</TotalAmount></TotalInvoicesAmount>
<TotalOutstandingAmount><TotalAmount>120.00</TotalAmount></TotalOutstandingAmount>
<TotalExecutableAmount><TotalAmount>120.00</TotalAmount></TotalExecutableAmount></Batch></FileHeader>
<Parties><SellerParty><TaxIdentification><ResidenceTypeCode>R</ResidenceTypeCode><TaxIdentificationNumber>B99999997</TaxIdentificationNumber></TaxIdentification></SellerParty>
<BuyerParty><TaxIdentification><ResidenceTypeCode>R</ResidenceTypeCode><TaxIdentificationNumber>12345678Z</TaxIdentificationNumber></TaxIdentification></BuyerParty></Parties>
<Invoices><Invoice><TaxesOutputs><Tax><TaxRate>21.00</TaxRate><TaxableBase><TotalAmount>100.00</TotalAmount></TaxableBase><TaxAmount><TotalAmount>21.00</TotalAmount></TaxAmount></Tax></TaxesOutputs>
<InvoiceTotals><TotalGrossAmount>100.00</TotalGrossAmount><TotalGrossAmountBeforeTaxes>100.00</TotalGrossAmountBeforeTaxes><TotalTaxOutputs>21.00</TotalTaxOutputs>
<InvoiceTotal>120.00</InvoiceTotal><TotalOutstandingAmount>120.00</TotalOutstandingAmount><TotalExecutableAmount>120.00</TotalExecutableAmount></InvoiceTotals>
<Items><InvoiceLine><TotalCost>100.00</TotalCost><GrossAmount>100.00</GrossAmount></InvoiceLine></Items></Invoice></Invoices></Facturae>`
	entrada := filepath.Join(t.TempDir(), "factura.xml")
	if err := os.WriteFile(entrada, []byte(factura), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr strings.Builder
	a := New(nil, nil)
	a.Stdout, a.Stderr = &stdout, &stderr
	rc := a.Run(context.Background(), []string{"-operacion", "validar-factura", "-entrada", entrada})
	if rc != 1 || !strings.Contains(stdout.String(), "InvoiceTotal") {
		t.Fatalf("rc=%d stdout=%q stderr=%q", rc, stdout.String(), stderr.String())
	}

	corregida := strings.ReplaceAll(factura, "120.00", "121.00")
	if err := os.WriteFile(entrada, []byte(corregida), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if rc := a.Run(context.Background(), []string{"-operacion", "validar-factura", "-entrada", entrada}); rc != 0 {
		t.Fatalf("rc=%d stdout=%q", rc, stdout.String())
	}
}
