// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Views;

/// <summary>
/// «Firmas desde los portales»: elige si GrxFirma o AutoFirma (la aplicación
/// Java del Gobierno) abre los enlaces afirma:// de las webs. El estado y el
/// cambio los hace el motor con la API del registro; aquí solo se muestran.
/// </summary>
public sealed partial class SettingsPage
{
    private bool _afirmaBusy;
    private bool _afirmaApplyingView;
    private string _afirmaCurrent = AfirmaHandlerChoices.None;

    private static DesktopOperationSession Session =>
        ((App)Application.Current).OperationSession;

    private void ApplyAfirmaHandlerLabels()
    {
        AfirmaHandlerTitle.Text = Localizer.Text("protocolo.titulo");
        AfirmaHandlerDescription.Text = Localizer.Text("protocolo.descripcion");
        AfirmaGrxFirmaRadio.Content = Localizer.Text("protocolo.opcion.grxfirma");
        AfirmaAutoFirmaRadio.Content = Localizer.Text("protocolo.opcion.autofirma");
        AfirmaHandlerRefreshButton.Content = Localizer.Text("protocolo.comprobar");
        AfirmaAutoFirmaMissingText.Text = Localizer.Text(AfirmaHandlerPresentation.AutoFirmaMissing);
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(
            AfirmaHandlerOptions, Localizer.Text("protocolo.selector"));
    }

    private void OnAfirmaSessionAvailabilityChanged(object? sender, EventArgs args) =>
        DispatcherQueue.TryEnqueue(async () => await RefreshAfirmaHandlerAsync());

    private async Task RefreshAfirmaHandlerAsync()
    {
        var cancellation = _pageCancellation;
        if (cancellation is null || _afirmaBusy)
        {
            return;
        }
        if (!Session.TryGetOperations(DesktopOperationActions.AfirmaHandlerStatus, out var operations))
        {
            AfirmaHandlerCard.Visibility = Visibility.Collapsed;
            return;
        }

        SetAfirmaBusy(true, AfirmaHandlerPresentation.StatusChecking);
        try
        {
            var result = await operations.GetAfirmaHandlerStatusAsync(cancellation.Token);
            if (result.IsSuccess)
            {
                ApplyAfirmaView(AfirmaHandlerPresentation.From(result.Data), null);
            }
            else
            {
                AfirmaHandlerStatusText.Text = Localizer.Text(AfirmaHandlerPresentation.StatusError);
            }
        }
        catch (OperationCanceledException) when (cancellation.IsCancellationRequested)
        {
        }
        catch (Exception)
        {
            AfirmaHandlerStatusText.Text = Localizer.Text(AfirmaHandlerPresentation.StatusError);
        }
        finally
        {
            SetAfirmaBusy(false, null);
        }
    }

    private void ApplyAfirmaView(AfirmaHandlerView view, string? extraKey)
    {
        AfirmaHandlerCard.Visibility = view.Visible ? Visibility.Visible : Visibility.Collapsed;
        if (!view.Visible)
        {
            return;
        }
        _afirmaApplyingView = true;
        try
        {
            AfirmaGrxFirmaRadio.IsChecked = view.GrxFirmaSelected;
            AfirmaAutoFirmaRadio.IsChecked = view.AutoFirmaSelected;
            AfirmaGrxFirmaRadio.Tag = view.CanChooseGrxFirma;
            AfirmaAutoFirmaRadio.Tag = view.CanChooseAutoFirma;
        }
        finally
        {
            _afirmaApplyingView = false;
        }
        _afirmaCurrent = view.GrxFirmaSelected
            ? AfirmaHandlerChoices.GrxFirma
            : view.AutoFirmaSelected ? AfirmaHandlerChoices.AutoFirma : AfirmaHandlerChoices.None;
        AfirmaAutoFirmaMissingText.Visibility =
            view.ShowAutoFirmaMissing ? Visibility.Visible : Visibility.Collapsed;
        var status = view.StatusArgument is null
            ? Localizer.Text(view.StatusKey)
            : Localizer.Format(view.StatusKey, view.StatusArgument);
        AfirmaHandlerStatusText.Text = extraKey is null
            ? status
            : status + " " + Localizer.Text(extraKey);
        UpdateAfirmaControls();
    }

    private void SetAfirmaBusy(bool busy, string? statusKey)
    {
        _afirmaBusy = busy;
        if (statusKey is not null)
        {
            AfirmaHandlerStatusText.Text = Localizer.Text(statusKey);
        }
        UpdateAfirmaControls();
    }

    private void UpdateAfirmaControls()
    {
        AfirmaGrxFirmaRadio.IsEnabled = !_afirmaBusy && AfirmaGrxFirmaRadio.Tag is true;
        AfirmaAutoFirmaRadio.IsEnabled = !_afirmaBusy && AfirmaAutoFirmaRadio.Tag is true;
        AfirmaHandlerRefreshButton.IsEnabled = !_afirmaBusy;
    }

    private async void OnRefreshAfirmaHandlerClick(object sender, RoutedEventArgs args) =>
        await RefreshAfirmaHandlerAsync();

    private async void OnAfirmaHandlerChecked(object sender, RoutedEventArgs args)
    {
        var handler = ReferenceEquals(sender, AfirmaAutoFirmaRadio)
            ? AfirmaHandlerChoices.AutoFirma
            : AfirmaHandlerChoices.GrxFirma;
        var cancellation = _pageCancellation;
        if (_afirmaApplyingView || _afirmaBusy || cancellation is null ||
            handler == _afirmaCurrent ||
            !Session.TryGetOperations(DesktopOperationActions.AfirmaHandlerSelect, out var operations))
        {
            return;
        }

        SetAfirmaBusy(true, AfirmaHandlerPresentation.StatusChanging);
        string? failureKey = null;
        try
        {
            var result = await operations.SelectAfirmaHandlerAsync(handler, cancellation.Token);
            if (result.IsSuccess)
            {
                SetAfirmaBusy(false, null);
                ApplyAfirmaView(
                    AfirmaHandlerPresentation.From(result.Data),
                    AfirmaHandlerPresentation.ReloadPortal);
                return;
            }
            failureKey = AfirmaHandlerPresentation.ErrorKey(result.ErrorCode);
        }
        catch (OperationCanceledException) when (cancellation.IsCancellationRequested)
        {
            return;
        }
        catch (Exception)
        {
            failureKey = AfirmaHandlerPresentation.ErrorKey(null);
        }
        finally
        {
            if (_afirmaBusy)
            {
                SetAfirmaBusy(false, null);
            }
        }

        // Vuelve a mostrar el estado real y explica por qué no cambió.
        await RefreshAfirmaHandlerAsync();
        if (failureKey is not null)
        {
            AfirmaHandlerStatusText.Text = Localizer.Text(failureKey) + " " + AfirmaHandlerStatusText.Text;
        }
    }
}
