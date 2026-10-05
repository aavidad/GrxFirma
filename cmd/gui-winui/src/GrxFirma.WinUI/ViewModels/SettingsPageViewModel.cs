// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Security.Cryptography;
using System.Text;
using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.ViewModels;

public sealed record SettingsOption(
    string Label,
    string Value)
{
    public override string ToString() => Label;
}

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
    private SettingsOption _selectedSealLanguage;
    private SettingsOption _selectedProxyType;
    private string _proxyHost = string.Empty;
    private string _proxyPortText = "8080";
    private string _proxyExcludedUrlsText = string.Empty;
    private string _proxyRealm = string.Empty;
    private string _proxyUsername = string.Empty;
    private string _proxySecurityStatusTitle =
        Localizer.Text("winui.ajustes.estado_del_almacen_pendiente");
    private string _proxySecurityStatusMessage =
        Localizer.Text("winui.ajustes.actualice_el_estado_para_comprobar_la");
    private string _proxyRuntimeModeMessage = string.Empty;
    private string _validationMessage =
        Localizer.Text("winui.ajustes.cargue_las_preferencias_del_motor_local");
    private string _tsaValidationMessage = string.Empty;
    private string _statusTitle = Localizer.Text("winui.ajustes.sin_cambios");
    private string _statusMessage =
        Localizer.Text("winui.ajustes.las_preferencias_todavia_no_se_han");
    private bool _confirmBeforeSigning = true;
    private bool _stayResident = true;
    private bool _strictSignatureCompatibility = true;
    private bool _visiblePdfSeal;
    private bool _facturaeToolsEnabled;
    private bool _checkForUpdates = true;
    private bool _tsaEnabled;
    private string _tsaUrl = string.Empty;
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
            Localizer.Text("winui.comun.configuracion"),
            Localizer.Text("winui.ajustes.preferencias_compartidas_de_firma_red_y"),
            Localizer.Text("winui.ajustes.la_configuracion_no_esta_disponible"))
    {
        ArgumentNullException.ThrowIfNull(session);
        _session = session;
        _selectedLanguage = FindOption(Languages, Localizer.Language);
        _selectedTheme = Themes[0];
        _selectedDefaultFormat = DefaultFormats[0];
        _selectedSealLanguage = SealLanguages[0];
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

    // Idioma de los rótulos del sello visible. La primera opción sigue al
    // idioma de la aplicación; las demás lo fijan (p. ej. castellano para
    // documentos de la Administración).
    public IReadOnlyList<SettingsOption> SealLanguages { get; } =
    [
        new(Localizer.Text("winui.ajustes.el_mismo_que_la_aplicacion"),
            DesktopSettingsDocument.SealLanguageFollowsInterface),
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
        new(Localizer.Text("winui.ajustes.cristal_oscuro"), "0"),
        new(Localizer.Text("winui.ajustes.minimalista_claro"), "1"),
        new(Localizer.Text("winui.ajustes.futurista_oscuro_en_windows"), "2"),
    ];

    public IReadOnlyList<SettingsOption> DefaultFormats { get; } =
    [
        new(Localizer.Text("winui.comun.automatico"), ""),
        new("PAdES", "pades"),
        new("CAdES", "cades"),
        new("XAdES", "xades"),
        new("XMLDSig", "xmldsig"),
        new(Localizer.Text("winui.comun.odf"), "odf"),
        new(Localizer.Text("winui.comun.ooxml"), "ooxml"),
        new(Localizer.Text("winui.comun.facturae"), "facturae"),
        new(Localizer.Text("winui.comun.asic_xades"), "asic-xades"),
    ];

    public IReadOnlyList<SettingsOption> ProxyTypes { get; } =
    [
        new(Localizer.Text("winui.ajustes.sin_proxy"), "none"),
        new(Localizer.Text("winui.ajustes.manual"), "manual"),
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

    public SettingsOption SelectedSealLanguage
    {
        get => _selectedSealLanguage;
        set
        {
            if (value is not null &&
                SealLanguages.Contains(value) &&
                SetProperty(ref _selectedSealLanguage, value))
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

    public bool TsaEnabled
    {
        get => _tsaEnabled;
        set { if (SetProperty(ref _tsaEnabled, value)) MarkDirty(); }
    }

    public string TsaUrl
    {
        get => _tsaUrl;
        set { if (SetProperty(ref _tsaUrl, value ?? string.Empty)) MarkDirty(); }
    }

    public string TsaValidationMessage
    {
        get => _tsaValidationMessage;
        private set => SetProperty(ref _tsaValidationMessage, Localizer.Text(value));
    }

    public IReadOnlyDictionary<string, string> ValidateNetworkFields()
    {
        var errors = new Dictionary<string, string>(StringComparer.Ordinal);
        if (NetworkFormValidation.TsaError(TsaEnabled, TsaUrl) is { } tsaError)
            errors["tsa"] = tsaError;
        if (NetworkFormValidation.ProxyPortError(ProxyPortText) is { } portError)
            errors["proxyPort"] = portError;
        if (NetworkFormValidation.ProxyHostError(ProxyEnabled, SelectedProxyType.Value, ProxyHost) is { } hostError)
            errors["proxyHost"] = hostError;
        return errors;
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
            Localizer.Text(value));
    }

    public string ProxySecurityStatusMessage
    {
        get => _proxySecurityStatusMessage;
        private set => SetProperty(
            ref _proxySecurityStatusMessage,
            Localizer.Text(value));
    }

    public string ProxyRuntimeModeMessage
    {
        get => _proxyRuntimeModeMessage;
        private set => SetProperty(
            ref _proxyRuntimeModeMessage,
            Localizer.Text(value));
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
        private set => SetProperty(ref _validationMessage, Localizer.Text(value));
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
            Localizer.Text("winui.ajustes.cargando_preferencias"),
            Localizer.Text("winui.ajustes.se_esta_leyendo_la_configuracion"),
            InfoBarSeverity.Informational);
        try
        {
            var result = await operations.GetSettingsAsync(
                operationCancellation.Token);
            if (!result.IsSuccess || result.Outcome != "success")
            {
                ShowStatus(
                    Localizer.Text("winui.ajustes.no_se_pudo_cargar"),
                    result.SafeUserMessage,
                    InfoBarSeverity.Error);
                RequestDiagnostic(
                    OperationDiagnosticMapper.FromResult(result));
                return;
            }
            if (result.Data is null)
            {
                ShowIncoherentResult(
                    Localizer.Text("winui.ajustes.el_motor_local_no_devolvio_un_documento"));
                return;
            }

            ApplyLoadedSettings(result.Data);
            ThemePreferenceApplied?.Invoke(result.Data.ThemeIndex);
            ShowStatus(
                Localizer.Text("winui.ajustes.preferencias_cargadas"),
                Localizer.Text("winui.ajustes.puede_modificar_los_valores_y_guardarlos"),
                InfoBarSeverity.Success);
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            ShowStatus(
                Localizer.Text("winui.ajustes.carga_cancelada"),
                Localizer.Text("winui.ajustes.la_lectura_de_preferencias_se_detuvo"),
                InfoBarSeverity.Warning);
        }
        catch (Exception exception)
        {
            ShowStatus(
                Localizer.Text("winui.ajustes.no_se_pudo_cargar"),
                Localizer.Text("winui.ajustes.la_lectura_termino_sin_un_documento_de"),
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
            Localizer.Text("winui.ajustes.guardando_preferencias"),
            Localizer.Text("winui.ajustes.el_motor_local_esta_validando_y"),
            InfoBarSeverity.Informational);
        try
        {
            var result = await operations.SaveSettingsAsync(
                document,
                operationCancellation.Token);
            if (!result.IsSuccess || result.Outcome != "success")
            {
                ShowStatus(
                    Localizer.Text("winui.ajustes.no_se_pudo_guardar"),
                    result.SafeUserMessage,
                    InfoBarSeverity.Error);
                RequestDiagnostic(
                    OperationDiagnosticMapper.FromResult(result));
                return;
            }
            if (string.IsNullOrWhiteSpace(result.Data))
            {
                ShowIncoherentResult(
                    Localizer.Text("winui.ajustes.el_motor_local_no_confirmo_el_guardado"));
                return;
            }

            UpdatePreferenceStore.Write(CheckForUpdates);
            _loadedSnapshot = document.CreateSafeSaveSnapshot();
            IsDirty = false;
            ThemePreferenceApplied?.Invoke(document.ThemeIndex);
            UpdateValidation();
            ShowStatus(
                Localizer.Text("winui.ajustes.preferencias_guardadas"),
                Localizer.Text("winui.ajustes.los_cambios_se_han_persistido"),
                InfoBarSeverity.Success);
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            ShowStatus(
                Localizer.Text("winui.comun.guardado_cancelado"),
                Localizer.Text("winui.ajustes.no_se_recibio_confirmacion_de_que_los"),
                InfoBarSeverity.Warning);
        }
        catch (Exception exception)
        {
            ShowStatus(
                Localizer.Text("winui.ajustes.no_se_pudo_guardar"),
                Localizer.Text("winui.ajustes.el_guardado_termino_sin_una_confirmacion"),
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

        ProxySecurityStatusTitle = Localizer.Text("winui.ajustes.comprobando_almacen_seguro");
        ProxySecurityStatusMessage =
            Localizer.Text("winui.ajustes.windows_esta_comprobando_la_proteccion");
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
            ProxySecurityStatusTitle = Localizer.Text("winui.comun.comprobacion_cancelada");
            ProxySecurityStatusMessage =
                Localizer.Text("winui.ajustes.no_se_modificaron_las_credenciales_del");
        }
        catch (Exception exception)
        {
            SetProxyStoreUnavailable(
                Localizer.Text("winui.ajustes.no_se_pudo_comprobar_el_almacen"));
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
                        Localizer.Text("winui.ajustes.credenciales_no_validas"),
                        Localizer.Text("winui.ajustes.el_dominio_y_el_usuario_deben_tener"),
                        InfoBarSeverity.Warning);
                    return;
                }

                ShowStatus(
                    Localizer.Text("winui.ajustes.protegiendo_credenciales"),
                    Localizer.Text("winui.ajustes.la_contrasena_se_enviara_una_sola_vez_al"),
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
                        Localizer.Text("winui.ajustes.no_se_guardaron_las_credenciales"),
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
                        Localizer.Text("winui.ajustes.el_motor_local_no_confirmo_que_la"));
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
                            ? Localizer.Text("winui.ajustes.credenciales_cambiadas")
                            : Localizer.Text("winui.ajustes.credenciales_protegidas")
                        : Localizer.Text("winui.ajustes.credenciales_protegidas_estado_pendiente"),
                    refreshed
                        ? Localizer.Text("winui.ajustes.el_almacen_seguro_confirmo_el_estado")
                        : Localizer.Text("winui.ajustes.el_guardado_se_confirmo_pero_no_se_pudo"),
                    refreshed
                        ? InfoBarSeverity.Success
                        : InfoBarSeverity.Warning);
            }
            catch (OperationCanceledException)
                when (operationCancellation.IsCancellationRequested)
            {
                ShowStatus(
                    Localizer.Text("winui.comun.operacion_cancelada"),
                    Localizer.Text("winui.ajustes.no_se_recibio_confirmacion_de_que_las"),
                    InfoBarSeverity.Warning);
            }
            catch (Exception exception)
            {
                ShowStatus(
                    Localizer.Text("winui.ajustes.no_se_guardaron_las_credenciales"),
                    Localizer.Text("winui.ajustes.la_proteccion_local_de_la_contrasena_no"),
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
                Localizer.Text("winui.ajustes.quitando_credenciales"),
                Localizer.Text("winui.ajustes.el_almacen_protegido_esta_eliminando_la"),
                InfoBarSeverity.Informational);
            var result = await operations.DeleteProxySecretAsync(
                operationCancellation.Token);
            if (!result.IsSuccess ||
                result.Outcome != "success")
            {
                ShowStatus(
                    Localizer.Text("winui.ajustes.no_se_quitaron_las_credenciales"),
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
                    Localizer.Text("winui.ajustes.el_motor_local_no_confirmo_la"));
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
                    ? Localizer.Text("winui.ajustes.credenciales_eliminadas")
                    : Localizer.Text("winui.ajustes.credenciales_eliminadas_estado_pendiente"),
                refreshed
                    ? Localizer.Text("winui.ajustes.el_proxy_manual_ya_no_tiene")
                    : Localizer.Text("winui.ajustes.la_eliminacion_se_confirmo_pero_no_se"),
                refreshed
                    ? InfoBarSeverity.Success
                    : InfoBarSeverity.Warning);
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            ShowStatus(
                Localizer.Text("winui.comun.operacion_cancelada"),
                Localizer.Text("winui.ajustes.no_se_recibio_confirmacion_de_que_la"),
                InfoBarSeverity.Warning);
        }
        catch (Exception exception)
        {
            ShowStatus(
                Localizer.Text("winui.ajustes.no_se_quitaron_las_credenciales"),
                Localizer.Text("winui.ajustes.el_almacen_protegido_no_confirmo_la"),
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
            Localizer.Text("winui.ajustes.no_se_pudo_capturar_la_contrasena"),
            Localizer.Text("winui.ajustes.windows_no_pudo_preparar_la_contrasena"),
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
                    : Localizer.Language);
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
            SelectedSealLanguage = FindOption(
                SealLanguages,
                DesktopSettingsDocument.IsSupportedLanguage(
                    safeSnapshot.SealLanguage)
                    ? safeSnapshot.SealLanguage!
                    : DesktopSettingsDocument.SealLanguageFollowsInterface);
            TsaEnabled = safeSnapshot.TsaEnabled ?? false;
            TsaUrl = safeSnapshot.TsaUrl ?? string.Empty;
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
            SealLanguage = SelectedSealLanguage.Value,
            TsaEnabled = TsaEnabled,
            TsaUrl = TsaUrl.Trim(),
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
        TsaValidationMessage = TsaEnabled &&
            !TsaConfiguration.TryNormalize(true, TsaUrl, out _)
            ? GrxFirma.WinUI.Services.SealUiCatalog.Text(
                SelectedLanguage.Value, "winui.parity.tsa.invalid")
            : string.Empty;
        ValidationMessage = Validate();
        RefreshCommandState();
    }

    private string Validate()
    {
        if (!_hasLoaded)
        {
            return Localizer.Text("winui.ajustes.cargue_las_preferencias_del_motor_local");
        }
        if (!DesktopSettingsDocument.IsSupportedLanguage(
            SelectedLanguage.Value))
        {
            return Localizer.Text("winui.ajustes.seleccione_un_idioma_admitido");
        }
        if (!DesktopSettingsDocument.IsSupportedSealLanguage(
            SelectedSealLanguage.Value))
        {
            return Localizer.Text("winui.ajustes.seleccione_un_idioma_admitido");
        }
        if (!int.TryParse(SelectedTheme.Value, out var themeIndex) ||
            !DesktopSettingsDocument.IsSupportedTheme(themeIndex))
        {
            return Localizer.Text("winui.ajustes.seleccione_un_tema_visual_admitido");
        }
        if (!DesktopSettingsDocument.IsSupportedSignatureFormat(
            SelectedDefaultFormat.Value))
        {
            return Localizer.Text("winui.ajustes.seleccione_un_formato_de_firma_admitido");
        }
        if (VisiblePdfSeal &&
            SelectedDefaultFormat.Value is not ("" or "pades"))
        {
            return Localizer.Text("winui.ajustes.el_sello_visible_solo_es_compatible_con");
        }
        if (NetworkFormValidation.TsaError(TsaEnabled, TsaUrl) is not null)
        {
            return GrxFirma.WinUI.Services.SealUiCatalog.Text(
                SelectedLanguage.Value, "winui.parity.tsa.invalid");
        }
        if (!DesktopSettingsDocument.IsSupportedProxyType(
            SelectedProxyType.Value))
        {
            return Localizer.Text("winui.ajustes.seleccione_un_tipo_de_proxy_admitido");
        }
        if (NetworkFormValidation.ProxyPortError(ProxyPortText) is not null)
        {
            return Localizer.Text("winui.ajustes.el_puerto_del_proxy_debe_estar_entre_1_y");
        }
        if (ProxyEnabled && SelectedProxyType.Value == "none")
        {
            return Localizer.Text("winui.ajustes.seleccione_proxy_manual_o_desactive_el");
        }
        if (NetworkFormValidation.ProxyHostError(ProxyEnabled, SelectedProxyType.Value, ProxyHost) is not null)
            return Localizer.Text("winui.ajustes.indique_solo_el_host_o_la_ip_del_proxy");
        if (!TryParseProxyExcludedUrls(
                ProxyExcludedUrlsText,
                out _))
        {
            return Localizer.Text("winui.ajustes.las_exclusiones_del_proxy_admiten_hasta");
        }
        if (ProxyCredentialsConfigured &&
            IsProxyEndpointDirty())
        {
            return Localizer.Text("winui.ajustes.quite_primero_las_credenciales");
        }

        return IsDirty
            ? Localizer.Text("winui.ajustes.cambios_pendientes_de_guardar")
            : Localizer.Text("winui.ajustes.las_preferencias_cargadas_no_tienen");
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
            Localizer.Text("winui.ajustes.motor_local_listo_para_cargar_y_guardar"));
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
                Localizer.Text("winui.ajustes.cambios_pendientes_de_guardar"),
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
            Localizer.Text("winui.ajustes.respuesta_no_utilizable"),
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
                Localizer.Text("winui.ajustes.el_motor_local_no_confirmo_el_estado_del"));
            return false;
        }

        ProxySecretStoreAvailable = result.Data.Available;
        ProxySecurityStatusTitle = result.Data.Available
            ? Localizer.Text("winui.ajustes.almacen_seguro_disponible")
            : Localizer.Text("winui.ajustes.almacen_seguro_no_disponible");
        // No se representa Reason: el backend puede incluir detalles internos.
        ProxySecurityStatusMessage = result.Data.Available
            ? result.Data.Backend switch
            {
                "dpapi-user" =>
                    Localizer.Text("winui.ajustes.la_contrasena_se_protege_con_dpapi_para"),
                "secret-service" =>
                    Localizer.Text("winui.ajustes.la_contrasena_se_protege_en_el_almacen"),
                "keychain" =>
                    Localizer.Text("winui.ajustes.la_contrasena_se_protege_en_el_llavero"),
                _ =>
                    Localizer.Text("winui.ajustes.el_sistema_operativo_confirmo_un_almacen"),
            }
            : Localizer.Text("winui.ajustes.la_autenticacion_del_proxy_no_puede");
        ProxyRuntimeModeMessage = result.Data.RuntimeMode switch
        {
            "disabled" => Localizer.Text("winui.ajustes.modo_en_uso_proxy_desactivado"),
            "system" => Localizer.Text("winui.ajustes.modo_en_uso_configuracion_del_sistema"),
            "manual-secure-store" =>
                Localizer.Text("winui.ajustes.modo_en_uso_proxy_manual_con"),
            "manual-no-secret" =>
                Localizer.Text("winui.ajustes.modo_en_uso_proxy_manual_sin"),
            "fail-closed" =>
                Localizer.Text("winui.ajustes.modo_en_uso_conexion_bloqueada_por"),
            "default-environment" =>
                Localizer.Text("winui.ajustes.modo_en_uso_configuracion_predeterminada"),
            _ => string.Empty,
        };
        return true;
    }

    private void SetProxyStoreUnavailable(string safeMessage)
    {
        ProxySecretStoreAvailable = false;
        ProxySecurityStatusTitle = Localizer.Text("winui.ajustes.almacen_seguro_no_disponible");
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
