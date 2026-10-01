// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;
using System.Text.RegularExpressions;
using System.Xml;
using System.Xml.Linq;

namespace GrxFirma.WinUI.Core.Facturae;

public sealed record FacturaeAddressDraft(
    string Address,
    string PostCode,
    string Town,
    string Province);

public sealed record FacturaePartyDraft(
    string PersonTypeCode,
    string TaxIdentificationNumber,
    string Name,
    string FirstSurname,
    string SecondSurname,
    FacturaeAddressDraft Address,
    string ElectronicMail = "");

public sealed record FacturaeLineDraft(
    string Description,
    decimal Quantity,
    decimal UnitPriceWithoutTax,
    decimal VatRate);

public sealed record FacturaeInvoiceDraft(
    string InvoiceNumber,
    string InvoiceSeriesCode,
    DateOnly IssueDate,
    FacturaePartyDraft Seller,
    FacturaePartyDraft Buyer,
    string AccountingOfficeDir3,
    string ManagingBodyDir3,
    string ProcessingUnitDir3,
    IReadOnlyList<FacturaeLineDraft> Lines,
    DateOnly? InstallmentDueDate = null,
    string Iban = "",
    string InvoiceDescription = "",
    string FileReference = "",
    string ReceiverContractReference = "");

public sealed record FacturaeInvoiceTotals(
    decimal TaxableBase,
    decimal TaxAmount,
    decimal InvoiceTotal);

public sealed record FacturaeGenerationResult(
    string Xml,
    FacturaeInvoiceTotals Totals);

public sealed class FacturaeValidationException(
    IReadOnlyList<string> errors)
    : ArgumentException(
        string.Join(Environment.NewLine, errors),
        nameof(FacturaeInvoiceDraft))
{
    public IReadOnlyList<string> Errors { get; } = errors;
}

public sealed partial class FacturaeInvoiceGenerator
{
    public const string SchemaVersion = "3.2.2";
    public const string SchemaNamespace =
        "http://www.facturae.gob.es/formato/Versiones/Facturaev3_2_2.xml";
    public const string OfficialSchemaUri =
        "https://www.facturae.gob.es/content/dam/facturae/formato/versiones/Facturaev3_2_2.xml";

    private const int MaximumLines = 100;
    private const decimal MaximumSupportedAmount = 999_999_999_999m;
    private const string CurrencyCode = "EUR";
    private const string CountryCode = "ESP";
    private const string LanguageCode = "es";
    private static readonly XNamespace FacturaeNamespace = SchemaNamespace;
    private static readonly XNamespace XsiNamespace =
        "http://www.w3.org/2001/XMLSchema-instance";

    public FacturaeGenerationResult Generate(
        FacturaeInvoiceDraft draft)
    {
        ArgumentNullException.ThrowIfNull(draft);
        var errors = Validate(draft);
        if (errors.Count != 0)
        {
            throw new FacturaeValidationException(errors);
        }

        CalculatedLine[] calculatedLines;
        decimal taxableBase;
        CalculatedTax[] taxGroups;
        decimal taxAmount;
        decimal invoiceTotal;
        try
        {
            calculatedLines = draft.Lines
                .Select(CalculateLine)
                .ToArray();
            taxableBase = RoundMoney(
                calculatedLines.Sum(
                    static line => line.GrossAmount));
            taxGroups = calculatedLines
                .GroupBy(static line => line.Source.VatRate)
                .OrderBy(static group => group.Key)
                .Select(static group => new CalculatedTax(
                    group.Key,
                    RoundMoney(
                        group.Sum(
                            static line => line.GrossAmount)),
                    RoundMoney(
                        group.Sum(
                            static line => line.TaxAmount))))
                .ToArray();
            taxAmount = RoundMoney(
                taxGroups.Sum(static tax => tax.TaxAmount));
            invoiceTotal = RoundMoney(taxableBase + taxAmount);
        }
        catch (OverflowException)
        {
            throw new FacturaeValidationException(
            [
                "Los importes de la factura superan el rango de cálculo admitido.",
            ]);
        }
        if (invoiceTotal > MaximumSupportedAmount)
        {
            throw new FacturaeValidationException(
            [
                "El total de la factura supera el límite admitido por este generador.",
            ]);
        }

        var document = BuildDocument(
            draft,
            calculatedLines,
            taxGroups,
            taxableBase,
            taxAmount,
            invoiceTotal);
        var xml = Serialize(document);
        return new(
            xml,
            new(taxableBase, taxAmount, invoiceTotal));
    }

