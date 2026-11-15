// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Controls;
using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Automation;
using Microsoft.UI.Xaml.Automation.Peers;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Input;

namespace GrxFirma.WinUI.Views;

public sealed partial class SettingsPage
{
    // Cada fila se crea una vez y se actualiza en su sitio. Rehacerlas tras
    // activar o desactivar quitaba del árbol el botón con el foco, que caía en
    // «Idioma», y la página saltaba al final de la lista.
    private sealed class UnsafeRow
    {
        public required string Id { get; init; }
        public required StackPanel Panel { get; init; }
        public required TextBlock Title { get; init; }
        public required TextBlock Status { get; init; }
        public required TextBlock Policy { get; init; }
        public required Button Change { get; init; }
        public required Button Help { get; init; }
        public UnsafeExceptionState Item { get; set; } = new();
    }

    private readonly List<UnsafeRow> _unsafeRows = [];
    // Evita dos cambios a la vez sin desactivar el botón: un botón con el foco
    // que se desactiva lo pierde y el foco salta al control siguiente.
    private bool _unsafeBusy;

    private async Task LoadUnsafeExceptionsAsync(string? changedId = null)
    {
        UnsafeHeading.Text = Localizer.Text("excepciones.pantalla.preferencias");
        UnsafeDescription.Text = Localizer.Text("excepciones.pantalla.descripcion");
        UnsafeRefresh.Content = Localizer.Text("excepciones.pantalla.actualizar");
        var client = ((App)Application.Current).UnsafeExceptions;
        UnsafeRefresh.IsEnabled = client is not null;
        // «Consultando las excepciones…» solo sale si la consulta tarda: la
        // región viva lo anunciaba con la lista ya cargada (recorrido G).
        var loading = Localizer.Text("excepciones.pantalla.cargando");
        using var slow = new CancellationTokenSource();
        _ = ShowUnsafeLoadingLaterAsync(loading, slow.Token);
        try
        {
            if (client is null) { UnsafeMessage.Text = Localizer.Text("excepciones.canal_no_local"); return; }
            var result = await client.ListAsync(_pageCancellation?.Token ?? default);
            if (!_isLoaded) return;
            if (!result.IsSuccess || result.Data is null)
            { UnsafeMessage.Text = Localizer.Text("excepciones.pantalla.error"); return; }
            var items = result.Data;
            if (UnsafeRows.Children.Count != _unsafeRows.Count ||
                !_unsafeRows.Select(row => row.Id).SequenceEqual(items.Select(item => item.Id), StringComparer.Ordinal))
            {
                UnsafeRows.Children.Clear();
                _unsafeRows.Clear();
                foreach (var item in items)
                {
                    var row = CreateUnsafeRow(client, item.Id);
                    _unsafeRows.Add(row);
                    UnsafeRows.Children.Add(row.Panel);
                }
            }
            for (var index = 0; index < items.Count; index++) UpdateUnsafeRow(_unsafeRows[index], items[index]);
            var changed = changedId is null ? null
                : _unsafeRows.FirstOrDefault(row => string.Equals(row.Id, changedId, StringComparison.Ordinal));
            // La región viva confirma el cambio: «Evaluar firmas SHA-1 antiguas: Desactivado».
            UnsafeMessage.Text = changed is null ? string.Empty
                : Localizer.LabelValue(changed.Title.Text, changed.Status.Text);
            if (changed is not null) KeepUnsafeFocus(changed);
        }
        catch (OperationCanceledException) { }
        catch { UnsafeMessage.Text = Localizer.Text("excepciones.pantalla.error"); }
        finally
        {
            slow.Cancel();
            // Una consulta cancelada no deja el aviso de carga puesto.
            if (string.Equals(UnsafeMessage.Text, loading, StringComparison.Ordinal)) UnsafeMessage.Text = string.Empty;
        }
    }

    private async Task ShowUnsafeLoadingLaterAsync(string loading, CancellationToken token)
    {
        try { await Task.Delay(UnsafeLoadingDelay, token); }
        catch (OperationCanceledException) { return; }
        if (!token.IsCancellationRequested && _isLoaded) UnsafeMessage.Text = loading;
    }

    private static readonly TimeSpan UnsafeLoadingDelay = TimeSpan.FromMilliseconds(600);

