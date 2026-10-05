// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;
using System.Text;
using Microsoft.UI.Xaml;
using Windows.Storage;
using Windows.Storage.Pickers;
using WinRT.Interop;

namespace GrxFirma.WinUI.Services;

public sealed class WindowsFilePickerService : IFilePickerService
{
    private const int MaximumSuggestedBaseNameLength = 100;
    private const int MaximumTextFileBytes = 128 * 1024;

    private static readonly HashSet<string> ReservedWindowsNames =
        new(StringComparer.OrdinalIgnoreCase)
        {
            "CON",
            "PRN",
            "AUX",
            "NUL",
            "COM1",
            "COM2",
            "COM3",
            "COM4",
            "COM5",
            "COM6",
            "COM7",
            "COM8",
            "COM9",
            "LPT1",
            "LPT2",
            "LPT3",
            "LPT4",
            "LPT5",
            "LPT6",
            "LPT7",
            "LPT8",
            "LPT9",
        };

    private readonly Func<nint> _windowHandleProvider;
    private readonly SemaphoreSlim _pickerGate = new(1, 1);

    public WindowsFilePickerService(Window ownerWindow)
        : this(
            ownerWindow is null
                ? throw new ArgumentNullException(nameof(ownerWindow))
                : () => WindowNative.GetWindowHandle(ownerWindow))
    {
    }

    public WindowsFilePickerService(Func<nint> windowHandleProvider)
    {
        ArgumentNullException.ThrowIfNull(windowHandleProvider);
        _windowHandleProvider = windowHandleProvider;
    }

    public async Task<string?> PickOpenFileAsync(
        OpenFilePickerProfile profile,
        CancellationToken cancellationToken = default)
    {
        await _pickerGate.WaitAsync(cancellationToken);
        try
        {
            cancellationToken.ThrowIfCancellationRequested();
            var picker = CreateOpenPicker(profile);
            var operation = picker.PickSingleFileAsync();
            using var cancellationRegistration = cancellationToken.Register(
                static state => ((Windows.Foundation.IAsyncInfo)state!).Cancel(),
                operation);
            var file = await operation;
            cancellationToken.ThrowIfCancellationRequested();
            return LocalPathOrNull(file);
        }
        finally
        {
            _pickerGate.Release();
        }
    }

    public async Task<IReadOnlyList<string>> PickOpenFilesAsync(
        OpenFilePickerProfile profile,
        CancellationToken cancellationToken = default)
    {
        await _pickerGate.WaitAsync(cancellationToken);
        try
        {
            cancellationToken.ThrowIfCancellationRequested();
            var picker = CreateOpenPicker(profile);
            var operation = picker.PickMultipleFilesAsync();
            using var cancellationRegistration = cancellationToken.Register(
                static state => ((Windows.Foundation.IAsyncInfo)state!).Cancel(),
                operation);
            var files = await operation;
            cancellationToken.ThrowIfCancellationRequested();
            if (files is null || files.Count == 0)
            {
                return Array.Empty<string>();
            }

            return files
                .Select(LocalPathOrNull)
                .Where(static path => path is not null)
                .Cast<string>()
                .ToArray();
        }
        finally
        {
            _pickerGate.Release();
        }
    }

    public async Task<string?> PickSaveFileAsync(
        SaveFilePickerProfile profile,
        string? suggestedFileName = null,
        CancellationToken cancellationToken = default)
    {
        await _pickerGate.WaitAsync(cancellationToken);
        try
        {
            cancellationToken.ThrowIfCancellationRequested();
            // El selector de escritorio devuelve una ruta, sin crear ni
            // truncar el archivo antes de que el motor complete la operación.
            // No borrar "reservas" por ruta: podrían ser archivos del usuario.
            var picker = CreatePathSavePicker(profile, suggestedFileName);
            var operation = picker.PickSaveFileAsync();
            using var cancellationRegistration = cancellationToken.Register(
                static state => ((Windows.Foundation.IAsyncInfo)state!).Cancel(),
                operation);
            var file = await operation;
            cancellationToken.ThrowIfCancellationRequested();
            return FullyQualifiedPathOrNull(file?.Path);
        }
        finally
        {
            _pickerGate.Release();
        }
    }

