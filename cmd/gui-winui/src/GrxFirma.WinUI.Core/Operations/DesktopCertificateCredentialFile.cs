// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Security.Cryptography;

using GrxFirma.WinUI.Core.Localization;

namespace GrxFirma.WinUI.Core.Operations;

/// <summary>
/// Valida y lee una credencial local con límites estrictos. El llamador toma
/// propiedad del buffer devuelto y debe borrarlo con
/// <see cref="CryptographicOperations.ZeroMemory(Span{byte})"/>.
/// </summary>
public static class DesktopCertificateCredentialFile
{
    private static readonly HashSet<string> SupportedExtensions =
        new(StringComparer.OrdinalIgnoreCase)
        {
            ".p12",
            ".pfx",
            ".pem",
            ".cer",
            ".crt",
        };

    public static bool IsSupportedPath(string? path)
    {
        if (string.IsNullOrWhiteSpace(path))
        {
            return false;
        }

        try
        {
            return SupportedExtensions.Contains(Path.GetExtension(path));
        }
        catch
        {
            return false;
        }
    }

    public static string SafeDisplayName(string? path)
    {
        if (string.IsNullOrWhiteSpace(path))
        {
            return CatalogLocalizer.Shared.TranslateVisibleText("Credencial seleccionada");
        }

        try
        {
            var fileName = Path.GetFileName(path);
            return string.IsNullOrWhiteSpace(fileName)
                ? CatalogLocalizer.Shared.TranslateVisibleText("Credencial seleccionada")
                : fileName;
        }
        catch
        {
            return CatalogLocalizer.Shared.TranslateVisibleText("Credencial seleccionada");
        }
    }

    public static void ValidateSelection(string path)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(path);
        if (!IsSupportedPath(path))
        {
            throw new InvalidDataException(
                CatalogLocalizer.Shared.TranslateVisibleText("Seleccione una credencial P12, PFX, PEM, CER o CRT."));
        }

        var fileInfo = new FileInfo(path);
        if (!fileInfo.Exists ||
            fileInfo.Length is <= 0 or
                > DesktopOperationsClient.MaximumCredentialBytes)
        {
            throw new InvalidDataException(
                CatalogLocalizer.Shared.TranslateVisibleText("La credencial está vacía, no está disponible o supera 2 MiB."));
        }
    }

    public static async Task<byte[]> ReadAsync(
        string path,
        CancellationToken cancellationToken = default)
    {
        ValidateSelection(path);

        FileStream? stream = null;
        byte[]? credential = null;
        try
        {
            stream = new FileStream(
                path,
                FileMode.Open,
                FileAccess.Read,
                FileShare.Read,
                bufferSize: 64 * 1024,
                FileOptions.Asynchronous |
                FileOptions.SequentialScan);
            var length = stream.Length;
            if (length is <= 0 or
                > DesktopOperationsClient.MaximumCredentialBytes)
            {
                throw new InvalidDataException(
                    CatalogLocalizer.Shared.TranslateVisibleText("La credencial está vacía o supera 2 MiB."));
            }

            credential = GC.AllocateUninitializedArray<byte>(
                checked((int)length));
            var offset = 0;
            while (offset < credential.Length)
            {
                var read = await stream.ReadAsync(
                    credential.AsMemory(offset),
                    cancellationToken).ConfigureAwait(false);
                if (read == 0)
                {
                    throw new InvalidDataException(
                        CatalogLocalizer.Shared.TranslateVisibleText("La credencial cambió mientras se leía."));
                }
                offset += read;
            }

            var extra = new byte[1];
            try
            {
                if (await stream.ReadAsync(
                    extra,
                    cancellationToken).ConfigureAwait(false) != 0)
                {
                    throw new InvalidDataException(
                        CatalogLocalizer.Shared.TranslateVisibleText("La credencial cambió mientras se leía."));
                }
            }
            finally
            {
                CryptographicOperations.ZeroMemory(extra);
            }

            var result = credential;
            credential = null;
            return result;
        }
        catch
        {
            if (credential is not null)
            {
                CryptographicOperations.ZeroMemory(credential);
            }
            throw;
        }
        finally
        {
            if (stream is not null)
            {
                await stream.DisposeAsync().ConfigureAwait(false);
            }
        }
    }
}
