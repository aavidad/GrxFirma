// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Controls;
using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Services;
using GrxFirma.WinUI.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Views;

public sealed partial class DiagnosticsPage : Page
{
    private bool _isLoaded;
    private int _supportStepIndex;
    private readonly IReadOnlyList<SettingsOption> _supportGoals =
    [
        new(SealUiCatalog.Text(System.Globalization.CultureInfo.CurrentUICulture.TwoLetterISOLanguageName, "winui.parity.support.goal.sign"), "sign"),
        new(SealUiCatalog.Text(System.Globalization.CultureInfo.CurrentUICulture.TwoLetterISOLanguageName, "winui.parity.support.goal.verify"), "verify"),
        new(SealUiCatalog.Text(System.Globalization.CultureInfo.CurrentUICulture.TwoLetterISOLanguageName, "winui.parity.support.goal.certificate"), "certificate"),
        new(SealUiCatalog.Text(System.Globalization.CultureInfo.CurrentUICulture.TwoLetterISOLanguageName, "winui.parity.support.goal.sign_failure"), "sign-failure"),
        new(SealUiCatalog.Text(System.Globalization.CultureInfo.CurrentUICulture.TwoLetterISOLanguageName, "winui.parity.support.goal.verify_failure"), "verify-failure"),
        new(SealUiCatalog.Text(System.Globalization.CultureInfo.CurrentUICulture.TwoLetterISOLanguageName, "winui.parity.support.goal.issue"), "support"),
    ];

    public DiagnosticsPage()
    {
        var app = (App)Application.Current;
        ViewModel = new DiagnosticsPageViewModel(
            app.OperationSession);
        InitializeComponent();
        SupportAssistantTitle.Text = SupportLabel("winui.parity.support.title");
        SupportGoalCombo.Header = SupportLabel("winui.parity.support.goal");
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(
            SupportGoalCombo, (string)SupportGoalCombo.Header);
        SupportGoalCombo.ItemsSource = _supportGoals;
        SupportGoalCombo.SelectedIndex = 0;
        SetSupportButton(SupportNextButton, SupportLabel("winui.parity.support.next"));
        SetSupportButton(SupportExportButton, SupportLabel("winui.parity.support.export"));
        UpdateSupportStep();
    }

    private static void SetSupportButton(Button button, string label)
    {
        button.Content = label;
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(button, label);
    }

    private static string SupportLabel(string key) => SealUiCatalog.Text(
        System.Globalization.CultureInfo.CurrentUICulture.TwoLetterISOLanguageName, key);

    private SupportAssistantPlan CurrentSupportPlan =>
        SupportAssistantPlan.ForGoal((SupportGoalCombo.SelectedItem as SettingsOption)?.Value);

    private void UpdateSupportStep()
    {
        var plan = CurrentSupportPlan;
        _supportStepIndex %= plan.StepKeys.Count;
        SupportStepText.Text = $"{_supportStepIndex + 1}/{plan.StepKeys.Count}: " +
            SupportLabel(plan.StepKeys[_supportStepIndex]);
        SetSupportButton(SupportPrimaryButton,
            SupportLabel(plan.Destination == "diagnostics"
                ? "winui.parity.support.primary.diagnostics"
                : "winui.parity.support.primary." + plan.Destination));
    }

    private void OnSupportGoalChanged(object sender, SelectionChangedEventArgs args)
    {
        if (SupportStepText is null) return;
        _supportStepIndex = 0;
        UpdateSupportStep();
    }

    private void OnSupportNextClick(object sender, RoutedEventArgs args)
    {
        _supportStepIndex = (_supportStepIndex + 1) % CurrentSupportPlan.StepKeys.Count;
        UpdateSupportStep();
    }

    private async void OnSupportPrimaryClick(object sender, RoutedEventArgs args)
    {
        var destination = CurrentSupportPlan.Destination;
        if (destination == "diagnostics")
        {
            await ViewModel.RunDiagnosticAsync();
            SupportStatusText.Text = ViewModel.CanRun
                ? SupportLabel("winui.parity.support.diagnostic_ready")
                : ViewModel.PendingMessage;
        }
        else
        {
            ((App)Application.Current).ShowSupportTargetPage(destination);
        }
    }

