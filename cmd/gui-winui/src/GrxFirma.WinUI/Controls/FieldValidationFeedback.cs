// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Runtime.CompilerServices;
using Microsoft.UI.Xaml.Automation;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Controls;

// Marca un campo como erróneo sin perder la ayuda que ya tenía en el XAML:
// el lector oye primero el error y después la ayuda original.
public static class FieldValidationFeedback
{
    private sealed class HelpState
    {
        public string BaseHelp { get; set; } = string.Empty;
        public string? Composed { get; set; }
    }

    private static readonly ConditionalWeakTable<Control, HelpState> s_states = new();

    public static void Apply(Control field, string detail)
    {
        var current = AutomationProperties.GetHelpText(field) ?? string.Empty;
        var state = s_states.GetValue(field, _ => new HelpState());
        // Si otra parte (p. ej. el cambio de idioma) reescribió la ayuda, esa
        // pasa a ser la ayuda base.
        if (state.Composed is null || !string.Equals(current, state.Composed, StringComparison.Ordinal))
            state.BaseHelp = current;
        var help = Compose(detail, state.BaseHelp);
        state.Composed = help;
        AutomationProperties.SetHelpText(field, help);
        if (detail.Length > 0) field.BorderBrush = ThemeBrushes.Get(ThemeBrushes.Failure, field);
        else field.ClearValue(Control.BorderBrushProperty);
    }

    public static string Compose(string detail, string baseHelp)
    {
        if (string.IsNullOrWhiteSpace(detail)) return baseHelp;
        if (string.IsNullOrWhiteSpace(baseHelp)) return detail;
        return detail + " " + baseHelp;
    }
}