    private UnsafeRow CreateUnsafeRow(UnsafeExceptionsClient client, string id)
    {
        static TextBlock Prose() => new() { TextWrapping = TextWrapping.Wrap };
        var title = Prose();
        title.FontWeight = Microsoft.UI.Text.FontWeights.SemiBold;
        AutomationProperties.SetHeadingLevel(title, AutomationHeadingLevel.Level3);
        var row = new UnsafeRow
        {
            Id = id,
            Panel = new StackPanel { Spacing = 8 },
            Title = title,
            Status = Prose(),
            Policy = Prose(),
            // Mismo índice que «Actualizar lista»: a igualdad, el foco
            // sigue el orden visual y no salta al final de la página.
            Change = new Button { MinHeight = 44, TabIndex = UnsafeRefresh.TabIndex },
            Help = new Button { MinHeight = 44, TabIndex = UnsafeRefresh.TabIndex },
        };
        row.Change.Click += async (_, _) =>
        {
            if (_unsafeBusy) return;
            var item = row.Item;
            if (item.Fixed || !(item.Enabled || item.Activable)) return;
            _unsafeBusy = true;
            try
            {
                if (item.Enabled)
                {
                    var disabled = await client.DisableAsync(item, _pageCancellation?.Token ?? default);
                    if (!disabled.IsSuccess) { UnsafeMessage.Text = Localizer.Text("excepciones.pantalla.error"); return; }
                }
                else if (!await client.ActivateAsync(item, ct: _pageCancellation?.Token ?? default)) return;
                await LoadUnsafeExceptionsAsync(item.Id);
            }
            catch (OperationCanceledException) { }
            catch { UnsafeMessage.Text = Localizer.Text("excepciones.pantalla.error"); }
            finally { _unsafeBusy = false; }
        };
        row.Help.Click += async (_, _) => await UnsafeExceptionDialog.ShowAsync(row.Item, XamlRoot, ActualTheme, true);
        var actions = new ResponsiveActionPanel { Spacing = 12 };
        actions.Children.Add(row.Change);
        actions.Children.Add(row.Help);
        row.Panel.Children.Add(row.Title);
        row.Panel.Children.Add(row.Status);
        row.Panel.Children.Add(row.Policy);
        row.Panel.Children.Add(actions);
        return row;
    }

    private static void UpdateUnsafeRow(UnsafeRow row, UnsafeExceptionState item)
    {
        row.Item = item;
        var setting = UnsafeExceptionDialog.CatalogText(item.SettingKey);
        row.Title.Text = setting;
        // Mismo formato que Qt: el del idioma de la interfaz, con el año en
        // cuatro cifras (lote 39h). Antes salía con el formato «g» de Windows.
        row.Status.Text = item.StatusKey == "excepciones.pantalla.activada_fecha"
            ? Localizer.Fill(item.StatusKey, ("date", UnsafeExceptionText.DateText(item.LastAcceptance!.AcceptedAt,
                Localizer.Text("format.datetime_minutes"), TimeZoneInfo.Local)))
            : Localizer.Text(item.StatusKey);
        // Una sola nota y sin repetir el estado: «solo lectura» si la
        // organización lo activa, su texto si lo prohíbe.
        row.Policy.Text = UnsafeExceptionDialog.CatalogText(UnsafeExceptionText.PolicyNoteKey(item));
        row.Policy.Visibility = row.Policy.Text.Length > 0 ? Visibility.Visible : Visibility.Collapsed;
        row.Change.Content = Localizer.Text(item.Enabled ? "excepciones.pantalla.desactivar" : "excepciones.pantalla.activar");
        row.Change.Visibility = item.Enabled || (item.Activable && !item.Fixed) ? Visibility.Visible : Visibility.Collapsed;
        row.Change.IsEnabled = !item.Fixed && (item.Enabled || item.Activable);
        row.Help.Content = Localizer.Text("excepciones.mas_informacion");
        AutomationProperties.SetHelpText(row.Change, setting);
        AutomationProperties.SetHelpText(row.Help, setting);
    }

    // El foco se queda en la fila del caso. Si el botón pulsado ya no se puede
    // usar (p. ej. la organización acaba de fijar el ajuste), pasa a «Más
    // información» de la misma fila.
    private void KeepUnsafeFocus(UnsafeRow row)
    {
        if (XamlRoot is null) return;
        var focused = FocusManager.GetFocusedElement(XamlRoot);
        var usable = row.Change.Visibility == Visibility.Visible && row.Change.IsEnabled;
        if ((usable && ReferenceEquals(focused, row.Change)) || ReferenceEquals(focused, row.Help)) return;
        var target = usable ? row.Change : row.Help;
        DispatcherQueue.TryEnqueue(Microsoft.UI.Dispatching.DispatcherQueuePriority.Low, () =>
        {
            if (!_isLoaded) return;
            target.Focus(FocusState.Programmatic);
            target.StartBringIntoView();
        });
    }

    private async void OnUnsafeRefresh(object sender, RoutedEventArgs args)
    {
        if (_unsafeBusy) return;
        _unsafeBusy = true;
        try { await LoadUnsafeExceptionsAsync(); }
        finally { _unsafeBusy = false; }
    }
}
