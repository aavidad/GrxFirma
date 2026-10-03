// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Diagnostics;

public sealed record SupportAssistantPlan(string Goal, string Destination,
    IReadOnlyList<string> StepKeys)
{
    public static SupportAssistantPlan ForGoal(string? goal) => goal switch
    {
        "verify" or "verify-failure" => new(goal!, "verify",
            ["winui.parity.support.verify.1", "winui.parity.support.verify.2", "winui.parity.support.verify.3"]),
        "certificate" => new("certificate", "certificates",
            ["winui.parity.support.certificate.1", "winui.parity.support.certificate.2", "winui.parity.support.certificate.3"]),
        "support" or "sign-failure" => new(goal!, "diagnostics",
            ["winui.parity.support.issue.1", "winui.parity.support.issue.2", "winui.parity.support.issue.3"]),
        _ => new("sign", "sign",
            ["winui.parity.support.sign.1", "winui.parity.support.sign.2", "winui.parity.support.sign.3"]),
    };

    public static string ExportText(string goalLabel, string currentStep,
        string summary, string owner, string responsibility,
        string suggestedAction, IReadOnlyList<string> steps)
    {
        static string Safe(string? value) => (value ?? string.Empty)
            .Replace('\r', ' ').Replace('\n', ' ').Trim();
        return string.Join(Environment.NewLine,
            new[] { Safe(goalLabel), Safe(currentStep), Safe(summary),
                Safe(owner), Safe(responsibility), Safe(suggestedAction) }
            .Concat(steps.Select((step, index) => $"{index + 1}. {Safe(step)}"))) +
            Environment.NewLine;
    }
}
