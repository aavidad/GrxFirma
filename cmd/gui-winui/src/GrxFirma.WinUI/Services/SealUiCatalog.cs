// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Services;

internal static class SealUiCatalog
{
    private const string LogoOpacityKey = "sign.seal.opacity";

    public static string LogoOpacityLabel(string? language)
        => Text(language, LogoOpacityKey);

    public static string SealOpacityHelp(string? language)
        => Text(language, "sign.seal.opacity_help");

    public static string Text(string? language, string key)
        => Localizer.Text(key);
}
