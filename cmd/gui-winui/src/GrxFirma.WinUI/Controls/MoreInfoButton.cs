// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Automation;
using Microsoft.UI.Xaml.Automation.Peers;
using Microsoft.UI.Xaml.Automation.Provider;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Controls;

/// <summary>
/// Botón «+» de la ayuda: despliega la explicación ampliada de una opción y
/// pasa a «−» para plegarla. Es un botón normal (ratón, táctil, Intro o
/// Espacio) y el lector de pantalla lo anuncia como desplegable con su estado
/// (patrón ExpandCollapse). El nombre accesible lo pone quien lo crea.
/// </summary>
public sealed class MoreInfoButton : Button
{
    private const string CollapsedGlyph = "\uECC8"; // AddTo (círculo con «+») de Segoe Fluent Icons
    private const string ExpandedGlyph = "\uECC9"; // RemoveFrom (círculo con «−»)
    private readonly FontIcon _icon = new() { FontSize = 16, Glyph = CollapsedGlyph };
    private bool _isExpanded;

    public MoreInfoButton()
    {
        // 32 × 32: por encima del mínimo táctil de 24 px (WCAG 2.5.8) sin
        // descuadrar la línea de texto. Como el «?» de HelpButton, solo se
        // ve el glifo: sin fondo ni borde de botón; al pasar el ratón o con
        // el foco aparece el círculo suave del tema. Colores y foco, del tema.
        MinWidth = 32;
        MinHeight = 32;
        Padding = new Thickness(0);
        VerticalAlignment = VerticalAlignment.Top;
        Background = new Microsoft.UI.Xaml.Media.SolidColorBrush(Microsoft.UI.Colors.Transparent);
        BorderThickness = new Thickness(0);
        CornerRadius = new CornerRadius(16);
        Content = _icon;
        Click += (_, _) => IsExpanded = !IsExpanded;
    }

    /// <summary>Se lanza al desplegar o plegar.</summary>
    public event EventHandler? ExpandedChanged;

    public bool IsExpanded
    {
        get => _isExpanded;
        set
        {
            if (_isExpanded == value) return;
            _isExpanded = value;
            _icon.Glyph = value ? ExpandedGlyph : CollapsedGlyph;
            ExpandedChanged?.Invoke(this, EventArgs.Empty);
            if (FrameworkElementAutomationPeer.FromElement(this) is MoreInfoButtonAutomationPeer peer)
                peer.RaiseExpandCollapseChanged(!value, value);
        }
    }

    protected override AutomationPeer OnCreateAutomationPeer() => new MoreInfoButtonAutomationPeer(this);
}

/// <summary>Anuncia el «+» como botón desplegable con su estado.</summary>
public sealed class MoreInfoButtonAutomationPeer : ButtonAutomationPeer, IExpandCollapseProvider
{
    private readonly MoreInfoButton _owner;

    public MoreInfoButtonAutomationPeer(MoreInfoButton owner) : base(owner) => _owner = owner;

    public ExpandCollapseState ExpandCollapseState =>
        _owner.IsExpanded ? ExpandCollapseState.Expanded : ExpandCollapseState.Collapsed;

    public void Expand() => _owner.IsExpanded = true;

    public void Collapse() => _owner.IsExpanded = false;

    protected override object GetPatternCore(PatternInterface patternInterface) =>
        patternInterface == PatternInterface.ExpandCollapse ? this : base.GetPatternCore(patternInterface);

    internal void RaiseExpandCollapseChanged(bool wasExpanded, bool isExpanded) =>
        RaisePropertyChangedEvent(
            ExpandCollapsePatternIdentifiers.ExpandCollapseStateProperty,
            wasExpanded ? ExpandCollapseState.Expanded : ExpandCollapseState.Collapsed,
            isExpanded ? ExpandCollapseState.Expanded : ExpandCollapseState.Collapsed);
}
