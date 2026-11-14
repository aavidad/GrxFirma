// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Operations;

/// <summary>
/// Qué ficheros puede «Abrir» la interfaz con el programa asociado de
/// Windows. ShellExecute ejecuta los tipos ejecutables (.exe, .lnk, .hta,
/// .js, .scr, .bat...) y el nombre de un documento desprotegido lo elige
/// quien envió el sobre. Solo se abren tipos documentales; para el resto se
/// muestra la carpeta. Es la misma lista que conserva el backend al
/// desproteger (handler.go, extensionesDocumentalesDesprotegidas).
/// </summary>
public static class OutputOpeningPolicy
{
    private static readonly HashSet<string> DocumentExtensions = new(
        new[]
        {
            ".pdf", ".xml", ".txt", ".csv", ".json",
            ".docx", ".xlsx", ".pptx", ".odt", ".ods", ".odp",
            ".png", ".jpg", ".jpeg", ".gif", ".tif", ".tiff",
            ".xsig", ".csig", ".p7s", ".p7m", ".asice", ".asics",
        },
        StringComparer.OrdinalIgnoreCase);

    public static bool IsDocumentToOpen(string? path)
    {
        if (string.IsNullOrWhiteSpace(path))
        {
            return false;
        }

        var extension = Path.GetExtension(path);
        if (extension.Length < 2 ||
            !extension.Skip(1).All(char.IsAsciiLetterOrDigit))
        {
            return false;
        }

        return DocumentExtensions.Contains(extension);
    }
}
