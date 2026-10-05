// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Localization;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Automation;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;

namespace GrxFirma.WinUI.Services;

// En una aplicación sin empaquetar, DatePicker y TimePicker toman del idioma
// de Windows el nombre del mes y el nombre que oye el lector de pantalla
// («Selector de fecha … 5 de octubre de 2026»), aunque la aplicación esté en
// otro idioma. El mes se muestra en número y el botón del selector recibe un
// nombre propio: su rótulo y la fecha u hora con el formato del idioma de la
// aplicación.
internal static class PickerLanguage
{
    public const string NumericMonth = "{month.integer(2)}";

    public static void Attach(DatePicker picker)
    {
        ArgumentNullException.ThrowIfNull(picker);
        picker.MonthFormat = NumericMonth;
        picker.Loaded += (_, _) => Update(picker);
        picker.SelectedDateChanged += (_, _) => Update(picker);
    }

    public static void Attach(TimePicker picker)
    {
        ArgumentNullException.ThrowIfNull(picker);
        picker.Loaded += (_, _) => Update(picker);
        picker.SelectedTimeChanged += (_, _) => Update(picker);
    }

    private static void Update(DatePicker picker)
    {
        var culture = AppCulture.For(Localizer.Language);
        SetButtonName(picker, Label(picker), picker.SelectedDate?.ToString("D", culture));
    }

    private static void Update(TimePicker picker)
    {
        var culture = AppCulture.For(Localizer.Language);
        SetButtonName(picker, Label(picker),
            picker.SelectedTime is { } time ? DateTime.Today.Add(time).ToString("t", culture) : null);
    }

    private static string Label(Control picker)
    {
        var name = AutomationProperties.GetName(picker);
        if (!string.IsNullOrWhiteSpace(name)) return name;
        return picker switch
        {
            DatePicker date => date.Header?.ToString() ?? string.Empty,
            TimePicker time => time.Header?.ToString() ?? string.Empty,
            _ => string.Empty,
        };
    }

    private static void SetButtonName(DependencyObject picker, string label, string? value)
    {
        if (FindFlyoutButton(picker) is not { } button) return;
        AutomationProperties.SetName(button,
            string.IsNullOrWhiteSpace(value)
                ? label
                : Localizer.Fill("winui.selector.etiqueta_valor", ("label", label), ("value", value)));
    }

    private static Button? FindFlyoutButton(DependencyObject root)
    {
        var count = VisualTreeHelper.GetChildrenCount(root);
        for (var i = 0; i < count; i++)
        {
            var child = VisualTreeHelper.GetChild(root, i);
            if (child is Button { Name: "FlyoutButton" } button) return button;
            if (FindFlyoutButton(child) is { } found) return found;
        }
        return null;
    }
}