    public async Task<bool> PickAndSaveTextFileAsync(
        SaveFilePickerProfile profile,
        string contents,
        string? suggestedFileName = null,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(contents);
        if (Encoding.UTF8.GetByteCount(contents) > MaximumTextFileBytes)
        {
            throw new ArgumentException(
                Localizer.Text("winui.selector.el_informe_de_texto_supera_el_limite"),
                nameof(contents));
        }

        await _pickerGate.WaitAsync(cancellationToken);
        try
        {
            cancellationToken.ThrowIfCancellationRequested();
            var picker = CreateSavePicker(profile, suggestedFileName);
            var operation = picker.PickSaveFileAsync();
            using var cancellationRegistration = cancellationToken.Register(
                static state => ((Windows.Foundation.IAsyncInfo)state!).Cancel(),
                operation);
            var file = await operation;
            cancellationToken.ThrowIfCancellationRequested();
            if (file is null)
            {
                return false;
            }

            // Tras la confirmación del diálogo el informe es pequeño y se
            // completa como una única escritura sobre el StorageFile elegido.
            // No se devuelve ni se vuelve a abrir su ruta.
            await FileIO.WriteTextAsync(
                file,
                contents,
                Windows.Storage.Streams.UnicodeEncoding.Utf8);
            return true;
        }
        finally
        {
            _pickerGate.Release();
        }
    }

    public async Task<string?> PickFolderAsync(
        CancellationToken cancellationToken = default)
    {
        await _pickerGate.WaitAsync(cancellationToken);
        try
        {
            cancellationToken.ThrowIfCancellationRequested();
            var picker = new FolderPicker
            {
                SuggestedStartLocation = PickerLocationId.DocumentsLibrary,
                CommitButtonText = Localizer.Text("winui.selector.seleccionar_carpeta"),
            };
            // WinRT exige al menos un filtro también para FolderPicker. Este
            // comodín no permite seleccionar ficheros: el diálogo solo expone
            // carpetas.
            picker.FileTypeFilter.Add("*");
            InitializeWithOwner(picker);

            var operation = picker.PickSingleFolderAsync();
            using var cancellationRegistration = cancellationToken.Register(
                static state => ((Windows.Foundation.IAsyncInfo)state!).Cancel(),
                operation);
            var folder = await operation;
            cancellationToken.ThrowIfCancellationRequested();
            return LocalPathOrNull(folder);
        }
        finally
        {
            _pickerGate.Release();
        }
    }

    private FileOpenPicker CreateOpenPicker(OpenFilePickerProfile profile)
    {
        var picker = new FileOpenPicker
        {
            SuggestedStartLocation = PickerLocationId.DocumentsLibrary,
            ViewMode = PickerViewMode.List,
            CommitButtonText = Localizer.Text("winui.selector.seleccionar"),
        };
        foreach (var extension in OpenExtensions(profile))
        {
            picker.FileTypeFilter.Add(extension);
        }
        InitializeWithOwner(picker);
        return picker;
    }

    private Microsoft.Windows.Storage.Pickers.FileSavePicker CreatePathSavePicker(
        SaveFilePickerProfile profile,
        string? suggestedFileName)
    {
        var windowHandle = _windowHandleProvider();
        if (windowHandle == 0)
        {
            throw new InvalidOperationException(
                Localizer.Text("winui.comun.la_ventana_propietaria_todavia_no_esta"));
        }
        var policy = SavePolicy(profile);
        var picker = new Microsoft.Windows.Storage.Pickers.FileSavePicker(
            Microsoft.UI.Win32Interop.GetWindowIdFromWindow(windowHandle))
        {
            SuggestedStartLocation =
                Microsoft.Windows.Storage.Pickers.PickerLocationId.DocumentsLibrary,
            CommitButtonText = Localizer.Text("winui.selector.guardar"),
            ShowOverwritePrompt = true,
            DefaultFileExtension = policy.Extensions[0],
            SuggestedFileName = SafeSuggestedFileName(suggestedFileName, policy),
        };
        picker.FileTypeChoices.Add(Localizer.Text(policy.DisplayName), policy.Extensions.ToArray());
        return picker;
    }

    private FileSavePicker CreateSavePicker(
        SaveFilePickerProfile profile,
        string? suggestedFileName)
    {
        var policy = SavePolicy(profile);
        var picker = new FileSavePicker
        {
            SuggestedStartLocation = PickerLocationId.DocumentsLibrary,
            CommitButtonText = Localizer.Text("winui.selector.guardar"),
            DefaultFileExtension = policy.Extensions[0],
            SuggestedFileName = SafeSuggestedFileName(
                suggestedFileName,
                policy),
        };
        picker.FileTypeChoices.Add(
            Localizer.Text(policy.DisplayName),
            policy.Extensions.ToArray());
        InitializeWithOwner(picker);
        return picker;
    }

