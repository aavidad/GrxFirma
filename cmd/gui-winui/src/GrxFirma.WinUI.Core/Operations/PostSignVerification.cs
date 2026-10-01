// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Operations;

/// <summary>Prepara la verificación sin confundir contenido y contenedor firmado.</summary>
public static class PostSignVerification
{
    public static VerifyParameters Create(
        string outputPath,
        string inputPath,
        string? generatedFormat,
        string action)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(outputPath);
        // Solo una firma CAdES inicial dispone aquí del contenido original.
        // En cofirma/contrafirma inputPath es otro contenedor firmado: no sirve
        // como contenido externo de una firma detached. Sin el original real,
        // el verificador debe informar de evidencia insuficiente, nunca éxito.
        var provideOriginal = string.Equals(
            generatedFormat?.Trim(), "CAdES", StringComparison.OrdinalIgnoreCase) &&
            string.Equals(action?.Trim(), "sign", StringComparison.OrdinalIgnoreCase);
        if (provideOriginal)
        {
            ArgumentException.ThrowIfNullOrWhiteSpace(inputPath);
        }

        return new VerifyParameters
        {
            InputPath = outputPath,
            // Para formatos incorporados o un motor antiguo sin metadato,
            // se solicita autodetección. No se adivina el formato por extensión
            // ni se repite una verificación fallida ignorando un original.
            OriginalPath = provideOriginal ? inputPath : null,
        };
    }
}
