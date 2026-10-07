// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Controls;

public sealed partial class SigningMethodsHelp : UserControl
{
    public SigningMethodsHelp()
    {
        InitializeComponent();
        // El UserControl no recibe el foco: su TabIndex se pasa al Expander
        // para que ocupe su sitio en el orden de la página y no caiga al final.
        IsTabStop = false;
        MethodsExpander.TabIndex = TabIndex;
        RegisterPropertyChangedCallback(TabIndexProperty,
            (_, _) => MethodsExpander.TabIndex = TabIndex);
    }
}
