// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Services;
using GrxFirma.WinUI.Controls;
using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Views;

public sealed partial class HashPage : Page
{
    private bool _isLoaded;

    public HashPage()
    {
        var app = (App)Application.Current;
        ViewModel = new HashPageViewModel(
            app.OperationSession,
            app.FilePickerService);
        InitializeComponent();
    }

    public HashPageViewModel ViewModel { get; }

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

    private void OnCreateChecked(
        object sender,
        RoutedEventArgs args) =>
        ViewModel.SetCreateMode(create: true);

    private void OnCheckChecked(
        object sender,
        RoutedEventArgs args) =>
        ViewModel.SetCreateMode(create: false);

    private void OnFileChecked(
        object sender,
        RoutedEventArgs args) =>
        ViewModel.SetDirectoryMode(directory: false);

    private void OnDirectoryChecked(
        object sender,
        RoutedEventArgs args) =>
        ViewModel.SetDirectoryMode(directory: true);

    private async void OnSelectInputClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.SelectInputAsync();

    private async void OnSelectManifestClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.SelectManifestAsync();

    private void OnAlgorithmSelectionChanged(
        object sender,
        SelectionChangedEventArgs args)
    {
        ViewModel.SetAlgorithm(
            HashAlgorithmCombo.SelectedItem as string);
    }

    private void OnFormatSelectionChanged(
        object sender,
        SelectionChangedEventArgs args)
    {
        ViewModel.SetFormat(
            HashFormatCombo.SelectedItem as
                HashPageViewModel.HashFormatOption);
    }

    private void OnRecursiveChecked(
        object sender,
        RoutedEventArgs args) =>
        ViewModel.SetRecursive(recursive: true);

    private void OnRecursiveUnchecked(
        object sender,
        RoutedEventArgs args) =>
        ViewModel.SetRecursive(recursive: false);

    private async void OnExecuteClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.ExecuteAsync();

    private void OnCancelClick(
        object sender,
        RoutedEventArgs args) =>
        ViewModel.CancelCurrentOperation();

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
