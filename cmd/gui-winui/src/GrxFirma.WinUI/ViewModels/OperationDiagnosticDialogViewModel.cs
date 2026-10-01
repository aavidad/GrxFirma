// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Diagnostics;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.ViewModels;

public sealed class OperationDiagnosticDialogViewModel
{
    public OperationDiagnosticDialogViewModel(OperationDiagnostic diagnostic)
    {
        Presentation = OperationDiagnosticPresentation.From(diagnostic);
        FailureCodeVisibility = string.IsNullOrWhiteSpace(Presentation.FailureCode)
            ? Visibility.Collapsed
            : Visibility.Visible;
        TimelineVisibility = Presentation.Steps.Count == 0
            ? Visibility.Collapsed
            : Visibility.Visible;
        EmptyTimelineVisibility = Presentation.Steps.Count == 0
            ? Visibility.Visible
            : Visibility.Collapsed;
        CauseSeverity =
            !string.IsNullOrWhiteSpace(Presentation.FailureCode) ||
            Presentation.Steps.Any(step =>
                step.Status == DiagnosticStepStatus.Failure)
            ? InfoBarSeverity.Error
            : InfoBarSeverity.Warning;
    }

    public OperationDiagnosticPresentation Presentation { get; }
    public Visibility FailureCodeVisibility { get; }
    public Visibility TimelineVisibility { get; }
    public Visibility EmptyTimelineVisibility { get; }
    public InfoBarSeverity CauseSeverity { get; }
}
