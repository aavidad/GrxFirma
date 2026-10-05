// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Services;
using GrxFirma.WinUI.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Views;

public sealed partial class HelpPage : Page
{
    private bool _isLoaded;

    public HelpPage()
    {
        ViewModel = new HelpPageViewModel(
            new WindowsHelpLauncherService());
        InitializeComponent();
        var label = Localizer.Text("winui.comun.novedades");
        ReleaseNotesButton.Content = label;
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(
            ReleaseNotesButton, label);
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetHelpText(
            ReleaseNotesButton, label);
    }

    public HelpPageViewModel ViewModel { get; }

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

    private async void OnOpenInstalledManualClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.OpenInstalledManualAsync();

    private async void OnOpenInstallationFolderClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.OpenInstallationFolderAsync();

    private async void OnOpenOfficialProjectClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.OpenOfficialProjectAsync();

    private async void OnOpenPrivateSupportClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.OpenPrivateSupportAsync();

    private async void OnReleaseNotesClick(
        object sender, RoutedEventArgs args) =>
        await ((App)Application.Current).ShowReleaseNotesHistoryAsync();
}
