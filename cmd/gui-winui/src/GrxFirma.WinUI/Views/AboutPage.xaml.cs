// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Services;
using GrxFirma.WinUI.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Views;

public sealed partial class AboutPage : Page
{
    private bool _isLoaded;

    public AboutPage()
    {
        var app = (App)Application.Current;
        ViewModel = new AboutPageViewModel(
            new WindowsHelpLauncherService(),
            app.OperationSession);
        InitializeComponent();
        var label = SealUiCatalog.Text(
            Localizer.Language,
            "Novedades");
        ReleaseNotesButton.Content = label;
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(
            ReleaseNotesButton, label);
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetHelpText(
            ReleaseNotesButton, label);
    }

    public AboutPageViewModel ViewModel { get; }

    private void OnLoaded(
        object sender,
        RoutedEventArgs args)
    {
        if (_isLoaded)
        {
            return;
        }

        _isLoaded = true;
        ViewModel.Activate();
    }

    private void OnUnloaded(
        object sender,
        RoutedEventArgs args)
    {
        if (!_isLoaded)
        {
            return;
        }

        _isLoaded = false;
        ViewModel.Deactivate();
    }

    private async void OnOpenOfficialLicenseClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.OpenOfficialLicenseAsync();

    private async void OnOpenOfficialProjectClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.OpenOfficialProjectAsync();

    private async void OnOpenOfficialReleasesClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.OpenOfficialReleasesAsync();

    private async void OnCheckUpdatesClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.CheckUpdatesAsync();

    private async void OnReleaseNotesClick(
        object sender, RoutedEventArgs args) =>
        await ((App)Application.Current).ShowReleaseNotesHistoryAsync();
}