    public IReadOnlyList<string> Validate(
        FacturaeInvoiceDraft draft)
    {
        ArgumentNullException.ThrowIfNull(draft);
        var errors = new List<string>();

        ValidateRequiredText(
            errors,
            draft.InvoiceNumber,
            "El número de factura",
            20);
        ValidateOptionalText(
            errors,
            draft.InvoiceSeriesCode,
            "La serie de factura",
            20);
        if (draft.IssueDate > DateOnly.FromDateTime(DateTime.Today))
        {
            errors.Add(
                "La fecha de expedición no puede ser posterior a hoy.");
        }

        ValidateParty(errors, draft.Seller, "emisor");
        ValidateParty(errors, draft.Buyer, "receptor");
        ValidateDir3(
            errors,
            draft.AccountingOfficeDir3,
            "oficina contable");
        ValidateDir3(
            errors,
            draft.ManagingBodyDir3,
            "órgano gestor");
        ValidateDir3(
            errors,
            draft.ProcessingUnitDir3,
            "unidad tramitadora");

        if (draft.Lines is null || draft.Lines.Count == 0)
        {
            errors.Add("La factura debe contener al menos una línea.");
        }
        else if (draft.Lines.Count > MaximumLines)
        {
            errors.Add(
                $"La factura no puede superar {MaximumLines} líneas.");
        }
        else
        {
            for (var index = 0; index < draft.Lines.Count; index++)
            {
                ValidateLine(errors, draft.Lines[index], index + 1);
            }
        }

        ValidateOptionalText(
            errors,
            draft.InvoiceDescription,
            "La descripción general",
            2500);
        ValidateOptionalText(
            errors,
            draft.FileReference,
            "La referencia de expediente",
            20);
        ValidateOptionalText(
            errors,
            draft.ReceiverContractReference,
            "La referencia de contrato",
            20);

        var normalizedIban = NormalizeIban(draft.Iban);
        if (draft.InstallmentDueDate.HasValue !=
            !string.IsNullOrEmpty(normalizedIban))
        {
            errors.Add(
                "Para incluir el pago indique juntos el vencimiento y el IBAN.");
        }
        if (draft.InstallmentDueDate is { } dueDate &&
            dueDate < draft.IssueDate)
        {
            errors.Add(
                "La fecha de vencimiento no puede ser anterior a la expedición.");
        }
        if (!string.IsNullOrEmpty(normalizedIban) &&
            !IsValidIban(normalizedIban))
        {
            errors.Add("El IBAN no es válido.");
        }

        return errors;
    }

    public static string SuggestedFileName(
        FacturaeInvoiceDraft draft)
    {
        ArgumentNullException.ThrowIfNull(draft);
        var identity = string.Concat(
            draft.InvoiceSeriesCode.Trim(),
            string.IsNullOrWhiteSpace(draft.InvoiceSeriesCode)
                ? ""
                : "-",
            draft.InvoiceNumber.Trim());
        var safeIdentity = SuggestedNameUnsafeCharacters()
            .Replace(identity, "-")
            .Replace("..", "-", StringComparison.Ordinal)
            .Trim(' ', '.', '-');
        if (safeIdentity.Length > 80)
        {
            safeIdentity = safeIdentity[..80].TrimEnd(' ', '.', '-');
        }
        return string.IsNullOrWhiteSpace(safeIdentity)
            ? "facturae-3.2.2.xml"
            : $"facturae-{safeIdentity}.xml";
    }

