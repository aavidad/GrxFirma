// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Services;

public interface IFacePortalLauncherService
{
    Task<HelpLaunchResult> OpenValidatorAsync(
        CancellationToken cancellationToken = default);

    Task<HelpLaunchResult> OpenOrganisationDirectoryAsync(
        CancellationToken cancellationToken = default);

    Task<HelpLaunchResult> OpenSubmissionAsync(
        CancellationToken cancellationToken = default);

    Task<HelpLaunchResult> OpenInvoiceStatusAsync(
        CancellationToken cancellationToken = default);

    Task<HelpLaunchResult> OpenReceiptVerificationAsync(
        CancellationToken cancellationToken = default);
}
