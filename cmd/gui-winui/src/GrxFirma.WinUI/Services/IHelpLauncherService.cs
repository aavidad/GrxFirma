// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Services;

public sealed record HelpLaunchResult(
    bool Succeeded,
    string Message);

// Las operaciones no reciben rutas ni URI. Cada destino pertenece a una lista
// cerrada mantenida por la aplicación para que datos de IPC o de documentos no
// puedan convertir la pantalla de ayuda en un lanzador genérico.
public interface IHelpLauncherService
{
    Task<HelpLaunchResult> OpenInstalledManualAsync(
        CancellationToken cancellationToken = default);

    Task<HelpLaunchResult> OpenInstallationFolderAsync(
        CancellationToken cancellationToken = default);

    Task<HelpLaunchResult> OpenOfficialProjectAsync(
        CancellationToken cancellationToken = default);

    Task<HelpLaunchResult> OpenOfficialReleasesAsync(
        CancellationToken cancellationToken = default);

    Task<HelpLaunchResult> OpenOfficialLicenseAsync(
        CancellationToken cancellationToken = default);

    Task<HelpLaunchResult> OpenPrivateSupportAsync(
        CancellationToken cancellationToken = default);

    // Página oficial de renovación del certificado de ciudadano de la FNMT.
    Task<HelpLaunchResult> OpenFnmtRenewalAsync(
        CancellationToken cancellationToken = default);

    // Portal oficial de validación externa de certificados.
    Task<HelpLaunchResult> OpenValideAsync(
        CancellationToken cancellationToken = default);

    Task<HelpLaunchResult> OpenOfficialDNIeAsync(
        CancellationToken cancellationToken = default);
}
