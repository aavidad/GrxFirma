// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/signer"
)

func facturaeSampleDraft() facturaeDraft {
	return facturaeDraft{
		InvoiceNumber: "F-2026-1", IssueDate: "2026-10-02",
		Seller:               facturaeParty{PersonTypeCode: "J", TaxIdentificationNumber: "B12345678", Name: "Emisor SL", Address: facturaeAddress{"Calle Uno 1", "18001", "Granada", "Granada"}},
		Buyer:                facturaeParty{PersonTypeCode: "J", TaxIdentificationNumber: "B87654321", Name: "Receptor SL", Address: facturaeAddress{"Calle Dos 2", "28001", "Madrid", "Madrid"}},
		AccountingOfficeDir3: "A12345678", ManagingBodyDir3: "B12345678", ProcessingUnitDir3: "C12345678",
		Lines: []facturaeLine{{Description: "Servicio & mantenimiento", Quantity: "2", UnitPriceWithoutTax: "50.005", VatRate: "21"}},
	}
}
func TestGenerateFacturaeProducesBalancedXML(t *testing.T) {
	data, total, err := generateFacturae(facturaeSampleDraft(), time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if total != "121.01" {
		t.Fatalf("total=%s", total)
	}
	if !strings.Contains(string(data), "Servicio &amp; mantenimiento") {
		t.Fatal("texto XML sin escapar")
	}
	var root struct {
		XMLName xml.Name
		Version string `xml:"FileHeader>SchemaVersion"`
		Total   string `xml:"Invoices>Invoice>InvoiceTotals>InvoiceTotal"`
	}
	if err := xml.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	if root.XMLName.Space != facturaeNamespace || root.Version != "3.2.2" || root.Total != "121.01" {
		t.Fatalf("estructura inesperada: %+v", root)
	}
	incidents, err := signer.RevisarFacturaE(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, incident := range incidents {
		if incident.Nivel == signer.IncidenciaError {
			t.Fatalf("factura inválida: %v", incident)
		}
	}
}
func TestGenerateFacturaeRejectsInvalidPaymentAndAmount(t *testing.T) {
	draft := facturaeSampleDraft()
	draft.Iban = "ES9121000418450200051332"
	if _, _, err := generateFacturae(draft, time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)); err == nil || err.Error() != "facturae.error.payment" {
		t.Fatalf("pago: %v", err)
	}
	draft = facturaeSampleDraft()
	draft.Lines[0].Quantity = "100000000000000000"
	if _, _, err := generateFacturae(draft, time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("importe desbordado aceptado")
	}
}

func TestHandleFacturaeCreateSupportsBothFrontends(t *testing.T) {
	draft := facturaeSampleDraft()
	output := filepath.Join(t.TempDir(), "factura.xml")
	raw, err := json.Marshal(facturaeCreateParams{Draft: draft, OutputPath: output})
	if err != nil {
		t.Fatal(err)
	}
	response := (&Manejador{}).despachar(context.Background(), peticion{Action: "facturae_create", Params: raw})
	if !response.OK {
		t.Fatalf("Qt: %+v", response)
	}
	file, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(file), "<InvoiceTotal>121.01</InvoiceTotal>") {
		t.Fatalf("salida: %s", file)
	}
	info, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	// Windows no tiene permisos Unix: allí la protección la da el perfil del usuario.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("permisos: %v", info.Mode().Perm())
	}
	raw, err = json.Marshal(facturaeCreateParams{Draft: draft})
	if err != nil {
		t.Fatal(err)
	}
	response = (&Manejador{}).despachar(context.Background(), peticion{Action: "facturae_create", Params: raw})
	result, ok := response.Data.(map[string]string)
	if !response.OK || !ok || !strings.Contains(result["xml"], "<Facturae") {
		t.Fatalf("WinUI: %+v", response)
	}
	draft.AccountingOfficeDir3 = "invalid"
	raw, _ = json.Marshal(facturaeCreateParams{Draft: draft, OutputPath: output})
	response = (&Manejador{}).despachar(context.Background(), peticion{Action: "facturae_create", Params: raw})
	if response.OK || response.ErrorCode != "facturae.error.dir3" || response.Diagnostic == nil {
		t.Fatalf("validación: %+v", response)
	}
}