    private async void OnSupportExportClick(object sender, RoutedEventArgs args)
    {
        var app = (App)Application.Current;
        var plan = CurrentSupportPlan;
        var goal = (SupportGoalCombo.SelectedItem as SettingsOption)?.Label ?? string.Empty;
        var report = SupportAssistantPlan.ExportText(
            SupportLabel("winui.parity.support.goal") + ": " + goal,
            SupportLabel("winui.parity.support.current") + ": " + SupportStepText.Text,
            SupportLabel("winui.parity.support.summary") + ": " + ViewModel.SummaryMessage,
            SupportLabel("winui.parity.support.owner") + ": " + ViewModel.OwnerLabel,
            SupportLabel("winui.parity.support.responsibility") + ": " + ViewModel.Responsibility,
            SupportLabel("winui.parity.support.suggestion") + ": " + ViewModel.SuggestedAction,
            plan.StepKeys.Select(SupportLabel).ToArray());
        try
        {
            var saved = await app.FilePickerService.PickAndSaveTextFileAsync(
                SaveFilePickerProfile.SupportIncidentText, report);
            if (saved) SupportStatusText.Text = SupportLabel("winui.parity.support.saved");
        }
        catch (Exception)
        {
            SupportStatusText.Text = SupportLabel("winui.parity.support.save_error");
        }
    }

    public DiagnosticsPageViewModel ViewModel { get; }

    private void OnLoaded(object sender, RoutedEventArgs args)
    {
        if (_isLoaded)
        {
            return;
        }

        _isLoaded = true;
        ViewModel.DiagnosticRequested += OnDiagnosticRequested;
        ViewModel.Activate();
    }

    private void OnUnloaded(object sender, RoutedEventArgs args)
    {
        if (!_isLoaded)
        {
            return;
        }

        _isLoaded = false;
        ViewModel.DiagnosticRequested -= OnDiagnosticRequested;
        ViewModel.Deactivate();
    }

    private async void OnRunDiagnosticClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.RunDiagnosticAsync();

    private void OnCancelClick(
        object sender,
        RoutedEventArgs args) =>
        ViewModel.CancelCurrentOperation();

    private void OnOpenDetailedDiagnosticClick(
        object sender,
        RoutedEventArgs args) =>
        ViewModel.RequestDetailedDiagnostic();

    private async void OnRefreshTlsClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.RefreshTlsStatusAsync();

    private async void OnInstallTlsTrustClick(
        object sender,
        RoutedEventArgs args)
    {
        if (!_isLoaded || XamlRoot is null)
        {
            return;
        }

        var confirmation = new ContentDialog
        {
            XamlRoot = XamlRoot,
            RequestedTheme = ActualTheme,
            Title = "Instalar confianza TLS local",
            Content =
                "GrxFirma creará, si es necesario, una CA local propia y la confiará únicamente para el usuario actual. No se modifican certificados personales ni raíces ajenas.",
            PrimaryButtonText = "Instalar confianza",
            CloseButtonText = "Cancelar",
            DefaultButton = ContentDialogButton.Close,
        };
        if (await Localizer.ShowAsync(confirmation) ==
            ContentDialogResult.Primary)
        {
            await ViewModel.InstallTlsTrustAsync();
        }
    }

    private async void OnClearTlsTrustClick(
        object sender,
        RoutedEventArgs args)
    {
        if (!_isLoaded || XamlRoot is null)
        {
            return;
        }

        var confirmation = new ContentDialog
        {
            XamlRoot = XamlRoot,
            RequestedTheme = ActualTheme,
            Title = "Retirar confianza TLS local",
            Content =
                "Se retirará primero la CA local inventariada y después sus artefactos gestionados. La firma desde navegador dejará de funcionar hasta volver a instalarla.",
            PrimaryButtonText = "Retirar confianza",
            CloseButtonText = "Cancelar",
            DefaultButton = ContentDialogButton.Close,
        };
        if (await Localizer.ShowAsync(confirmation) ==
            ContentDialogResult.Primary)
        {
            await ViewModel.ClearTlsTrustAsync();
        }
    }

    private async void OnDiagnosticRequested(
        OperationDiagnostic diagnostic)
    {
        if (!_isLoaded || XamlRoot is null)
        {
            return;
        }

        var dialog = new OperationDiagnosticDialog(diagnostic)
        {
            XamlRoot = XamlRoot,
            RequestedTheme = ActualTheme,
        };
        await Localizer.ShowAsync(dialog);
    }
}
