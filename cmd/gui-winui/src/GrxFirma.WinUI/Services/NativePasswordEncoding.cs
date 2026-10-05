// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Security.Cryptography;

namespace GrxFirma.WinUI.Services;

/// <summary>
/// Codifica el buffer UTF-16 nativo como bytes UTF-8 borrables sin crear una
/// cadena administrada intermedia.
/// </summary>
public static class NativePasswordEncoding
{
    private const uint CodePageUtf8 = 65001;
    private const uint ErrorOnInvalidCharacters = 0x00000080;

    public static byte[] ToUtf8(NativePasswordBuffer password)
    {
        ArgumentNullException.ThrowIfNull(password);
        return password.Use(static (pointer, characterCount) =>
        {
            if (characterCount == 0)
            {
                return [];
            }

            var requiredBytes = WideCharToMultiByte(
                CodePageUtf8,
                ErrorOnInvalidCharacters,
                pointer,
                characterCount,
                null,
                0,
                0,
                0);
            if (requiredBytes <= 0)
            {
                throw new Win32Exception(
                    Marshal.GetLastWin32Error(),
                    Localizer.Text("winui.contrasena.la_contrasena_contiene_texto_unicode_no"));
            }

            var utf8 = GC.AllocateUninitializedArray<byte>(
                requiredBytes);
            try
            {
                var written = WideCharToMultiByte(
                    CodePageUtf8,
                    ErrorOnInvalidCharacters,
                    pointer,
                    characterCount,
                    utf8,
                    utf8.Length,
                    0,
                    0);
                if (written != utf8.Length)
                {
                    throw new Win32Exception(
                        Marshal.GetLastWin32Error(),
                        Localizer.Text("winui.contrasena.windows_no_pudo_codificar_la_contrasena"));
                }
                return utf8;
            }
            catch
            {
                CryptographicOperations.ZeroMemory(utf8);
                throw;
            }
        });
    }

    [DllImport(
        "kernel32.dll",
        EntryPoint = "WideCharToMultiByte",
        ExactSpelling = true,
        SetLastError = true)]
    private static extern int WideCharToMultiByte(
        uint codePage,
        uint flags,
        nint wideCharacters,
        int wideCharacterCount,
        [Out] byte[]? utf8Bytes,
        int utf8ByteCount,
        nint defaultCharacter,
        nint usedDefaultCharacter);
}