    private static XDocument BuildDocument(
        FacturaeInvoiceDraft draft,
        IReadOnlyList<CalculatedLine> calculatedLines,
        IReadOnlyList<CalculatedTax> taxGroups,
        decimal taxableBase,
        decimal taxAmount,
        decimal invoiceTotal)
    {
        var batchIdentifier = string.Concat(
            NormalizeTaxId(draft.Seller.TaxIdentificationNumber),
            draft.InvoiceNumber.Trim(),
            draft.InvoiceSeriesCode.Trim());
        var root = new XElement(
            FacturaeNamespace + "Facturae",
            new XAttribute(
                XNamespace.Xmlns + "xsi",
                XsiNamespace),
            new XAttribute(
                XsiNamespace + "schemaLocation",
                $"{SchemaNamespace} {OfficialSchemaUri}"),
            Element(
                "FileHeader",
                Element("SchemaVersion", SchemaVersion),
                Element("Modality", "I"),
                Element("InvoiceIssuerType", "EM"),
                Element(
                    "Batch",
                    Element("BatchIdentifier", batchIdentifier),
                    Element("InvoicesCount", "1"),
                    AmountElement("TotalInvoicesAmount", invoiceTotal),
                    AmountElement("TotalOutstandingAmount", invoiceTotal),
                    AmountElement("TotalExecutableAmount", invoiceTotal),
                    Element("InvoiceCurrencyCode", CurrencyCode))),
            Element(
                "Parties",
                BuildParty("SellerParty", draft.Seller, null),
                BuildParty(
                    "BuyerParty",
                    draft.Buyer,
                    new[]
                    {
                        new AdministrativeCentre(
                            draft.AccountingOfficeDir3,
                            "01",
                            "Oficina contable"),
                        new AdministrativeCentre(
                            draft.ManagingBodyDir3,
                            "02",
                            "Órgano gestor"),
                        new AdministrativeCentre(
                            draft.ProcessingUnitDir3,
                            "03",
                            "Unidad tramitadora"),
                    })),
            Element(
                "Invoices",
                Element(
                    "Invoice",
                    BuildInvoiceHeader(draft),
                    BuildIssueData(draft),
                    Element(
                        "TaxesOutputs",
                        taxGroups.Select(BuildTax)),
                    BuildTotals(
                        taxableBase,
                        taxAmount,
                        invoiceTotal),
                    Element(
                        "Items",
                        calculatedLines.Select(BuildLine)),
                    BuildPaymentDetails(draft, invoiceTotal))));

        return new XDocument(
            new XDeclaration("1.0", "utf-8", null),
            root);
    }

    private static XElement BuildParty(
        string elementName,
        FacturaePartyDraft party,
        IReadOnlyList<AdministrativeCentre>? centres)
    {
        var content = new List<object>
        {
            Element(
                "TaxIdentification",
                Element(
                    "PersonTypeCode",
                    party.PersonTypeCode.Trim().ToUpperInvariant()),
                Element("ResidenceTypeCode", "R"),
                Element(
                    "TaxIdentificationNumber",
                    NormalizeTaxId(party.TaxIdentificationNumber))),
        };
        if (centres is not null)
        {
            content.Add(
                Element(
                    "AdministrativeCentres",
                    centres.Select(
                        centre => Element(
                            "AdministrativeCentre",
                            Element("CentreCode", centre.Code.Trim().ToUpperInvariant()),
                            Element("RoleTypeCode", centre.Role),
                            Element("Name", centre.Name),
                            BuildAddress(party.Address)))));
        }

        content.Add(BuildIdentity(party));
        return Element(elementName, content);
    }

    private static XElement BuildIdentity(
        FacturaePartyDraft party)
    {
        var content = new List<object>();
        if (string.Equals(
                party.PersonTypeCode.Trim(),
                "F",
                StringComparison.OrdinalIgnoreCase))
        {
            content.Add(Element("Name", party.Name.Trim()));
            content.Add(
                Element("FirstSurname", party.FirstSurname.Trim()));
            if (!string.IsNullOrWhiteSpace(party.SecondSurname))
            {
                content.Add(
                    Element(
                        "SecondSurname",
                        party.SecondSurname.Trim()));
            }
        }
        else
        {
            content.Add(Element("CorporateName", party.Name.Trim()));
        }
        content.Add(BuildAddress(party.Address));
        if (!string.IsNullOrWhiteSpace(party.ElectronicMail))
        {
            content.Add(
                Element(
                    "ContactDetails",
                    Element(
                        "ElectronicMail",
                        party.ElectronicMail.Trim())));
        }

        return Element(
            string.Equals(
                party.PersonTypeCode.Trim(),
                "F",
                StringComparison.OrdinalIgnoreCase)
                ? "Individual"
                : "LegalEntity",
            content);
    }

