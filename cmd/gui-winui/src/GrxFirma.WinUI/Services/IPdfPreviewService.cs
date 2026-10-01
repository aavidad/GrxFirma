// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Services;

public interface IPdfPreviewService
{
    Task<PdfPreviewResult> RenderPageAsync(
        string path,
        int page,
        CancellationToken cancellationToken);
}