    private void InitializeWithOwner(object picker)
    {
        var windowHandle = _windowHandleProvider();
        if (windowHandle == 0)
        {
            throw new InvalidOperationException(
                Localizer.Text("winui.comun.la_ventana_propietaria_todavia_no_esta"));
        }
        InitializeWithWindow.Initialize(picker, windowHandle);
    }

    private static IReadOnlyList<string> OpenExtensions(
        OpenFilePickerProfile profile) =>
        profile switch
        {
            OpenFilePickerProfile.SignableDocument =>
            [
                ".pdf",
                ".xml",
                ".json",
                ".txt",
                ".csv",
                ".rtf",
                ".doc",
                ".docx",
                ".xls",
                ".xlsx",
                ".ppt",
                ".pptx",
                ".odt",
                ".ods",
                ".odp",
                ".jpg",
                ".jpeg",
                ".png",
                ".tif",
                ".tiff",
                ".zip",
            ],
            OpenFilePickerProfile.SignedOrOriginalDocument =>
            [
                ".pdf",
                ".xml",
                ".json",
                ".txt",
                ".csv",
                ".rtf",
                ".doc",
                ".docx",
                ".xls",
                ".xlsx",
                ".ppt",
                ".pptx",
                ".odt",
                ".ods",
                ".odp",
                ".jpg",
                ".jpeg",
                ".png",
                ".tif",
                ".tiff",
                ".zip",
                ".p7s",
                ".p7m",
                ".csig",
                ".xsig",
                ".asics",
                ".asice",
                ".sig",
            ],
            OpenFilePickerProfile.Certificate =>
            [
                ".p12",
                ".pfx",
                ".pem",
                ".cer",
                ".crt",
            ],
            OpenFilePickerProfile.PublicRecipientCertificate =>
            [
                ".cer",
                ".crt",
                ".pem",
                ".der",
            ],
            OpenFilePickerProfile.HashManifest =>
            [
                ".hexhash",
                ".hashb64",
                ".hash",
                ".hashfiles",
                ".txthashfiles",
                ".csv",
                ".hashreport",
                ".xml",
                ".txt",
            ],
            OpenFilePickerProfile.ProtectedContainer =>
            [
                ".afp",
                ".enveloped",
                ".authenveloped.p7m",
                ".encrypted.p7m",
                ".p7m",
                ".cms",
                ".asice",
            ],
            OpenFilePickerProfile.SealImage =>
            [
                ".png",
                ".jpg",
                ".jpeg",
            ],
            OpenFilePickerProfile.VeriFactuQrSource =>
            [
                ".png",
                ".jpg",
                ".jpeg",
                ".pdf",
            ],
            _ => throw new ArgumentOutOfRangeException(
                nameof(profile),
                profile,
                Localizer.Text("winui.selector.perfil_de_apertura_no_soportado")),
        };

