// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;
using GrxFirma.WinUI.Core.Facturae;
using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.ViewModels;

public sealed class FacturaePageViewModel
    : WorkspacePageViewModel
{
    private readonly IFacePortalLauncherService _launcher;
    private readonly IFilePickerService _filePicker;
    private readonly DesktopOperationSession _session;
    private CancellationTokenSource? _pageLifetime;
    private string _statusTitle = "Asistente preparado";
    private string _statusMessage =
        "Puede crear un XML Facturae 3.2.2 o seguir la guía de envío.";
    private string _invoiceNumber = "";
    private string _invoiceSeriesCode = "";
    private DateTimeOffset _issueDate = DateTimeOffset.Now.Date;
    private bool _sellerIsIndividual;
    private string _sellerTaxId = "";
    private string _sellerName = "";
    private string _sellerFirstSurname = "";
    private string _sellerSecondSurname = "";
    private string _sellerAddress = "";
    private string _sellerPostCode = "";
    private string _sellerTown = "";
    private string _sellerProvince = "";
    private string _sellerEmail = "";
    private string _buyerTaxId = "";
    private string _buyerName = "";
    private string _buyerAddress = "";
    private string _buyerPostCode = "";
    private string _buyerTown = "";
    private string _buyerProvince = "";
    private string _accountingOfficeDir3 = "";
    private string _managingBodyDir3 = "";
    private string _processingUnitDir3 = "";
    private string _lineDescription = "";
    private double _lineQuantity = 1;
    private double _lineUnitPrice;
    private double _lineVatRate = 21;
    private string _invoiceDescription = "";
    private string _fileReference = "";
    private string _receiverContractReference = "";
    private bool _includePayment;
    private DateTimeOffset _installmentDueDate =
        DateTimeOffset.Now.Date.AddDays(30);
    private string _iban = "";
    private string _calculatedBase = "0,00 €";
    private string _calculatedTax = "0,00 €";
    private string _calculatedTotal = "0,00 €";
    private bool _isActive;
    private bool _isBusy;
    private bool _canLaunch;
    private bool _canCreate;
    private bool _hasStatus;
    private int _operationInProgress;
    private InfoBarSeverity _statusSeverity =
        InfoBarSeverity.Informational;

    public FacturaePageViewModel(
        IFacePortalLauncherService launcher,
        IFilePickerService filePicker,
        DesktopOperationSession session)
        : base(
            "Facturae y FACe",
            "Cree una factura Facturae 3.2.2 y preséntela en FACe mediante un flujo separado y opcional.",
            "El asistente de Facturae y FACe no está disponible.")
    {
        ArgumentNullException.ThrowIfNull(launcher);
        ArgumentNullException.ThrowIfNull(filePicker);
        _launcher = launcher;
        _filePicker = filePicker;
        _session = session ?? throw new ArgumentNullException(nameof(session));
        RefreshCalculatedTotals();
    }

    public string StatusTitle
    {
        get => _statusTitle;
        private set => SetProperty(ref _statusTitle, Localizer.Text(value));
    }

    public string StatusMessage
    {
        get => _statusMessage;
        private set => SetProperty(ref _statusMessage, Localizer.Text(value));
    }

    public bool IsBusy
    {
        get => _isBusy;
        private set => SetProperty(ref _isBusy, value);
    }

    public bool CanLaunch
    {
        get => _canLaunch;
        private set => SetProperty(ref _canLaunch, value);
    }

    public bool CanCreate
    {
        get => _canCreate;
        private set => SetProperty(ref _canCreate, value);
    }

    public bool HasStatus
    {
        get => _hasStatus;
        private set => SetProperty(ref _hasStatus, value);
    }

    public InfoBarSeverity StatusSeverity
    {
        get => _statusSeverity;
        private set => SetProperty(ref _statusSeverity, value);
    }

    public string InvoiceNumber
    {
        get => _invoiceNumber;
        set => SetProperty(ref _invoiceNumber, value);
    }

    public string InvoiceSeriesCode
    {
        get => _invoiceSeriesCode;
        set => SetProperty(ref _invoiceSeriesCode, value);
    }

    public DateTimeOffset IssueDate
    {
        get => _issueDate;
        set => SetProperty(ref _issueDate, value);
    }

    public bool SellerIsIndividual
    {
        get => _sellerIsIndividual;
        set => SetProperty(ref _sellerIsIndividual, value);
    }

    public string SellerTaxId
    {
        get => _sellerTaxId;
        set => SetProperty(ref _sellerTaxId, value);
    }

    public string SellerName
    {
        get => _sellerName;
        set => SetProperty(ref _sellerName, value);
    }

    public string SellerFirstSurname
    {
        get => _sellerFirstSurname;
        set => SetProperty(ref _sellerFirstSurname, value);
    }

    public string SellerSecondSurname
    {
        get => _sellerSecondSurname;
        set => SetProperty(ref _sellerSecondSurname, value);
    }

    public string SellerAddress
    {
        get => _sellerAddress;
        set => SetProperty(ref _sellerAddress, value);
    }

    public string SellerPostCode
    {
        get => _sellerPostCode;
        set => SetProperty(ref _sellerPostCode, value);
    }

    public string SellerTown
    {
        get => _sellerTown;
        set => SetProperty(ref _sellerTown, value);
    }

    public string SellerProvince
    {
        get => _sellerProvince;
        set => SetProperty(ref _sellerProvince, value);
    }

    public string SellerEmail
    {
        get => _sellerEmail;
        set => SetProperty(ref _sellerEmail, value);
    }

    public string BuyerTaxId
    {
        get => _buyerTaxId;
        set => SetProperty(ref _buyerTaxId, value);
    }

    public string BuyerName
    {
        get => _buyerName;
        set => SetProperty(ref _buyerName, value);
    }

    public string BuyerAddress
    {
        get => _buyerAddress;
        set => SetProperty(ref _buyerAddress, value);
    }

    public string BuyerPostCode
    {
        get => _buyerPostCode;
        set => SetProperty(ref _buyerPostCode, value);
    }

    public string BuyerTown
    {
        get => _buyerTown;
        set => SetProperty(ref _buyerTown, value);
    }

    public string BuyerProvince
    {
        get => _buyerProvince;
        set => SetProperty(ref _buyerProvince, value);
    }

    public string AccountingOfficeDir3
    {
        get => _accountingOfficeDir3;
        set => SetProperty(ref _accountingOfficeDir3, value);
    }

    public string ManagingBodyDir3
    {
        get => _managingBodyDir3;
        set => SetProperty(ref _managingBodyDir3, value);
    }

    public string ProcessingUnitDir3
    {
        get => _processingUnitDir3;
        set => SetProperty(ref _processingUnitDir3, value);
    }

    public string LineDescription
    {
        get => _lineDescription;
        set => SetProperty(ref _lineDescription, value);
    }

    public double LineQuantity
    {
        get => _lineQuantity;
        set
        {
            if (SetProperty(ref _lineQuantity, value))
            {
                RefreshCalculatedTotals();
            }
        }
    }

    public double LineUnitPrice
    {
        get => _lineUnitPrice;
        set
        {
            if (SetProperty(ref _lineUnitPrice, value))
            {
                RefreshCalculatedTotals();
            }
        }
    }

    public double LineVatRate
    {
        get => _lineVatRate;
        set
        {
            if (SetProperty(ref _lineVatRate, value))
            {
                RefreshCalculatedTotals();
            }
        }
    }

    public string InvoiceDescription
    {
        get => _invoiceDescription;
        set => SetProperty(ref _invoiceDescription, value);
    }

    public string FileReference
    {
        get => _fileReference;
        set => SetProperty(ref _fileReference, value);
    }

    public string ReceiverContractReference
    {
        get => _receiverContractReference;
        set => SetProperty(ref _receiverContractReference, value);
    }

    public bool IncludePayment
    {
        get => _includePayment;
        set => SetProperty(ref _includePayment, value);
    }

    public DateTimeOffset InstallmentDueDate
    {
        get => _installmentDueDate;
        set => SetProperty(ref _installmentDueDate, value);
    }

    public string Iban
    {
        get => _iban;
        set => SetProperty(ref _iban, value);
    }

    public string CalculatedBase
    {
        get => _calculatedBase;
        private set => SetProperty(ref _calculatedBase, value);
    }

    public string CalculatedTax
    {
        get => _calculatedTax;
        private set => SetProperty(ref _calculatedTax, value);
    }

    public string CalculatedTotal
    {
        get => _calculatedTotal;
        private set => SetProperty(ref _calculatedTotal, value);
    }

    public void Activate()
    {
        if (_isActive)
        {
            return;
        }

        _pageLifetime = new CancellationTokenSource();
        _isActive = true;
        _session.AvailabilityChanged += OnSessionAvailabilityChanged;
        SetOperationAvailability(
            _session.Supports(DesktopOperationActions.FacturaeCreate),
            "El XML se crea localmente. Los enlaces abren únicamente páginas HTTPS fijas del Portal FACe Proveedores.");
        SetActionsEnabled(
            Volatile.Read(ref _operationInProgress) == 0);
    }

    public void Deactivate()
    {
        if (!_isActive)
        {
            return;
        }

        _isActive = false;
        _session.AvailabilityChanged -= OnSessionAvailabilityChanged;
        SetActionsEnabled(false);
        _pageLifetime?.Cancel();
        _pageLifetime?.Dispose();
        _pageLifetime = null;
    }

    public async Task CreateInvoiceAsync()
    {
        var lifetime = TryBeginOperation();
        if (lifetime is null)
        {
            return;
        }

        StatusTitle = "Comprobando la factura";
        StatusMessage =
            "Se validan los campos y se calculan los totales antes de guardar.";
        StatusSeverity = InfoBarSeverity.Informational;
        try
        {
            var draft = BuildDraft();
            if (!_session.TryGetOperations(DesktopOperationActions.FacturaeCreate, out var operations))
            {
                StatusTitle = "No se pudo crear la factura";
                StatusMessage = PendingMessage;
                StatusSeverity = InfoBarSeverity.Warning;
                return;
            }
            var result = await operations.CreateFacturaeAsync(draft, lifetime.Token);
            if (!IsCurrentLifetime(lifetime)) return;
            if (!result.IsSuccess || result.Data is null || string.IsNullOrEmpty(result.Data.Xml))
            {
                StatusTitle = "Revise los datos";
                StatusMessage = result.SafeUserMessage;
                StatusSeverity = InfoBarSeverity.Warning;
                return;
            }
            var saved = await _filePicker.PickAndSaveTextFileAsync(
                SaveFilePickerProfile.FacturaeXml,
                result.Data.Xml,
                FacturaeInvoiceGenerator.SuggestedFileName(draft),
                lifetime.Token);
            if (!IsCurrentLifetime(lifetime))
            {
                return;
            }

            if (!saved)
            {
                StatusTitle = "Guardado cancelado";
                StatusMessage =
                    "No se creó ningún fichero. Los datos siguen en el formulario.";
                StatusSeverity = InfoBarSeverity.Informational;
                return;
            }

            StatusTitle = "XML Facturae 3.2.2 creado";
            StatusMessage = Localizer.Fill(
                "Total {total} €. Abra «Firmar», seleccione el XML y el formato FacturaE; después valídelo en FACe.",
                ("total", result.Data.Total.ToString(CultureInfo.CurrentCulture)));
            StatusSeverity = InfoBarSeverity.Success;
        }
        catch (FacturaeValidationException exception)
        {
            if (IsCurrentLifetime(lifetime))
            {
                StatusTitle = "Revise los datos";
                StatusMessage = string.Join(
                    " ",
                    exception.Errors.Take(5));
                StatusSeverity = InfoBarSeverity.Warning;
            }
        }
        catch (OperationCanceledException)
        {
            if (IsCurrentLifetime(lifetime))
            {
                StatusTitle = "Creación cancelada";
                StatusMessage =
                    "No se creó ni modificó ningún fichero.";
                StatusSeverity = InfoBarSeverity.Informational;
            }
        }
        catch
        {
            if (IsCurrentLifetime(lifetime))
            {
                StatusTitle = "No se pudo crear la factura";
                StatusMessage =
                    "Windows no pudo guardar el XML Facturae. Revise los datos y vuelva a intentarlo.";
                StatusSeverity = InfoBarSeverity.Error;
            }
        }
        finally
        {
            EndOperation(lifetime);
        }
    }

    public Task OpenValidatorAsync() =>
        ExecutePortalAsync(
            "Abriendo el validador oficial",
            _launcher.OpenValidatorAsync);

    public Task OpenOrganisationDirectoryAsync() =>
        ExecutePortalAsync(
            "Abriendo el buscador DIR3",
            _launcher.OpenOrganisationDirectoryAsync);

    public Task OpenSubmissionAsync() =>
        ExecutePortalAsync(
            "Abriendo la remisión oficial",
            _launcher.OpenSubmissionAsync);

    public Task OpenInvoiceStatusAsync() =>
        ExecutePortalAsync(
            "Abriendo la consulta oficial",
            _launcher.OpenInvoiceStatusAsync);

    public Task OpenReceiptVerificationAsync() =>
        ExecutePortalAsync(
            "Abriendo la verificación del justificante",
            _launcher.OpenReceiptVerificationAsync);

    private async Task ExecutePortalAsync(
        string progressTitle,
        Func<CancellationToken, Task<HelpLaunchResult>> operation)
    {
        var lifetime = TryBeginOperation();
        if (lifetime is null)
        {
            return;
        }

        StatusTitle = progressTitle;
        StatusMessage = "Espere un momento.";
        StatusSeverity = InfoBarSeverity.Informational;
        try
        {
            var result = await operation(lifetime.Token);
            if (!IsCurrentLifetime(lifetime))
            {
                return;
            }

            StatusTitle = result.Succeeded
                ? "Portal oficial abierto"
                : "No se pudo abrir FACe";
            StatusMessage = result.Message;
            StatusSeverity = result.Succeeded
                ? InfoBarSeverity.Success
                : InfoBarSeverity.Warning;
        }
        catch (OperationCanceledException)
        {
            if (IsCurrentLifetime(lifetime))
            {
                StatusTitle = "Apertura cancelada";
                StatusMessage =
                    "No se ha enviado ni modificado ninguna factura.";
                StatusSeverity = InfoBarSeverity.Informational;
            }
        }
        catch
        {
            if (IsCurrentLifetime(lifetime))
            {
                StatusTitle = "No se pudo abrir FACe";
                StatusMessage =
                    "Windows no pudo abrir el recurso oficial solicitado.";
                StatusSeverity = InfoBarSeverity.Error;
            }
        }
        finally
        {
            EndOperation(lifetime);
        }
    }

    private CancellationTokenSource? TryBeginOperation()
    {
        var lifetime = _pageLifetime;
        if (!_isActive ||
            lifetime is null ||
            Interlocked.CompareExchange(
                ref _operationInProgress,
                1,
                0) != 0)
        {
            return null;
        }

        IsBusy = true;
        HasStatus = true;
        SetActionsEnabled(false);
        return lifetime;
    }

    private void EndOperation(
        CancellationTokenSource lifetime)
    {
        Interlocked.Exchange(ref _operationInProgress, 0);
        if (IsCurrentLifetime(lifetime))
        {
            IsBusy = false;
            SetActionsEnabled(true);
        }
    }

    private FacturaeInvoiceDraft BuildDraft()
    {
        var quantity = ToDecimal(LineQuantity);
        var unitPrice = ToDecimal(LineUnitPrice);
        var vatRate = ToDecimal(LineVatRate);
        return new(
            InvoiceNumber,
            InvoiceSeriesCode,
            DateOnly.FromDateTime(IssueDate.LocalDateTime),
            new(
                SellerIsIndividual ? "F" : "J",
                SellerTaxId,
                SellerName,
                SellerFirstSurname,
                SellerSecondSurname,
                new(
                    SellerAddress,
                    SellerPostCode,
                    SellerTown,
                    SellerProvince),
                SellerEmail),
            new(
                "J",
                BuyerTaxId,
                BuyerName,
                "",
                "",
                new(
                    BuyerAddress,
                    BuyerPostCode,
                    BuyerTown,
                    BuyerProvince)),
            AccountingOfficeDir3,
            ManagingBodyDir3,
            ProcessingUnitDir3,
            [
                new(
                    LineDescription,
                    quantity,
                    unitPrice,
                    vatRate),
            ],
            IncludePayment
                ? DateOnly.FromDateTime(
                    InstallmentDueDate.LocalDateTime)
                : null,
            IncludePayment ? Iban : "",
            InvoiceDescription,
            FileReference,
            ReceiverContractReference);
    }

    private void RefreshCalculatedTotals()
    {
        if (!TryToDecimal(LineQuantity, out var quantity) ||
            !TryToDecimal(LineUnitPrice, out var price) ||
            !TryToDecimal(LineVatRate, out var vat) ||
            quantity < 0 ||
            price < 0 ||
            vat is < 0 or > 100)
        {
            CalculatedBase = "—";
            CalculatedTax = "—";
            CalculatedTotal = "—";
            return;
        }

        try
        {
            var taxableBase = decimal.Round(
                quantity * price,
                2,
                MidpointRounding.AwayFromZero);
            var tax = decimal.Round(
                taxableBase * vat / 100m,
                2,
                MidpointRounding.AwayFromZero);
            CalculatedBase = FormatCurrency(taxableBase);
            CalculatedTax = FormatCurrency(tax);
            CalculatedTotal = FormatCurrency(taxableBase + tax);
        }
        catch (OverflowException)
        {
            CalculatedBase = "—";
            CalculatedTax = "—";
            CalculatedTotal = "—";
        }
    }

    private void OnSessionAvailabilityChanged(object? sender, EventArgs args)
    {
        if (!_isActive) return;
        SetOperationAvailability(
            _session.Supports(DesktopOperationActions.FacturaeCreate),
            "El XML se crea localmente. Los enlaces abren únicamente páginas HTTPS fijas del Portal FACe Proveedores.");
        SetActionsEnabled(Volatile.Read(ref _operationInProgress) == 0);
    }

    private void SetActionsEnabled(
        bool enabled)
    {
        CanLaunch = enabled;
        CanCreate = enabled && _session.Supports(DesktopOperationActions.FacturaeCreate);
    }

    private bool IsCurrentLifetime(
        CancellationTokenSource lifetime) =>
        _isActive &&
        ReferenceEquals(_pageLifetime, lifetime);

    private static decimal ToDecimal(
        double value)
    {
        if (!TryToDecimal(value, out var result))
        {
            throw new FacturaeValidationException(
            [
                "Cantidad, precio e IVA deben ser números válidos.",
            ]);
        }
        return result;
    }

    private static bool TryToDecimal(
        double value,
        out decimal result)
    {
        if (double.IsFinite(value) &&
            value <= (double)decimal.MaxValue &&
            value >= (double)decimal.MinValue)
        {
            try
            {
                result = (decimal)value;
                return true;
            }
            catch (OverflowException)
            {
                // La representación double de los límites de decimal no es
                // exacta; el valor se tratará como entrada fuera de rango.
            }
        }
        result = 0;
        return false;
    }

    private static string FormatCurrency(
        decimal value) =>
        value.ToString(
            "C2",
            CultureInfo.GetCultureInfo("es-ES"));
}
