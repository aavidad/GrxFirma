// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Operations;

public static class CertificateStructuredFilter
{
    public static bool Matches(CertificateInfo certificate, bool requireNif,
        bool requireOrganization, IReadOnlySet<string> types)
    {
        ArgumentNullException.ThrowIfNull(certificate);
        ArgumentNullException.ThrowIfNull(types);
        if (requireNif && string.IsNullOrWhiteSpace(certificate.Nif) &&
            string.IsNullOrWhiteSpace(certificate.SerialNumber)) return false;
        if (requireOrganization && string.IsNullOrWhiteSpace(certificate.Organization)) return false;
        return types.Count == 0 || types.Contains(certificate.Type.ToLowerInvariant());
    }
}