    private static SavePickerPolicy SavePolicy(
        SaveFilePickerProfile profile) =>
        profile switch
        {
            SaveFilePickerProfile.PublicCertificate =>
                new("winui.selector.certificado_publico_x_509", "winui.selector.nombre_certificado_publico", [".cer", ".pem"]),
            SaveFilePickerProfile.SignedPdf =>
                new("winui.selector.documento_pdf_firmado", "winui.firmar.documento_firmado", [".pdf"]),
            SaveFilePickerProfile.CadesSignature =>
                new("winui.selector.firma_cades", "winui.selector.nombre_firma", [".p7s"]),
            SaveFilePickerProfile.XadesSignature =>
                new("winui.selector.firma_xades", "winui.selector.nombre_firma", [".xsig"]),
            SaveFilePickerProfile.XmlDsigSignature =>
                new("winui.selector.firma_xmldsig", "winui.selector.nombre_firma", [".dsig"]),
            SaveFilePickerProfile.FacturaeXml =>
                new(
                    "Facturae 3.2.2",
                    "facturae",
                    [".xml", ".xsig"]),
            SaveFilePickerProfile.AsicContainer =>
                new("winui.selector.contenedor_asic", "winui.selector.nombre_contenedor_firmado", [".asics", ".asice"]),
            SaveFilePickerProfile.HashManifest =>
                new("winui.selector.informe_de_huella", "winui.selector.nombre_informe_huella", [".hashreport", ".hash"]),
            SaveFilePickerProfile.ProtectedContainer =>
                new(
                    "winui.selector.documento_protegido",
                    "winui.proteger.documento_protegido",
                    [
                        ".afp",
                        ".enveloped",
                        ".authenveloped.p7m",
                        ".encrypted.p7m",
                        ".p7m",
                    ]),
            SaveFilePickerProfile.ProtectedJson =>
                new(
                    "winui.selector.sobre_protegido_grxfirma",
                    "winui.proteger.documento_protegido",
                    [".afp"]),
            SaveFilePickerProfile.CmsEnveloped =>
                new(
                    "CMS EnvelopedData",
                    "winui.proteger.documento_protegido",
                    [".enveloped"]),
            SaveFilePickerProfile.CmsEncrypted =>
                new(
                    "CMS EncryptedData",
                    "winui.proteger.documento_protegido",
                    [".encrypted.p7m"]),
            SaveFilePickerProfile.CmsAuthEnveloped =>
                new(
                    "CMS AuthEnvelopedData",
                    "winui.proteger.documento_protegido",
                    [".authenveloped.p7m"]),
            SaveFilePickerProfile.CmsSignedEnveloped =>
                new(
                    "CMS SignedAndEnvelopedData",
                    "winui.selector.nombre_documento_protegido_firmado",
                    [".signedenveloped.p7m"]),
            SaveFilePickerProfile.DiagnosticReport =>
                new("winui.selector.informe_de_diagnostico", "winui.selector.nombre_diagnostico", [".json", ".txt"]),
            SaveFilePickerProfile.VerificationReport =>
                new(SealUiCatalog.Text(Localizer.Language,
                    "winui.parity.verify.export"),
                    SealUiCatalog.Text(Localizer.Language,
                    "winui.parity.verify.filename"), [".json"]),
            SaveFilePickerProfile.InvoiceReport =>
                new(SealUiCatalog.Text(Localizer.Language,
                    "paridad.lote3.invoice.report"), SealUiCatalog.Text(Localizer.Language,
                    "paridad.lote3.invoice.filename"), [".txt"]),
            SaveFilePickerProfile.EniXml =>
                new(SealUiCatalog.Text(Localizer.Language,
                    "paridad.lote3.eni.xml_filter"), SealUiCatalog.Text(Localizer.Language,
                    "paridad.lote3.eni.filename"), [".xml"]),
            SaveFilePickerProfile.SupportIncidentText =>
                new(SealUiCatalog.Text(Localizer.Language,
                    "winui.parity.support.export"),
                    SealUiCatalog.Text(Localizer.Language,
                    "winui.parity.support.filename"), [".txt"]),
            _ => throw new ArgumentOutOfRangeException(
                nameof(profile),
                profile,
                "winui.selector.perfil_de_guardado_no_soportado"),
        };

    private static string SafeSuggestedFileName(
        string? suggestedFileName,
        SavePickerPolicy policy)
    {
        var candidate = string.IsNullOrWhiteSpace(suggestedFileName)
            ? Localizer.Text(policy.DefaultBaseName)
            : Path.GetFileNameWithoutExtension(
                Path.GetFileName(suggestedFileName.Trim()));
        var invalidCharacters = Path.GetInvalidFileNameChars();
        var safeBaseName = new StringBuilder(
            Math.Min(candidate.Length, MaximumSuggestedBaseNameLength));
        foreach (var rune in candidate.EnumerateRunes())
        {
            var category = Rune.GetUnicodeCategory(rune);
            var runeText = rune.ToString();
            if (category is UnicodeCategory.Control or
                UnicodeCategory.Format or
                UnicodeCategory.Surrogate ||
                runeText.Length == 1 &&
                invalidCharacters.Contains(runeText[0]))
            {
                continue;
            }
            if (safeBaseName.Length + runeText.Length >
                MaximumSuggestedBaseNameLength)
            {
                break;
            }
            safeBaseName.Append(runeText);
        }

        var normalized = safeBaseName
            .ToString()
            .Trim()
            .TrimEnd(' ', '.');
        if (string.IsNullOrWhiteSpace(normalized))
        {
            normalized = Localizer.Text(policy.DefaultBaseName);
        }
        var deviceBaseName = normalized.Split('.', 2)[0];
        if (ReservedWindowsNames.Contains(deviceBaseName))
        {
            normalized = "_" + normalized;
        }
        return normalized + policy.Extensions[0];
    }

    private static string? LocalPathOrNull(IStorageItem? item) =>
        FullyQualifiedPathOrNull(item?.Path);

    private static string? FullyQualifiedPathOrNull(string? path) =>
        string.IsNullOrWhiteSpace(path) ||
            !Path.IsPathFullyQualified(path)
            ? null
            : path;

    private sealed record SavePickerPolicy(
        string DisplayName,
        string DefaultBaseName,
        IReadOnlyList<string> Extensions);
}