    private static XElement BuildAddress(
        FacturaeAddressDraft address) =>
        Element(
            "AddressInSpain",
            Element("Address", address.Address.Trim()),
            Element("PostCode", address.PostCode.Trim()),
            Element("Town", address.Town.Trim()),
            Element("Province", address.Province.Trim()),
            Element("CountryCode", CountryCode));

    private static XElement BuildInvoiceHeader(
        FacturaeInvoiceDraft draft)
    {
        var content = new List<object>
        {
            Element("InvoiceNumber", draft.InvoiceNumber.Trim()),
        };
        if (!string.IsNullOrWhiteSpace(draft.InvoiceSeriesCode))
        {
            content.Add(
                Element(
                    "InvoiceSeriesCode",
                    draft.InvoiceSeriesCode.Trim()));
        }
        content.Add(Element("InvoiceDocumentType", "FC"));
        content.Add(Element("InvoiceClass", "OO"));
        return Element("InvoiceHeader", content);
    }

    private static XElement BuildIssueData(
        FacturaeInvoiceDraft draft)
    {
        var content = new List<object>
        {
            Element(
                "IssueDate",
                draft.IssueDate.ToString(
                    "yyyy-MM-dd",
                    CultureInfo.InvariantCulture)),
            Element("InvoiceCurrencyCode", CurrencyCode),
            Element("TaxCurrencyCode", CurrencyCode),
            Element("LanguageName", LanguageCode),
        };
        AddOptional(
            content,
            "InvoiceDescription",
            draft.InvoiceDescription);
        AddOptional(
            content,
            "FileReference",
            draft.FileReference);
        AddOptional(
            content,
            "ReceiverContractReference",
            draft.ReceiverContractReference);
        return Element("InvoiceIssueData", content);
    }

    private static XElement BuildTotals(
        decimal taxableBase,
        decimal taxAmount,
        decimal invoiceTotal) =>
        Element(
            "InvoiceTotals",
            Element("TotalGrossAmount", FormatAmount(taxableBase)),
            Element(
                "TotalGrossAmountBeforeTaxes",
                FormatAmount(taxableBase)),
            Element("TotalTaxOutputs", FormatAmount(taxAmount)),
            Element("TotalTaxesWithheld", "0.00"),
            Element("InvoiceTotal", FormatAmount(invoiceTotal)),
            Element(
                "TotalOutstandingAmount",
                FormatAmount(invoiceTotal)),
            Element(
                "TotalExecutableAmount",
                FormatAmount(invoiceTotal)));

    private static XElement BuildLine(
        CalculatedLine line) =>
        Element(
            "InvoiceLine",
            Element("ItemDescription", line.Source.Description.Trim()),
            Element(
                "Quantity",
                FormatDecimal(line.Source.Quantity)),
            Element(
                "UnitPriceWithoutTax",
                FormatDecimal(line.Source.UnitPriceWithoutTax)),
            Element("TotalCost", FormatAmount(line.GrossAmount)),
            Element("GrossAmount", FormatAmount(line.GrossAmount)),
            Element(
                "TaxesOutputs",
                BuildTax(
                    new(
                        line.Source.VatRate,
                        line.GrossAmount,
                        line.TaxAmount))));

    private static XElement BuildTax(
        CalculatedTax tax) =>
        Element(
            "Tax",
            Element("TaxTypeCode", "01"),
            Element("TaxRate", FormatDecimal(tax.Rate)),
            AmountElement("TaxableBase", tax.TaxableBase),
            AmountElement("TaxAmount", tax.TaxAmount));

    private static object? BuildPaymentDetails(
        FacturaeInvoiceDraft draft,
        decimal invoiceTotal)
    {
        if (draft.InstallmentDueDate is not { } dueDate)
        {
            return null;
        }

        return Element(
            "PaymentDetails",
            Element(
                "Installment",
                Element(
                    "InstallmentDueDate",
                    dueDate.ToString(
                        "yyyy-MM-dd",
                        CultureInfo.InvariantCulture)),
                Element(
                    "InstallmentAmount",
                    FormatAmount(invoiceTotal)),
                Element("PaymentMeans", "04"),
                Element(
                    "AccountToBeCredited",
                    Element("IBAN", NormalizeIban(draft.Iban)))));
    }

