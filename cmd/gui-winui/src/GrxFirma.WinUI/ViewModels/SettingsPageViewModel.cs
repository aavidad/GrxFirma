// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Security.Cryptography;
using System.Text;
using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Operations;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.ViewModels;

public sealed record SettingsOption(
    string Label,
    string Value);

public sealed class SettingsPageViewModel
    : WorkspacePageViewModel
{
    private readonly DesktopOperationSession _session;
    private CancellationTokenSource? _pageLifetime;
    private CancellationTokenSource? _operationCancellation;
    private DesktopSettingsDocument? _loadedSnapshot;
    private SettingsOption _selectedLanguage;
    private SettingsOption _selectedTheme;
    private SettingsOption _selectedDefaultFormat;
    private SettingsOption _selectedProxyType;
    private string _proxyHost = string.Empty;
    private string _proxyPortText = "8080";
    private string _proxyExcludedUrlsText = string.Empty;
    private string _proxyRealm = string.Empty;
    private string _proxyUsername = string.Empty;
    private string _proxySecurityStatusTitle =
        "Estado del almacén pendiente";
    private string _proxySecurityStatusMessage =
        "Actualice el estado para comprobar la protección de credenciales.";
    private string _proxyRuntimeModeMessage = string.Empty;
    private string _validationMessage =
        "Cargue las preferencias del motor local antes de editarlas.";
    private string _statusTitle = "Sin cambios";
    private string _statusMessage =
        "Las preferencias todavía no se han cargado.";
    private bool _confirmBeforeSigning = true;
    private bool _stayResident = true;
    private bool _strictSignatureCompatibility = true;
    private bool _visiblePdfSeal;
    private bool _facturaeToolsEnabled;
    private bool _checkForUpdates = true;
    private bool _proxyEnabled;
    private bool _isActive;
    private bool _isApplying;
    private bool _hasLoaded;
    private bool _isDirty;
    private bool _isBusy;
    private bool _canEdit;
    private bool _canChooseProxyType;
    private bool _canEditManualProxy;
    private bool _proxyCredentialsConfigured;
    private bool _proxySecretStoreAvailable;
    private bool _canRefreshProxySecretStatus;
    private bool _canStoreProxyCredentials;
    private bool _canDeleteProxyCredentials;
    private bool _canReload;
    private bool _canSave;
    private bool _canCancel;
    private bool _hasStatus;
    private int _operationInProgress;
    private InfoBarSeverity _statusSeverity =
        InfoBarSeverity.Informational;

    public SettingsPageViewModel(DesktopOperationSession session)
        : base(
            "Configuración",
            "Preferencias compartidas de firma, red y experiencia de usuario.",
            "La configuración no está disponible porque el motor local no ha publicado get_settings y save_settings.")
    {
        ArgumentNullException.ThrowIfNull(session);
        _session = session;
        _selectedLanguage = Languages[0];
        _selectedTheme = Themes[0];
        _selectedDefaultFormat = DefaultFormats[0];
        _selectedProxyType = ProxyTypes[0];
    }

    public event Action<OperationDiagnostic>? DiagnosticRequested;
    public event Action<int?>? ThemePreferenceApplied;

    public IReadOnlyList<SettingsOption> Languages { get; } =
    [
        new("Español", "es"),
        new("Català", "ca"),
        new("Valencià", "va"),
        new("Euskara", "eu"),
        new("Galego", "gl"),
        new("English", "en"),
        new("Deutsch", "de"),
        new("Français", "fr"),
        new("Português", "pt"),
        new("Italiano", "it"),
        new("中文", "zh"),
    ];

    public IReadOnlyList<SettingsOption> Themes { get; } =
    [
        new("Cristal oscuro", "0"),
        new("Minimalista claro", "1"),
        new("Futurista (oscuro en Windows)", "2"),
    ];

    public IReadOnlyList<SettingsOption> DefaultFormats { get; } =
    [
        new("Automático", ""),
        new("PAdES", "pades"),
        new("CAdES", "cades"),
        new("XAdES", "xades"),
        new("XMLDSig", "xmldsig"),
        new("ODF", "odf"),
        new("OOXML", "ooxml"),
        new("FacturaE", "facturae"),
        new("ASiC-XAdES", "asic-xades"),
    ];

    public IReadOnlyList<SettingsOption> ProxyTypes { get; } =
    [
        new("Sin proxy", "none"),
        new("Manual", "manual"),
    ];

    public SettingsOption SelectedLanguage
    {
        get => _selectedLanguage;
        set
        {
            if (value is not null &&
                Languages.Contains(value) &&
                SetProperty(ref _selectedLanguage, value))
            {
                MarkDirty();
            }
        }
    }

    public SettingsOption SelectedTheme
    {
        get => _selectedTheme;
        set
        {
            if (value is not null &&
                Themes.Contains(value) &&
                SetProperty(ref _selectedTheme, value))
            {
                MarkDirty();
            }
        }
    }

    public SettingsOption SelectedDefaultFormat
    {
        get => _selectedDefaultFormat;
        set
        {
            if (value is not null &&
                DefaultFormats.Contains(value) &&
                SetProperty(ref _selectedDefaultFormat, value))
            {
                MarkDirty();
            }
        }
    }

    public SettingsOption SelectedProxyType
    {
        get => _selectedProxyType;
        set
        {
            if (value is not null &&
                ProxyTypes.Contains(value) &&
                SetProperty(ref _selectedProxyType, value))
            {
                MarkDirty();
            }
        }
    }

    public bool ConfirmBeforeSigning
    {
        get => _confirmBeforeSigning;
        set
        {
            if (SetProperty(ref _confirmBeforeSigning, value))
            {
                MarkDirty();
            }
        }
    }

    public bool StayResident
    {
        get => _stayResident;
        set
        {
            if (SetProperty(ref _stayResident, value))
            {
                MarkDirty();
            }
        }
    }

    public bool StrictSignatureCompatibility
    {
        get => _strictSignatureCompatibility;
        set
        {
            if (SetProperty(
                ref _strictSignatureCompatibility,
                value))
            {
                MarkDirty();
            }
        }
    }

    public bool VisiblePdfSeal
    {
        get => _visiblePdfSeal;
        set
        {
            if (SetProperty(ref _visiblePdfSeal, value))
            {
                MarkDirty();
            }
        }
    }

    public bool FacturaeToolsEnabled
    {
        get => _facturaeToolsEnabled;
        set
        {
            if (SetProperty(ref _facturaeToolsEnabled, value))
            {
                MarkDirty();
            }
        }
    }

    public bool CheckForUpdates
    {
        get => _checkForUpdates;
        set
        {
            if (SetProperty(ref _checkForUpdates, value))
            {
                MarkDirty();
            }
        }
    }

    public bool ProxyEnabled
    {
        get => _proxyEnabled;
        set
        {
            if (SetProperty(ref _proxyEnabled, value))
            {
                MarkDirty();
            }
        }
    }

    public string ProxyHost
    {
        get => _proxyHost;
        set
        {
            var safeValue = value ?? string.Empty;
            if (SetProperty(ref _proxyHost, safeValue))
            {
                MarkDirty();
            }
        }
    }

    public string ProxyPortText
    {
        get => _proxyPortText;
        set
        {
            var safeValue = value ?? string.Empty;
            if (SetProperty(ref _proxyPortText, safeValue))
            {
                MarkDirty();
            }
        }
    }

    public string ProxyExcludedUrlsText
    {
        get => _proxyExcludedUrlsText;
        set
        {
            var safeValue = value ?? string.Empty;
            if (SetProperty(ref _proxyExcludedUrlsText, safeValue))
            {
                MarkDirty();
            }
        }
    }

    public string ProxyRealm
    {
        get => _proxyRealm;
        set
        {
            var safeValue = value ?? string.Empty;
            if (SetProperty(ref _proxyRealm, safeValue))
            {
                RefreshCommandState();
            }
        }
    }

    public string ProxyUsername
    {
        get => _proxyUsername;
        set
        {
            var safeValue = value ?? string.Empty;
            if (SetProperty(ref _proxyUsername, safeValue))
            {
                RefreshCommandState();
            }
        }
    }

    public bool ProxyCredentialsConfigured
    {
        get => _proxyCredentialsConfigured;
        private set
        {
            if (SetProperty(ref _proxyCredentialsConfigured, value))
            {
                RefreshCommandState();
            }
        }
    }

    public bool ProxySecretStoreAvailable
    {
        get => _proxySecretStoreAvailable;
        private set
        {
            if (SetProperty(ref _proxySecretStoreAvailable, value))
            {
                RefreshCommandState();
            }
        }
    }

    public string ProxySecurityStatusTitle
    {
        get => _proxySecurityStatusTitle;
        private set => SetProperty(
            ref _proxySecurityStatusTitle,
            value);
    }

    public string ProxySecurityStatusMessage
    {
        get => _proxySecurityStatusMessage;
        private set => SetProperty(
            ref _proxySecurityStatusMessage,
            value);
    }

    public string ProxyRuntimeModeMessage
    {
        get => _proxyRuntimeModeMessage;
        private set => SetProperty(
            ref _proxyRuntimeModeMessage,
            value);
    }

    public bool CanRefreshProxySecretStatus
    {
        get => _canRefreshProxySecretStatus;
        private set => SetProperty(
            ref _canRefreshProxySecretStatus,
            value);
    }

    public bool CanStoreProxyCredentials
    {
        get => _canStoreProxyCredentials;
        private set => SetProperty(
            ref _canStoreProxyCredentials,
            value);
    }

    public bool CanDeleteProxyCredentials
    {
        get => _canDeleteProxyCredentials;
        private set => SetProperty(
            ref _canDeleteProxyCredentials,
            value);
    }

    public string ValidationMessage
    {
        get => _validationMessage;
        private set => SetProperty(ref _validationMessage, value);
    }

    public string StatusTitle
    {
        get => _statusTitle;
        private set => SetProperty(ref _statusTitle, value);
    }

    public string StatusMessage
    {
        get => _statusMessage;
        private set => SetProperty(ref _statusMessage, value);
    }

    public bool IsDirty
    {
        get => _isDirty;
        private set => SetProperty(ref _isDirty, value);
    }

    public bool IsBusy
    {
        get => _isBusy;
        private set => SetProperty(ref _isBusy, value);
    }

    public bool CanEdit
    {
        get => _canEdit;
        private set => SetProperty(ref _canEdit, value);
    }

    public bool CanChooseProxyType
    {
        get => _canChooseProxyType;
        private set => SetProperty(ref _canChooseProxyType, value);
    }

    public bool CanEditManualProxy
    {
        get => _canEditManualProxy;
        private set => SetProperty(ref _canEditManualProxy, value);
    }

    public bool CanReload
    {
        get => _canReload;
        private set => SetProperty(ref _canReload, value);
    }

    public bool CanSave
    {
        get => _canSave;
        private set => SetProperty(ref _canSave, value);
    }

    public bool CanCancel
    {
        get => _canCancel;
        private set => SetProperty(ref _canCancel, value);
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

    public void Activate()
    {
        if (_isActive)
        {
            return;
        }

        _isActive = true;
        _pageLifetime = new CancellationTokenSource();
        _session.AvailabilityChanged += OnAvailabilityChanged;
        RefreshAvailability();
    }

    public void Deactivate()
    {
        if (!_isActive)
        {
            return;
        }

        _isActive = false;
        _session.AvailabilityChanged -= OnAvailabilityChanged;
        _operationCancellation?.Cancel();
        _pageLifetime?.Cancel();
        _pageLifetime?.Dispose();
        _pageLifetime = null;
        RefreshAvailability();
    }

    public async Task LoadAsync()
    {
        if (!CanReload ||
            _pageLifetime is null ||
            !_session.TryGetOperations(
                DesktopOperationActions.GetSettings,
                out var operations) ||
            !TryBeginOperation(out var operationCancellation))
        {
            RefreshAvailability();
            return;
        }

        ShowStatus(
            "Cargando preferencias…",
            "Se está leyendo la configuración persistida por el motor local.",
            InfoBarSeverity.Informational);
        try
        {
            var result = await operations.GetSettingsAsync(
                operationCancellation.Token);
            if (!result.IsSuccess || result.Outcome != "success")
            {
                ShowStatus(
                    "No se pudo cargar",
                    result.SafeUserMessage,
                    InfoBarSeverity.Error);
                RequestDiagnostic(
                    OperationDiagnosticMapper.FromResult(result));
                return;
            }
            if (result.Data is null)
            {
                ShowIncoherentResult(
                    "El motor local no devolvió un documento de preferencias.");
                return;
            }

            ApplyLoadedSettings(result.Data);
            ThemePreferenceApplied?.Invoke(result.Data.ThemeIndex);
            ShowStatus(
                "Preferencias cargadas",
                "Puede modificar los valores y guardarlos cuando termine.",
                InfoBarSeverity.Success);
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            ShowStatus(
                "Carga cancelada",
                "La lectura de preferencias se detuvo antes de completarse.",
                InfoBarSeverity.Warning);
        }
        catch (Exception exception)
        {
            ShowStatus(
                "No se pudo cargar",
                "La lectura terminó sin un documento de preferencias.",
                InfoBarSeverity.Error);
            RequestDiagnostic(OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation.Token));
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    public async Task SaveAsync()
    {
        UpdateValidation();
        if (!CanSave ||
            _loadedSnapshot is null ||
            _pageLifetime is null ||
            !_session.TryGetOperations(
                DesktopOperationActions.SaveSettings,
                out var operations) ||
            !TryBuildSaveDocument(out var document) ||
            !TryBeginOperation(out var operationCancellation))
        {
            RefreshCommandState();
            return;
        }

        ShowStatus(
            "Guardando preferencias…",
            "El motor local está validando y persistiendo los cambios.",
            InfoBarSeverity.Informational);
        try
        {
            var result = await operations.SaveSettingsAsync(
                document,
                operationCancellation.Token);
            if (!result.IsSuccess || result.Outcome != "success")
            {
                ShowStatus(
                    "No se pudo guardar",
                    result.SafeUserMessage,
                    InfoBarSeverity.Error);
                RequestDiagnostic(
                    OperationDiagnosticMapper.FromResult(result));
                return;
            }
            if (string.IsNullOrWhiteSpace(result.Data))
            {
                ShowIncoherentResult(
                    "El motor local no confirmó el guardado de las preferencias.");
                return;
            }

            _loadedSnapshot = document.CreateSafeSaveSnapshot();
            IsDirty = false;
            ThemePreferenceApplied?.Invoke(document.ThemeIndex);
            UpdateValidation();
            ShowStatus(
                "Preferencias guardadas",
                "Los cambios se han persistido correctamente.",
                InfoBarSeverity.Success);
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            ShowStatus(
                "Guardado cancelado",
                "No se recibió confirmación de que los cambios se hayan guardado.",
                InfoBarSeverity.Warning);
        }
        catch (Exception exception)
        {
            ShowStatus(
                "No se pudo guardar",
                "El guardado terminó sin una confirmación válida.",
                InfoBarSeverity.Error);
            RequestDiagnostic(OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation.Token));
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    public async Task RefreshProxySecretStatusAsync()
    {
        if (!CanRefreshProxySecretStatus ||
            _pageLifetime is null ||
            !_session.TryGetOperations(
                DesktopOperationActions.ProxySecretStoreStatus,
                out var operations) ||
            !TryBeginOperation(out var operationCancellation))
        {
            RefreshCommandState();
            return;
        }

        ProxySecurityStatusTitle = "Comprobando almacén seguro…";
        ProxySecurityStatusMessage =
            "Windows está comprobando la protección local de credenciales.";
        try
        {
            var result =
                await operations.GetProxySecretStoreStatusAsync(
                    operationCancellation.Token);
            if (!ApplyProxySecretStoreStatus(result))
            {
                RequestDiagnostic(
                    OperationDiagnosticMapper.FromResult(result));
            }
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            ProxySecurityStatusTitle = "Comprobación cancelada";
            ProxySecurityStatusMessage =
                "No se modificaron las credenciales del proxy.";
        }
        catch (Exception exception)
        {
            SetProxyStoreUnavailable(
                "No se pudo comprobar el almacén protegido.");
            RequestDiagnostic(OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation.Token));
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    /// <summary>
    /// Consume password. Tanto este ViewModel como DesktopOperationsClient
    /// sobrescriben el mismo buffer para cubrir todas las salidas.
    /// </summary>
    public async Task StoreProxyCredentialsAsync(byte[] password)
    {
        ArgumentNullException.ThrowIfNull(password);
        try
        {
            if (!CanStoreProxyCredentials ||
                _pageLifetime is null ||
                !_session.TryGetOperations(
                    DesktopOperationActions.ProxySecretStore,
                    out var operations) ||
                !TryBeginOperation(out var operationCancellation))
            {
                RefreshCommandState();
                return;
            }

            try
            {
                if (!TryNormalizeProxyCredentialText(
                        ProxyRealm,
                        out var realm) ||
                    !TryNormalizeProxyCredentialText(
                        ProxyUsername,
                        out var username))
                {
                    ShowStatus(
                        "Credenciales no válidas",
                        "El dominio y el usuario deben tener entre 1 y 256 bytes UTF-8 y no contener controles.",
                        InfoBarSeverity.Warning);
                    return;
                }

                ShowStatus(
                    "Protegiendo credenciales…",
                    "La contraseña se enviará una sola vez al almacén protegido de Windows.",
                    InfoBarSeverity.Informational);
                var result = await operations.StoreProxySecretAsync(
                    new ProxySecretStoreParameters
                    {
                        Realm = realm,
                        Username = username,
                        Password = password,
                    },
                    operationCancellation.Token);
                if (!result.IsSuccess ||
                    result.Outcome != "success")
                {
                    ShowStatus(
                        "No se guardaron las credenciales",
                        result.SafeUserMessage,
                        InfoBarSeverity.Error);
                    RequestDiagnostic(
                        OperationDiagnosticMapper.FromResult(result));
                    return;
                }
                if (result.Data is null ||
                    !result.Data.Configured)
                {
                    ShowIncoherentResult(
                        "El motor local no confirmó que la credencial quedara protegida.");
                    return;
                }

                ProxyCredentialsConfigured = true;
                ProxyRealm = SafeIpcText.Clean(
                    result.Data.Realm,
                    256,
                    realm);
                ProxyUsername = string.Empty;
                var refreshed =
                    await RefreshProxyStateAfterMutationAsync(
                        operations,
                        operationCancellation.Token);
                ShowStatus(
                    refreshed
                        ? result.Data.Rotated
                            ? "Credenciales cambiadas"
                            : "Credenciales protegidas"
                        : "Credenciales protegidas; estado pendiente",
                    refreshed
                        ? "El almacén seguro confirmó el estado persistido."
                        : "El guardado se confirmó, pero no se pudo volver a consultar su estado.",
                    refreshed
                        ? InfoBarSeverity.Success
                        : InfoBarSeverity.Warning);
            }
            catch (OperationCanceledException)
                when (operationCancellation.IsCancellationRequested)
            {
                ShowStatus(
                    "Operación cancelada",
                    "No se recibió confirmación de que las credenciales se guardaran.",
                    InfoBarSeverity.Warning);
            }
            catch (Exception exception)
            {
                ShowStatus(
                    "No se guardaron las credenciales",
                    "La protección local de la contraseña no pudo completarse.",
                    InfoBarSeverity.Error);
                RequestDiagnostic(OperationDiagnosticMapper.FromException(
                    exception,
                    operationCancellation.Token));
            }
            finally
            {
                EndOperation(operationCancellation);
            }
        }
        finally
        {
            CryptographicOperations.ZeroMemory(password);
        }
    }

    public async Task DeleteProxyCredentialsAsync()
    {
        if (!CanDeleteProxyCredentials ||
            _pageLifetime is null ||
            !_session.TryGetOperations(
                DesktopOperationActions.ProxySecretDelete,
                out var operations) ||
            !TryBeginOperation(out var operationCancellation))
        {
            RefreshCommandState();
            return;
        }

        try
        {
            ShowStatus(
                "Quitando credenciales…",
                "El almacén protegido está eliminando la credencial asociada.",
                InfoBarSeverity.Informational);
            var result = await operations.DeleteProxySecretAsync(
                operationCancellation.Token);
            if (!result.IsSuccess ||
                result.Outcome != "success")
            {
                ShowStatus(
                    "No se quitaron las credenciales",
                    result.SafeUserMessage,
                    InfoBarSeverity.Error);
                RequestDiagnostic(
                    OperationDiagnosticMapper.FromResult(result));
                return;
            }
            if (result.Data is null ||
                result.Data.Configured)
            {
                ShowIncoherentResult(
                    "El motor local no confirmó la eliminación de la credencial.");
                return;
            }

            ProxyCredentialsConfigured = false;
            ProxyRealm = string.Empty;
            ProxyUsername = string.Empty;
            var refreshed =
                await RefreshProxyStateAfterMutationAsync(
                    operations,
                    operationCancellation.Token);
            ShowStatus(
                refreshed
                    ? "Credenciales eliminadas"
                    : "Credenciales eliminadas; estado pendiente",
                refreshed
                    ? "El proxy manual ya no tiene autenticación protegida asociada."
                    : "La eliminación se confirmó, pero no se pudo volver a consultar el estado.",
                refreshed
                    ? InfoBarSeverity.Success
                    : InfoBarSeverity.Warning);
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            ShowStatus(
                "Operación cancelada",
                "No se recibió confirmación de que la credencial se eliminara.",
                InfoBarSeverity.Warning);
        }
        catch (Exception exception)
        {
            ShowStatus(
                "No se quitaron las credenciales",
                "El almacén protegido no confirmó la eliminación.",
                InfoBarSeverity.Error);
            RequestDiagnostic(OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation.Token));
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    public void ReportProxyCredentialCaptureFailure(
        Exception exception,
        CancellationToken cancellationToken)
    {
        ArgumentNullException.ThrowIfNull(exception);
        ShowStatus(
            "No se pudo capturar la contraseña",
            "Windows no pudo preparar la contraseña como memoria borrable.",
            InfoBarSeverity.Error);
        RequestDiagnostic(OperationDiagnosticMapper.FromException(
            exception,
            cancellationToken));
    }

    public void CancelCurrentOperation() =>
        _operationCancellation?.Cancel();

    private void ApplyLoadedSettings(DesktopSettingsDocument settings)
    {
        var safeSnapshot = settings.CreateSafeSaveSnapshot();
        _isApplying = true;
        try
        {
            ApplyProxyCredentialMetadata(settings);
            SelectedLanguage = FindOption(
                Languages,
                DesktopSettingsDocument.IsSupportedLanguage(
                    safeSnapshot.Language)
                    ? safeSnapshot.Language!
                    : "es");
            SelectedTheme = FindOption(
                Themes,
                DesktopSettingsDocument.IsSupportedTheme(
                    safeSnapshot.ThemeIndex)
                    ? safeSnapshot.ThemeIndex!.Value.ToString()
                    : "0");
            ConfirmBeforeSigning =
                safeSnapshot.ConfirmBeforeSigning ?? true;
            StayResident = safeSnapshot.CloseBehavior != "exit";
            SelectedDefaultFormat = FindOption(
                DefaultFormats,
                DesktopSettingsDocument.IsSupportedSignatureFormat(
                    safeSnapshot.DefaultSignatureFormat)
                    ? safeSnapshot.DefaultSignatureFormat!
                    : string.Empty);
            StrictSignatureCompatibility =
                safeSnapshot.StrictSignatureCompatibility ?? true;
            VisiblePdfSeal = safeSnapshot.VisiblePdfSeal ?? false;
            FacturaeToolsEnabled =
                safeSnapshot.FacturaeToolsEnabled ?? false;
            CheckForUpdates =
                safeSnapshot.CheckForUpdates ?? true;
            ProxyEnabled = safeSnapshot.ProxyEnabled ?? false;
            SelectedProxyType = FindOption(
                ProxyTypes,
                DesktopSettingsDocument.IsSupportedProxyType(
                    safeSnapshot.ProxyType)
                    ? safeSnapshot.ProxyType!
                    : "none");
            ProxyHost = SafeIpcText.Clean(
                safeSnapshot.ProxyHost,
                512,
                string.Empty);
            ProxyPortText =
                safeSnapshot.ProxyPort is >= 1 and <= 65535
                    ? safeSnapshot.ProxyPort.Value.ToString()
                    : "8080";
            ProxyExcludedUrlsText = string.Join(
                Environment.NewLine,
                safeSnapshot.ProxyExcludedUrls ?? []);
        }
        finally
        {
            _isApplying = false;
        }

        _loadedSnapshot = safeSnapshot;
        _hasLoaded = true;
        IsDirty = false;
        UpdateValidation();
    }

    private bool TryBuildSaveDocument(
        out DesktopSettingsDocument document)
    {
        document = default!;
        if (_loadedSnapshot is null ||
            !int.TryParse(SelectedTheme.Value, out var themeIndex) ||
            !int.TryParse(ProxyPortText, out var proxyPort) ||
            !TryParseProxyExcludedUrls(
                ProxyExcludedUrlsText,
                out var proxyExcludedUrls))
        {
            return false;
        }

        document = (_loadedSnapshot with
        {
            Language = SelectedLanguage.Value,
            ThemeIndex = themeIndex,
            ConfirmBeforeSigning = ConfirmBeforeSigning,
            CloseBehavior = StayResident ? "resident" : "exit",
            DefaultSignatureFormat = SelectedDefaultFormat.Value,
            StrictSignatureCompatibility =
                StrictSignatureCompatibility,
            VisiblePdfSeal = VisiblePdfSeal,
            FacturaeToolsEnabled = FacturaeToolsEnabled,
            CheckForUpdates = CheckForUpdates,
            ProxyEnabled = ProxyEnabled,
            ProxyType = SelectedProxyType.Value,
            ProxyHost = ProxyHost,
            ProxyPort = proxyPort,
            ProxyExcludedUrls = proxyExcludedUrls,
        }).CreateSafeSaveSnapshot();
        return true;
    }

    private void MarkDirty()
    {
        if (_isApplying || !_hasLoaded)
        {
            return;
        }

        IsDirty = true;
        UpdateValidation();
    }

    private void UpdateValidation()
    {
        ValidationMessage = Validate();
        RefreshCommandState();
    }

    private string Validate()
    {
        if (!_hasLoaded)
        {
            return "Cargue las preferencias del motor local antes de editarlas.";
        }
        if (!DesktopSettingsDocument.IsSupportedLanguage(
            SelectedLanguage.Value))
        {
            return "Seleccione un idioma admitido.";
        }
        if (!int.TryParse(SelectedTheme.Value, out var themeIndex) ||
            !DesktopSettingsDocument.IsSupportedTheme(themeIndex))
        {
            return "Seleccione un tema visual admitido.";
        }
        if (!DesktopSettingsDocument.IsSupportedSignatureFormat(
            SelectedDefaultFormat.Value))
        {
            return "Seleccione un formato de firma admitido.";
        }
        if (VisiblePdfSeal &&
            SelectedDefaultFormat.Value is not ("" or "pades"))
        {
            return "El sello visible solo es compatible con el formato automático o PAdES.";
        }
        if (!DesktopSettingsDocument.IsSupportedProxyType(
            SelectedProxyType.Value))
        {
            return "Seleccione un tipo de proxy admitido.";
        }
        if (!int.TryParse(ProxyPortText, out var port) ||
            port is < 1 or > 65535)
        {
            return "El puerto del proxy debe estar entre 1 y 65535.";
        }
        if (ProxyEnabled && SelectedProxyType.Value == "none")
        {
            return "Seleccione proxy manual o desactive el uso de proxy.";
        }
        if (ProxyEnabled && SelectedProxyType.Value == "manual")
        {
            var host = ProxyHost.Trim();
            if (host.Length is < 1 or > 512 ||
                host.Any(char.IsControl) ||
                host.Any(char.IsWhiteSpace) ||
                host.Contains("://", StringComparison.Ordinal) ||
                host.IndexOfAny(['/', '\\', '@']) >= 0)
            {
                return "Indique solo el host o la IP del proxy, sin esquema, ruta ni credenciales.";
            }
        }
        if (!TryParseProxyExcludedUrls(
                ProxyExcludedUrlsText,
                out _))
        {
            return "Las exclusiones del proxy admiten hasta 512 entradas de 2048 caracteres, sin controles.";
        }
        if (ProxyCredentialsConfigured &&
            IsProxyEndpointDirty())
        {
            return "Quite primero las credenciales protegidas antes de cambiar el host o el puerto del proxy.";
        }

        return IsDirty
            ? "Cambios pendientes de guardar."
            : "Las preferencias cargadas no tienen cambios pendientes.";
    }

    private void OnAvailabilityChanged(object? sender, EventArgs args)
    {
        var wasConnected = IsOperationConnected;
        RefreshAvailability();
        if (!wasConnected &&
            IsOperationConnected &&
            !_hasLoaded &&
            !IsBusy)
        {
            _ = LoadAndRefreshProxyStateAsync();
        }
    }

    private async Task LoadAndRefreshProxyStateAsync()
    {
        await LoadAsync();
        if (CanRefreshProxySecretStatus)
        {
            await RefreshProxySecretStatusAsync();
        }
    }

    private void RefreshAvailability()
    {
        var available =
            _isActive &&
            _session.Supports(DesktopOperationActions.GetSettings) &&
            _session.Supports(DesktopOperationActions.SaveSettings);
        SetOperationAvailability(
            available,
            "Motor local listo para cargar y guardar preferencias.");
        if (!available)
        {
            _operationCancellation?.Cancel();
        }
        RefreshCommandState();
    }

    private bool TryBeginOperation(
        out CancellationTokenSource operationCancellation)
    {
        operationCancellation = default!;
        if (_pageLifetime is null ||
            Interlocked.CompareExchange(
                ref _operationInProgress,
                1,
                0) != 0)
        {
            return false;
        }

        operationCancellation =
            CancellationTokenSource.CreateLinkedTokenSource(
                _pageLifetime.Token);
        _operationCancellation = operationCancellation;
        IsBusy = true;
        RefreshCommandState();
        return true;
    }

    private void EndOperation(
        CancellationTokenSource operationCancellation)
    {
        if (ReferenceEquals(
            _operationCancellation,
            operationCancellation))
        {
            _operationCancellation = null;
        }
        operationCancellation.Dispose();
        Interlocked.Exchange(ref _operationInProgress, 0);
        IsBusy = false;
        RefreshCommandState();
    }

    private void RefreshCommandState()
    {
        CanReload =
            _isActive &&
            IsOperationConnected &&
            !IsBusy;
        CanEdit = CanReload && _hasLoaded;
        CanChooseProxyType = CanEdit && ProxyEnabled;
        CanEditManualProxy =
            CanChooseProxyType &&
            SelectedProxyType.Value == "manual";
        CanRefreshProxySecretStatus =
            _isActive &&
            IsOperationConnected &&
            !IsBusy &&
            _session.Supports(
                DesktopOperationActions.ProxySecretStoreStatus);
        CanStoreProxyCredentials =
            CanEditManualProxy &&
            ProxySecretStoreAvailable &&
            !IsBusy &&
            _session.Supports(
                DesktopOperationActions.ProxySecretStore) &&
            !IsProxyEndpointDirty() &&
            TryNormalizeProxyCredentialText(
                ProxyRealm,
                out _) &&
            TryNormalizeProxyCredentialText(
                ProxyUsername,
                out _);
        CanDeleteProxyCredentials =
            CanEdit &&
            ProxySecretStoreAvailable &&
            ProxyCredentialsConfigured &&
            !IsBusy &&
            _session.Supports(
                DesktopOperationActions.ProxySecretDelete);
        CanSave =
            CanEdit &&
            IsDirty &&
            string.Equals(
                ValidationMessage,
                "Cambios pendientes de guardar.",
                StringComparison.Ordinal);
        CanCancel = _isActive && IsBusy;
    }

    private void ShowStatus(
        string title,
        string message,
        InfoBarSeverity severity)
    {
        HasStatus = true;
        StatusTitle = title;
        StatusMessage = message;
        StatusSeverity = severity;
    }

    private void ShowIncoherentResult(string message)
    {
        ShowStatus(
            "Respuesta no utilizable",
            message,
            InfoBarSeverity.Error);
        RequestDiagnostic(OperationDiagnosticMapper.FromException(
            new InvalidOperationException()));
    }

    private void RequestDiagnostic(OperationDiagnostic diagnostic) =>
        DiagnosticRequested?.Invoke(diagnostic);

    private async Task<bool> RefreshProxyStateAfterMutationAsync(
        DesktopOperationsClient operations,
        CancellationToken cancellationToken)
    {
        var settings = await operations.GetSettingsAsync(
            cancellationToken);
        if (!settings.IsSuccess ||
            settings.Outcome != "success" ||
            settings.Data is null)
        {
            RequestDiagnostic(
                OperationDiagnosticMapper.FromResult(settings));
            return false;
        }
        ApplyProxyCredentialMetadata(settings.Data);

        var status =
            await operations.GetProxySecretStoreStatusAsync(
                cancellationToken);
        if (!ApplyProxySecretStoreStatus(status))
        {
            RequestDiagnostic(
                OperationDiagnosticMapper.FromResult(status));
            return false;
        }
        return true;
    }

    private void ApplyProxyCredentialMetadata(
        DesktopSettingsDocument settings)
    {
        var configured =
            !string.IsNullOrWhiteSpace(settings.ProxySecretId);
        ProxyCredentialsConfigured = configured;
        ProxyRealm = configured
            ? SafeIpcText.Clean(
                settings.ProxyRealm,
                256,
                string.Empty)
            : string.Empty;
        // El backend no publica el usuario al cargar preferencias. No se
        // conserva una copia local ni se inventa un valor visible.
        ProxyUsername = string.Empty;
    }

    private bool ApplyProxySecretStoreStatus(
        IpcCallResult<ProxySecretStoreStatusResult> result)
    {
        if (!result.IsSuccess ||
            result.Outcome != "success" ||
            result.Data is null)
        {
            SetProxyStoreUnavailable(
                "El motor local no confirmó el estado del almacén.");
            return false;
        }

        ProxySecretStoreAvailable = result.Data.Available;
        ProxySecurityStatusTitle = result.Data.Available
            ? "Almacén seguro disponible"
            : "Almacén seguro no disponible";
        // No se representa Reason: el backend puede incluir detalles internos.
        ProxySecurityStatusMessage = result.Data.Available
            ? result.Data.Backend switch
            {
                "dpapi-user" =>
                    "La contraseña se protege con DPAPI para el usuario actual de Windows.",
                "secret-service" =>
                    "La contraseña se protege en el almacén de secretos del sistema.",
                "keychain" =>
                    "La contraseña se protege en el llavero del sistema.",
                _ =>
                    "El sistema operativo confirmó un almacén protegido.",
            }
            : "La autenticación del proxy no puede configurarse hasta que el almacén protegido esté disponible.";
        ProxyRuntimeModeMessage = result.Data.RuntimeMode switch
        {
            "disabled" => "Modo en uso: proxy desactivado.",
            "system" => "Modo en uso: configuración del sistema.",
            "manual-secure-store" =>
                "Modo en uso: proxy manual con credenciales protegidas.",
            "manual-no-secret" =>
                "Modo en uso: proxy manual sin autenticación protegida.",
            "fail-closed" =>
                "Modo en uso: conexión bloqueada por configuración no segura.",
            "default-environment" =>
                "Modo en uso: configuración predeterminada del entorno.",
            _ => string.Empty,
        };
        return true;
    }

    private void SetProxyStoreUnavailable(string safeMessage)
    {
        ProxySecretStoreAvailable = false;
        ProxySecurityStatusTitle = "Almacén seguro no disponible";
        ProxySecurityStatusMessage = safeMessage;
        ProxyRuntimeModeMessage = string.Empty;
    }

    private bool IsProxyEndpointDirty()
    {
        if (_loadedSnapshot is null)
        {
            return false;
        }
        var currentHost = ProxyHost.Trim();
        var loadedHost = (_loadedSnapshot.ProxyHost ?? string.Empty).Trim();
        if (!string.Equals(
                currentHost,
                loadedHost,
                StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }
        return !int.TryParse(ProxyPortText, out var currentPort) ||
            currentPort != (_loadedSnapshot.ProxyPort ?? 8080);
    }

    private static bool TryNormalizeProxyCredentialText(
        string? value,
        out string normalized)
    {
        normalized = value?.Trim() ?? string.Empty;
        return normalized.Length > 0 &&
            Encoding.UTF8.GetByteCount(normalized) <=
                DesktopOperationsClient.MaximumProxyCredentialTextBytes &&
            !normalized.Any(char.IsControl);
    }

    private static bool TryParseProxyExcludedUrls(
        string? value,
        out IReadOnlyList<string> urls)
    {
        var parsed = new List<string>();
        var seen = new HashSet<string>(StringComparer.Ordinal);
        foreach (var raw in (value ?? string.Empty)
            .Split(['\r', '\n', ',', ';']))
        {
            var candidate = raw.Trim();
            if (candidate.Length == 0)
            {
                continue;
            }
            if (candidate.Length >
                    DesktopSettingsDocument
                        .MaximumProxyExcludedUrlCharacters ||
                candidate.Any(char.IsControl) ||
                !seen.Add(candidate))
            {
                urls = [];
                return false;
            }
            parsed.Add(candidate);
            if (parsed.Count >
                DesktopSettingsDocument.MaximumProxyExcludedUrlItems)
            {
                urls = [];
                return false;
            }
        }
        urls = parsed;
        return true;
    }

    private static SettingsOption FindOption(
        IEnumerable<SettingsOption> options,
        string value) =>
        options.First(option =>
            string.Equals(
                option.Value,
                value,
                StringComparison.Ordinal));
}
