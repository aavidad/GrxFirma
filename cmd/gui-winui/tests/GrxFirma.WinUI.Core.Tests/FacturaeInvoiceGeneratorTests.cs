// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;
using System.Xml.Linq;
using GrxFirma.WinUI.Core.Facturae;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class FacturaeInvoiceGeneratorTests
{
    private static readonly XNamespace Facturae =
        FacturaeInvoiceGenerator.SchemaNamespace;

    [TestMethod]
    public void GenerateBuildsFacturae322WithDir3AndCalculatedTotals()
    {
        var generator = new FacturaeInvoiceGenerator();

        var result = generator.Generate(ValidDraft());
        var document = XDocument.Parse(
            result.Xml,
            LoadOptions.PreserveWhitespace);

        Assert.AreEqual(
            Facturae + "Facturae",
            document.Root?.Name);
        Assert.AreEqual(
            "3.2.2",
            Value(document, "SchemaVersion"));
        CollectionAssert.AreEqual(
            new[] { "01", "02", "03" },
            document
                .Descendants("RoleTypeCode")
                .Select(static element => element.Value)
                .ToArray());
        CollectionAssert.AreEqual(
            new[] { "EA0000001", "EA0000002", "EA0000003" },
            document
                .Descendants("CentreCode")
                .Select(static element => element.Value)
                .ToArray());
        Assert.AreEqual(
            "30.00",
            Value(document, "TotalGrossAmount"));
        Assert.AreEqual(
            "6.30",
            Value(document, "TotalTaxOutputs"));
        Assert.AreEqual(
            "36.30",
            Value(document, "InvoiceTotal"));
        Assert.AreEqual(30.00m, result.Totals.TaxableBase);
        Assert.AreEqual(6.30m, result.Totals.TaxAmount);
        Assert.AreEqual(36.30m, result.Totals.InvoiceTotal);
        StringAssert.Contains(
            result.Xml,
            FacturaeInvoiceGenerator.OfficialSchemaUri);
    }

    [TestMethod]
    public void GenerateGroupsTaxesAndPreservesUpToEightInputDecimals()
    {
        var generator = new FacturaeInvoiceGenerator();
        var original = ValidDraft();
        var draft = original with
        {
            Lines =
            [
                new("Servicio A", 2m, 10m, 21m),
                new("Servicio B", 1m, 10m, 10m),
                new("Servicio C", 1.25m, 8m, 21m),
            ],
        };

        var result = generator.Generate(draft);
        var document = XDocument.Parse(result.Xml);
        var invoiceTaxes = document
            .Root!
            .Element("Invoices")!
            .Element("Invoice")!
            .Element("TaxesOutputs")!
            .Elements("Tax")
            .ToArray();

        Assert.AreEqual(2, invoiceTaxes.Length);
        Assert.AreEqual(
            "10",
            invoiceTaxes[0].Element("TaxRate")?.Value);
        Assert.AreEqual(
            "21",
            invoiceTaxes[1].Element("TaxRate")?.Value);
        Assert.AreEqual(40m, result.Totals.TaxableBase);
        Assert.AreEqual(7.30m, result.Totals.TaxAmount);
        Assert.AreEqual(47.30m, result.Totals.InvoiceTotal);
    }

    [TestMethod]
    public void GenerateSupportsIndividualSellerAndOptionalPayment()
    {
        var generator = new FacturaeInvoiceGenerator();
        var original = ValidDraft();
        var draft = original with
        {
            Seller = original.Seller with
            {
                PersonTypeCode = "F",
                Name = "Ana",
                FirstSurname = "Pruebas",
                SecondSurname = "Firma",
            },
            InstallmentDueDate = new DateOnly(2026, 8, 31),
            Iban = "ES91 2100 0418 4502 0005 1332",
        };

        var document = XDocument.Parse(generator.Generate(draft).Xml);

        Assert.IsNotNull(
            document.Descendants("Individual").SingleOrDefault());
        Assert.AreEqual(
            "ES9121000418450200051332",
            Value(document, "IBAN"));
        Assert.AreEqual("04", Value(document, "PaymentMeans"));
    }

    [TestMethod]
    public void ValidateRejectsBadDir3IbanAndEconomicValues()
    {
        var generator = new FacturaeInvoiceGenerator();
        var original = ValidDraft();
        var draft = original with
        {
            AccountingOfficeDir3 = "NO",
            InstallmentDueDate = original.IssueDate.AddDays(-1),
            Iban = "ES0000000000000000000000",
            Lines =
            [
                new("", 0m, -1m, 101m),
            ],
        };

        var errors = generator.Validate(draft);

        Assert.IsTrue(
            errors.Any(static error => error.Contains(
                "oficina contable",
                StringComparison.Ordinal)));
        Assert.IsTrue(
            errors.Any(static error => error.Contains(
                "IBAN",
                StringComparison.Ordinal)));
        Assert.IsTrue(
            errors.Any(static error => error.Contains(
                "vencimiento",
                StringComparison.Ordinal)));
        Assert.IsTrue(
            errors.Any(static error => error.Contains(
                "cantidad",
                StringComparison.Ordinal)));
        Assert.IsTrue(
            errors.Any(static error => error.Contains(
                "precio",
                StringComparison.Ordinal)));
        Assert.IsTrue(
            errors.Any(static error => error.Contains(
                "IVA",
                StringComparison.Ordinal)));
    }

    [TestMethod]
    public void SuggestedFileNameDoesNotContainAPath()
    {
        var original = ValidDraft();
        var draft = original with
        {
            InvoiceSeriesCode = @"..\SERIE",
            InvoiceNumber = @"1/../../factura",
        };

        var suggested = FacturaeInvoiceGenerator.SuggestedFileName(draft);

        Assert.IsFalse(Path.IsPathFullyQualified(suggested));
        Assert.AreEqual(
            Path.GetFileName(suggested),
            suggested);
    }

    [TestMethod]
    public void GenerateReportsAmountOverflowAsDraftValidation()
    {
        var generator = new FacturaeInvoiceGenerator();
        var original = ValidDraft();
        var draft = original with
        {
            Lines =
            [
                new(
                    "Importe fuera de rango",
                    2m,
                    decimal.MaxValue,
                    21m),
            ],
        };

        var exception = Assert.ThrowsExactly<
            FacturaeValidationException>(
                () => generator.Generate(draft));

        Assert.IsTrue(
            exception.Errors.Any(static error => error.Contains(
                "rango de cálculo",
                StringComparison.Ordinal)));
    }

    private static FacturaeInvoiceDraft ValidDraft() =>
        new(
            InvoiceNumber: "2026-0001",
            InvoiceSeriesCode: "A",
            IssueDate: new DateOnly(2026, 7, 28),
            Seller: new(
                "J",
                "B12345678",
                "Proveedor de pruebas, S.L.",
                "",
                "",
                new(
                    "Calle Pruebas 1",
                    "18001",
                    "Granada",
                    "Granada"),
                "facturas@example.invalid"),
            Buyer: new(
                "J",
                "S4111001F",
                "Administración de pruebas",
                "",
                "",
                new(
                    "Plaza de Pruebas 1",
                    "41001",
                    "Sevilla",
                    "Sevilla")),
            AccountingOfficeDir3: "EA0000001",
            ManagingBodyDir3: "EA0000002",
            ProcessingUnitDir3: "EA0000003",
            Lines:
            [
                new("Servicio de pruebas", 2m, 15m, 21m),
            ],
            InvoiceDescription: "Factura sintética de pruebas",
            FileReference: "EXP-2026-1",
            ReceiverContractReference: "CONT-2026-1");

    private static string? Value(
        XDocument document,
        string localName) =>
        document
            .Descendants(localName)
            .FirstOrDefault()
            ?.Value;
}
