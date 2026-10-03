// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;

namespace GrxFirma.WinUI.Core.Operations;

public static class VerificationReport
{
    public static string Serialize(
        VerifyResult result,
        string? inputPath,
        string? originalPath,
        DateTimeOffset generatedAt)
    {
        ArgumentNullException.ThrowIfNull(result);
        var report = new Dictionary<string, object?>
        {
            ["generatedAt"] = generatedAt.ToUniversalTime().ToString("O"),
            ["source"] = "grxfirma-gui",
            ["result"] = result,
        };
        if (!string.IsNullOrWhiteSpace(inputPath)) report["inputPath"] = inputPath;
        if (!string.IsNullOrWhiteSpace(originalPath)) report["originalPath"] = originalPath;
        return JsonSerializer.Serialize(report, new JsonSerializerOptions
        {
            WriteIndented = true,
            PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
        });
    }
}
