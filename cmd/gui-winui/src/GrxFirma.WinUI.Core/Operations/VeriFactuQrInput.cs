// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Operations;

public static class VeriFactuQrInput
{
    // No pasar por Uri/IdnMapping: Windows puede normalizar autoridades que Go rechaza.
    // El motor vuelve a validar los cuatro parámetros antes de leer o consultar.
    public static void EnsureAllowedAuthority(string url)
    {
        ArgumentNullException.ThrowIfNull(url);
        if (url.Length is 0 or > 2048 || url.Any(char.IsControl) ||
            url.Contains('#') || url.Contains('\\')) throw new ArgumentException(null, nameof(url));
        string[] hosts = ["www2.agenciatributaria.gob.es", "prewww2.aeat.es"];
        string[] paths = ["/wlpl/TIKE-CONT/ValidarQR?", "/wlpl/TIKE-CONT/ValidarQRNoVerifactu?"];
        foreach (var host in hosts)
        foreach (var path in paths)
        {
            if (url.StartsWith("https://" + host + path, StringComparison.Ordinal)) return;
        }
        throw new ArgumentException(null, nameof(url));
    }

    // La página rasterizada viaja en una línea IPC de 4 MB y base64 añade un
    // tercio: el mismo tope que la vista previa nativa de PDF.
    public const int MaximumImagePayloadBytes = 2_900_000;

    // true: PDF, que la aplicación rasteriza con Windows.Data.Pdf porque el
    // motor de Windows no tiene Poppler. false: PNG o JPEG, que el motor lee
    // de la ruta con sus propios límites. Cualquier otra ruta se rechaza.
    public static bool IsPdfSource(string path)
    {
        ArgumentNullException.ThrowIfNull(path);
        if (path.Length is 0 or > 32_767 || path.Any(char.IsControl) ||
            path.Trim().Length != path.Length || !Path.IsPathFullyQualified(path))
            throw new ArgumentException(null, nameof(path));
        var extension = Path.GetExtension(path);
        if (string.Equals(extension, ".pdf", StringComparison.OrdinalIgnoreCase)) return true;
        string[] images = [".png", ".jpg", ".jpeg"];
        foreach (var image in images)
        {
            if (string.Equals(extension, image, StringComparison.OrdinalIgnoreCase)) return false;
        }
        throw new ArgumentException(null, nameof(path));
    }

    public static void EnsureImagePayload(byte[] image)
    {
        ArgumentNullException.ThrowIfNull(image);
        if (image.Length is 0 or > MaximumImagePayloadBytes)
            throw new ArgumentException(null, nameof(image));
    }
}