    private static XElement AmountElement(
        string name,
        decimal amount) =>
        Element(
            name,
            Element("TotalAmount", FormatAmount(amount)));

    private static XElement Element(
        string name,
        params object?[] content) =>
        new(name, content);

    private static string Serialize(
        XDocument document)
    {
        using var writer = new Utf8StringWriter();
        using var xmlWriter = XmlWriter.Create(
            writer,
            new XmlWriterSettings
            {
                Encoding = writer.Encoding,
                Indent = true,
                IndentChars = "  ",
                NewLineChars = "\n",
                NewLineHandling = NewLineHandling.Replace,
                OmitXmlDeclaration = false,
            });
        document.Save(xmlWriter);
        xmlWriter.Flush();
        return writer.ToString();
    }

    private static CalculatedLine CalculateLine(
        FacturaeLineDraft line)
    {
        var gross = RoundMoney(line.Quantity * line.UnitPriceWithoutTax);
        var tax = RoundMoney(gross * line.VatRate / 100m);
        return new(line, gross, tax);
    }

    private static void ValidateParty(
        List<string> errors,
        FacturaePartyDraft party,
        string label)
    {
        if (party is null)
        {
            errors.Add($"Faltan los datos del {label}.");
            return;
        }

        var personType = party.PersonTypeCode.Trim().ToUpperInvariant();
        if (personType is not ("J" or "F"))
        {
            errors.Add(
                $"El tipo de persona del {label} debe ser J o F.");
        }
        ValidateRequiredText(
            errors,
            party.TaxIdentificationNumber,
            $"El NIF del {label}",
            30,
            3);
        ValidateRequiredText(
            errors,
            party.Name,
            personType == "F"
                ? $"El nombre del {label}"
                : $"La razón social del {label}",
            personType == "F" ? 40 : 80);
        if (personType == "F")
        {
            ValidateRequiredText(
                errors,
                party.FirstSurname,
                $"El primer apellido del {label}",
                40);
            ValidateOptionalText(
                errors,
                party.SecondSurname,
                $"El segundo apellido del {label}",
                40);
        }
        ValidateAddress(errors, party.Address, label);
        ValidateOptionalText(
            errors,
            party.ElectronicMail,
            $"El correo del {label}",
            60);
        if (!string.IsNullOrWhiteSpace(party.ElectronicMail) &&
            !EmailRegex().IsMatch(party.ElectronicMail.Trim()))
        {
            errors.Add($"El correo del {label} no tiene un formato válido.");
        }
    }

    private static void ValidateAddress(
        List<string> errors,
        FacturaeAddressDraft address,
        string label)
    {
        if (address is null)
        {
            errors.Add($"Falta la dirección del {label}.");
            return;
        }

        ValidateRequiredText(
            errors,
            address.Address,
            $"La dirección del {label}",
            80);
        if (!PostCodeRegex().IsMatch(address.PostCode.Trim()))
        {
            errors.Add(
                $"El código postal del {label} debe tener cinco cifras.");
        }
        ValidateRequiredText(
            errors,
            address.Town,
            $"La población del {label}",
            50);
        ValidateRequiredText(
            errors,
            address.Province,
            $"La provincia del {label}",
            20);
    }

    private static void ValidateDir3(
        List<string> errors,
        string code,
        string label)
    {
        if (!Dir3Regex().IsMatch(code.Trim()))
        {
            errors.Add(
                $"El código DIR3 de la {label} debe tener nueve caracteres alfanuméricos.");
        }
    }

    private static void ValidateLine(
        List<string> errors,
        FacturaeLineDraft line,
        int lineNumber)
    {
        if (line is null)
        {
            errors.Add($"Faltan los datos de la línea {lineNumber}.");
            return;
        }
        ValidateRequiredText(
            errors,
            line.Description,
            $"La descripción de la línea {lineNumber}",
            2500);
        if (line.Quantity <= 0)
        {
            errors.Add(
                $"La cantidad de la línea {lineNumber} debe ser mayor que cero.");
        }
        if (line.UnitPriceWithoutTax < 0)
        {
            errors.Add(
                $"El precio de la línea {lineNumber} no puede ser negativo.");
        }
        if (line.VatRate is < 0 or > 100)
        {
            errors.Add(
                $"El IVA de la línea {lineNumber} debe estar entre 0 y 100.");
        }
        if (DecimalPlaces(line.Quantity) > 8 ||
            DecimalPlaces(line.UnitPriceWithoutTax) > 8 ||
            DecimalPlaces(line.VatRate) > 8)
        {
            errors.Add(
                $"Los valores de la línea {lineNumber} admiten como máximo ocho decimales.");
        }
    }

