// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Security.Cryptography;

namespace GrxFirma.WinUI.Core.Ipc;

public static class CorrelationId
{
    public static string NewRequestId() => Create("winui");
    public static string NewTraceId() => Create("trace");

    private static string Create(string prefix)
    {
        Span<byte> random = stackalloc byte[16];
        RandomNumberGenerator.Fill(random);
        return $"{prefix}-{Convert.ToHexString(random).ToLowerInvariant()}";
    }
}
