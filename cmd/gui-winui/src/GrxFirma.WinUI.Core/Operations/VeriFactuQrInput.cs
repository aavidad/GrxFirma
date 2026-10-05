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
}