    private static void ValidateRequiredText(
        List<string> errors,
        string? value,
        string label,
        int maximumLength,
        int minimumLength = 1)
    {
        var length = value?.Trim().Length ?? 0;
        if (length < minimumLength)
        {
            errors.Add($"{label} es obligatorio.");
        }
        else if (length > maximumLength)
        {
            errors.Add(
                $"{label} no puede superar {maximumLength} caracteres.");
        }
    }

    private static void ValidateOptionalText(
        List<string> errors,
        string? value,
        string label,
        int maximumLength)
    {
        if ((value?.Trim().Length ?? 0) > maximumLength)
        {
            errors.Add(
                $"{label} no puede superar {maximumLength} caracteres.");
        }
    }

    private static void AddOptional(
        List<object> target,
        string elementName,
        string value)
    {
        if (!string.IsNullOrWhiteSpace(value))
        {
            target.Add(Element(elementName, value.Trim()));
        }
    }

    private static string NormalizeTaxId(
        string value) =>
        value.Trim().Replace(" ", "", StringComparison.Ordinal);

    private static string NormalizeIban(
        string value) =>
        value
            .Replace(" ", "", StringComparison.Ordinal)
            .Replace("-", "", StringComparison.Ordinal)
            .ToUpperInvariant();

    private static bool IsValidIban(
        string value)
    {
        if (!IbanRegex().IsMatch(value))
        {
            return false;
        }

        var rearranged = value[4..] + value[..4];
        var remainder = 0;
        foreach (var character in rearranged)
        {
            if (char.IsDigit(character))
            {
                remainder = (remainder * 10 + character - '0') % 97;
                continue;
            }

            var numeric = character - 'A' + 10;
            remainder = (remainder * 10 + numeric / 10) % 97;
            remainder = (remainder * 10 + numeric % 10) % 97;
        }
        return remainder == 1;
    }

    private static decimal RoundMoney(
        decimal value) =>
        decimal.Round(value, 2, MidpointRounding.AwayFromZero);

    private static string FormatAmount(
        decimal value) =>
        RoundMoney(value).ToString("0.00", CultureInfo.InvariantCulture);

    private static string FormatDecimal(
        decimal value) =>
        value.ToString("0.########", CultureInfo.InvariantCulture);

    private static int DecimalPlaces(
        decimal value)
    {
        var bits = decimal.GetBits(value);
        return (bits[3] >> 16) & 0x7F;
    }

    [GeneratedRegex(@"^[A-Za-z0-9]{9}$", RegexOptions.CultureInvariant)]
    private static partial Regex Dir3Regex();

    [GeneratedRegex(@"^[0-9]{5}$", RegexOptions.CultureInvariant)]
    private static partial Regex PostCodeRegex();

    [GeneratedRegex(
        @"^[A-Z]{2}[0-9]{2}[A-Z0-9]{11,30}$",
        RegexOptions.CultureInvariant)]
    private static partial Regex IbanRegex();

    [GeneratedRegex(
        @"^[^@\s]+@[^@\s]+\.[^@\s]+$",
        RegexOptions.CultureInvariant)]
    private static partial Regex EmailRegex();

    [GeneratedRegex(
        @"[^\p{L}\p{Nd}._-]+",
        RegexOptions.CultureInvariant)]
    private static partial Regex SuggestedNameUnsafeCharacters();

    private sealed record CalculatedLine(
        FacturaeLineDraft Source,
        decimal GrossAmount,
        decimal TaxAmount);

    private sealed record CalculatedTax(
        decimal Rate,
        decimal TaxableBase,
        decimal TaxAmount);

    private sealed record AdministrativeCentre(
        string Code,
        string Role,
        string Name);

    private sealed class Utf8StringWriter : StringWriter
    {
        public override System.Text.Encoding Encoding { get; } =
            new System.Text.UTF8Encoding(
                encoderShouldEmitUTF8Identifier: false);
    }
}
