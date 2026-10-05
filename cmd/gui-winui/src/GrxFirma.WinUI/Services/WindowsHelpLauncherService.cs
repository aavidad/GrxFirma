// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;
using Windows.Foundation;
using Windows.Storage;
using Windows.System;

namespace GrxFirma.WinUI.Services;

public sealed class WindowsHelpLauncherService : IHelpLauncherService
{
    private const string OfficialProjectUrl =
        "https://github.com/aavidad/GrxFirma";
    private const string OfficialReleasesUrl =
        OfficialProjectUrl + "/releases";
    private const string OfficialLicenseUrl =
        "https://interoperable-europe.ec.europa.eu/collection/eupl/eupl-text-eupl-12";
    private const string FnmtRenewalUrl =
        "https://www.sede.fnmt.gob.es/certificados/persona-fisica/renovar";
    private const string ValideUrl =
        "https://valide.redsara.es/valide/";
    private const string OfficialDNIeUrl =
        "https://www.dnielectronico.es/PortalDNIe/";
    private const string PrivateSupportUrl =
        "mailto:avidad@dipgra.es?subject=Soporte%20privado%20GrxFirma";

    private static readonly HashSet<string> SupportedHelpLanguages =
        new(StringComparer.OrdinalIgnoreCase)
        {
            "ca",
            "de",
            "en",
            "es",
            "eu",
            "fr",
            "gl",
            "it",
            "pt",
            "va",
            "zh",
        };

    private static readonly HashSet<string> AllowedManualExtensions =
        new(StringComparer.OrdinalIgnoreCase)
        {
            ".md",
            ".pdf",
            ".txt",
        };

    public async Task<HelpLaunchResult> OpenInstalledManualAsync(
        CancellationToken cancellationToken = default)
    {
        try
        {
            foreach (var candidate in InstalledManualCandidates())
            {
                cancellationToken.ThrowIfCancellationRequested();
                if (!IsSafeExistingManual(candidate))
                {
                    continue;
                }

                var fileOperation =
                    StorageFile.GetFileFromPathAsync(candidate.Path);
                using var fileCancellation = CancelWith(
                    cancellationToken,
                    fileOperation);
                var file = await fileOperation;
                cancellationToken.ThrowIfCancellationRequested();

                var launchOperation = Launcher.LaunchFileAsync(file);
                using var launchCancellation = CancelWith(
                    cancellationToken,
                    launchOperation);
                if (await launchOperation)
                {
                    return new(
                        true,
                        Localizer.Text("winui.ayuda.se_ha_abierto_la_ayuda_instalada_con_la"));
                }

                return new(
                    false,
                    Localizer.Text("winui.ayuda.windows_no_encontro_una_aplicacion"));
            }

            return new(
                false,
                Localizer.Text("winui.ayuda.no_se_encontro_un_manual_instalado_en"));
        }
        catch (OperationCanceledException)
        {
            throw;
        }
        catch
        {
            return new(
                false,
                Localizer.Text("winui.ayuda.la_ayuda_instalada_no_se_pudo_abrir"));
        }
    }

    public async Task<HelpLaunchResult> OpenInstallationFolderAsync(
        CancellationToken cancellationToken = default)
    {
        try
        {
            cancellationToken.ThrowIfCancellationRequested();
            var installationDirectory = FullDirectory(
                AppContext.BaseDirectory);
            if (!Directory.Exists(installationDirectory) ||
                HasReparsePoint(installationDirectory))
            {
                return new(
                    false,
                    Localizer.Text("winui.ayuda.windows_no_pudo_localizar_de_forma"));
            }

            var folderOperation =
                StorageFolder.GetFolderFromPathAsync(installationDirectory);
            using var folderCancellation = CancelWith(
                cancellationToken,
                folderOperation);
            var folder = await folderOperation;
            cancellationToken.ThrowIfCancellationRequested();

            var launchOperation = Launcher.LaunchFolderAsync(folder);
            using var launchCancellation = CancelWith(
                cancellationToken,
                launchOperation);
            return await launchOperation
                ? new(
                    true,
                    Localizer.Text("winui.ayuda.se_ha_abierto_la_carpeta_de_instalacion"))
                : new(
                    false,
                    Localizer.Text("winui.ayuda.windows_no_pudo_abrir_la_carpeta_de"));
        }
        catch (OperationCanceledException)
        {
            throw;
        }
        catch
        {
            return new(
                false,
                Localizer.Text("winui.ayuda.la_carpeta_de_instalacion_no_se_pudo"));
        }
    }

    public Task<HelpLaunchResult> OpenOfficialProjectAsync(
        CancellationToken cancellationToken = default) =>
        LaunchTrustedUriAsync(
            TrustedOfficialProjectUri(),
            Localizer.Text("winui.ayuda.se_ha_abierto_el_proyecto_oficial_en_el"),
            Localizer.Text("winui.ayuda.windows_no_tiene_configurado_un"),
            cancellationToken);

    public Task<HelpLaunchResult> OpenOfficialReleasesAsync(
        CancellationToken cancellationToken = default) =>
        LaunchTrustedUriAsync(
            TrustedOfficialReleasesUri(),
            Localizer.Text("winui.ayuda.se_ha_abierto_la_pagina_oficial_de"),
            Localizer.Text("winui.ayuda.windows_no_tiene_configurado_un_2"),
            cancellationToken);

    public Task<HelpLaunchResult> OpenOfficialLicenseAsync(
        CancellationToken cancellationToken = default) =>
        LaunchTrustedUriAsync(
            TrustedOfficialLicenseUri(),
            Localizer.Text("winui.ayuda.se_ha_abierto_el_texto_oficial_de_la"),
            Localizer.Text("winui.ayuda.windows_no_tiene_configurado_un_3"),
            cancellationToken);

    public Task<HelpLaunchResult> OpenPrivateSupportAsync(
        CancellationToken cancellationToken = default) =>
        LaunchTrustedUriAsync(
            TrustedPrivateSupportUri(),
            Localizer.Text("winui.ayuda.se_ha_abierto_un_mensaje_nuevo_en_la"),
            Localizer.Text("winui.ayuda.windows_no_tiene_configurada_una"),
            cancellationToken);

    public Task<HelpLaunchResult> OpenFnmtRenewalAsync(
        CancellationToken cancellationToken = default) =>
        LaunchTrustedUriAsync(
            TrustedFnmtRenewalUri(),
            Localizer.Text("winui.ayuda.se_ha_abierto_la_pagina_oficial_de_2"),
            Localizer.Text("winui.ayuda.windows_no_tiene_configurado_un_4"),
            cancellationToken);

    public Task<HelpLaunchResult> OpenValideAsync(
        CancellationToken cancellationToken = default) =>
        LaunchTrustedUriAsync(
            TrustedValideUri(),
            Localizer.Text("winui.ayuda.se_ha_abierto_valide_en_el_navegador"),
            Localizer.Text("winui.ayuda.windows_no_tiene_configurado_un_5"),
            cancellationToken);

    public Task<HelpLaunchResult> OpenOfficialDNIeAsync(
        CancellationToken cancellationToken = default) =>
        LaunchTrustedUriAsync(
            TrustedOfficialDNIeUri(),
            Localizer.Text("winui.ayuda.se_ha_abierto_el_portal_oficial_del_dnie"),
            Localizer.Text("winui.ayuda.windows_no_tiene_configurado_un_6"),
            cancellationToken);

    private static Uri TrustedOfficialDNIeUri()
    {
        var uri = new Uri(OfficialDNIeUrl, UriKind.Absolute);
        if (!string.Equals(uri.Scheme, Uri.UriSchemeHttps,
                StringComparison.OrdinalIgnoreCase) ||
            !string.Equals(uri.Host, "www.dnielectronico.es",
                StringComparison.OrdinalIgnoreCase) ||
            !string.Equals(uri.AbsolutePath, "/PortalDNIe/",
                StringComparison.Ordinal))
        {
            throw new InvalidOperationException(
                Localizer.Text("winui.ayuda.la_direccion_del_dnie_no_es_la_oficial"));
        }
        return uri;
    }

    private static Uri TrustedValideUri()
    {
        var uri = new Uri(ValideUrl, UriKind.Absolute);
        if (!string.Equals(uri.Scheme, Uri.UriSchemeHttps,
                StringComparison.OrdinalIgnoreCase) ||
            !string.Equals(uri.Host, "valide.redsara.es",
                StringComparison.OrdinalIgnoreCase) ||
            !string.Equals(uri.AbsolutePath, "/valide/",
                StringComparison.Ordinal))
        {
            throw new InvalidOperationException(
                Localizer.Text("winui.ayuda.la_direccion_de_valide_no_es_la_oficial"));
        }
        return uri;
    }

    private static Uri TrustedFnmtRenewalUri()
    {
        var uri = new Uri(FnmtRenewalUrl, UriKind.Absolute);
        if (!string.Equals(
                uri.Scheme,
                Uri.UriSchemeHttps,
                StringComparison.OrdinalIgnoreCase) ||
            !string.Equals(
                uri.Host,
                "www.sede.fnmt.gob.es",
                StringComparison.OrdinalIgnoreCase))
        {
            throw new InvalidOperationException(
                Localizer.Text("winui.ayuda.la_direccion_de_renovacion_de_la_fnmt_no"));
        }
        return uri;
    }

    private static async Task<HelpLaunchResult> LaunchTrustedUriAsync(
        Uri trustedUri,
        string successMessage,
        string unavailableMessage,
        CancellationToken cancellationToken)
    {
        try
        {
            cancellationToken.ThrowIfCancellationRequested();
            var operation = Launcher.LaunchUriAsync(trustedUri);
            using var cancellationRegistration = CancelWith(
                cancellationToken,
                operation);
            return await operation
                ? new(true, successMessage)
                : new(false, unavailableMessage);
        }
        catch (OperationCanceledException)
        {
            throw;
        }
        catch
        {
            return new(false, unavailableMessage);
        }
    }

    private static Uri TrustedOfficialProjectUri()
    {
        var uri = new Uri(OfficialProjectUrl, UriKind.Absolute);
        if (!string.Equals(
                uri.Scheme,
                Uri.UriSchemeHttps,
                StringComparison.OrdinalIgnoreCase) ||
            !string.Equals(
                uri.Host,
                "github.com",
                StringComparison.OrdinalIgnoreCase) ||
            !string.Equals(
                uri.AbsolutePath.TrimEnd('/'),
                "/aavidad/GrxFirma",
                StringComparison.Ordinal))
        {
            throw new InvalidOperationException(
                Localizer.Text("winui.ayuda.el_destino_oficial_configurado_no_es"));
        }

        return uri;
    }

    private static Uri TrustedOfficialReleasesUri()
    {
        var uri = new Uri(OfficialReleasesUrl, UriKind.Absolute);
        if (!string.Equals(
                uri.Scheme,
                Uri.UriSchemeHttps,
                StringComparison.OrdinalIgnoreCase) ||
            !string.Equals(
                uri.Host,
                "github.com",
                StringComparison.OrdinalIgnoreCase) ||
            !string.Equals(
                uri.AbsolutePath.TrimEnd('/'),
                "/aavidad/GrxFirma/releases",
                StringComparison.Ordinal))
        {
            throw new InvalidOperationException(
                Localizer.Text("winui.ayuda.el_destino_oficial_de_versiones_no_es"));
        }

        return uri;
    }

    private static Uri TrustedOfficialLicenseUri()
    {
        var uri = new Uri(OfficialLicenseUrl, UriKind.Absolute);
        if (!string.Equals(
                uri.Scheme,
                Uri.UriSchemeHttps,
                StringComparison.OrdinalIgnoreCase) ||
            !string.Equals(
                uri.Host,
                "interoperable-europe.ec.europa.eu",
                StringComparison.OrdinalIgnoreCase) ||
            !string.Equals(
                uri.AbsolutePath.TrimEnd('/'),
                "/collection/eupl/eupl-text-eupl-12",
                StringComparison.Ordinal))
        {
            throw new InvalidOperationException(
                Localizer.Text("winui.ayuda.el_destino_oficial_de_la_licencia_no_es"));
        }

        return uri;
    }

    private static Uri TrustedPrivateSupportUri()
    {
        var uri = new Uri(PrivateSupportUrl, UriKind.Absolute);
        if (!string.Equals(
                uri.Scheme,
                Uri.UriSchemeMailto,
                StringComparison.OrdinalIgnoreCase) ||
            !string.Equals(
                uri.UserInfo,
                "avidad",
                StringComparison.Ordinal) ||
            !string.Equals(
                uri.Host,
                "dipgra.es",
                StringComparison.OrdinalIgnoreCase) ||
            !string.Equals(
                uri.OriginalString,
                PrivateSupportUrl,
                StringComparison.Ordinal))
        {
            throw new InvalidOperationException(
                Localizer.Text("winui.ayuda.el_contacto_privado_configurado_no_es"));
        }

        return uri;
    }

    private static IEnumerable<ManualCandidate>
        InstalledManualCandidates()
    {
        var applicationDirectory = FullDirectory(
            AppContext.BaseDirectory);
        var parentDirectory = Directory.GetParent(
            applicationDirectory)?.FullName;
        var language = CurrentHelpLanguage();

        // La guía de texto forma parte de toda publicación WinUI válida. Se
        // prioriza para que Windows 10 pueda abrir la ayuda con su editor de
        // texto sin depender de una asociación para Markdown o de un PDF.
        yield return UnderRoot(
            applicationDirectory,
            "help",
            "guia-usuario.txt");

        foreach (var root in ExistingCandidateRoots(
                     applicationDirectory,
                     parentDirectory))
        {
            if (language is not null)
            {
                yield return UnderRoot(
                    root,
                    "help",
                    $"ayuda-{language}.pdf");
                yield return UnderRoot(
                    root,
                    "help",
                    language,
                    "ayuda.pdf");
            }

            yield return UnderRoot(root, "help", "ayuda.pdf");
        }

        yield return UnderRoot(
            applicationDirectory,
            "README_DESKTOP_WINUI_WINDOWS.md");

        if (parentDirectory is not null)
        {
            yield return UnderRoot(
                parentDirectory,
                "README_DESKTOP_WINUI_WINDOWS.md");
            yield return UnderRoot(
                parentDirectory,
                "README_WINDOWS_SUITE.md");
        }

        var localApplicationData = Environment.GetFolderPath(
            Environment.SpecialFolder.LocalApplicationData);
        if (!string.IsNullOrWhiteSpace(localApplicationData))
        {
            var suiteDirectory = Path.Combine(
                FullDirectory(localApplicationData),
                "Programs",
                "GrxFirma",
                "Suite");
            yield return UnderRoot(
                suiteDirectory,
                "README_WINDOWS_SUITE.md");
        }
    }

    private static IEnumerable<string> ExistingCandidateRoots(
        string applicationDirectory,
        string? parentDirectory)
    {
        yield return applicationDirectory;
        if (!string.IsNullOrWhiteSpace(parentDirectory))
        {
            yield return FullDirectory(parentDirectory);
        }
    }

    private static ManualCandidate UnderRoot(
        string root,
        params string[] segments)
    {
        var fullRoot = FullDirectory(root);
        var pathParts = new string[segments.Length + 1];
        pathParts[0] = fullRoot;
        Array.Copy(segments, 0, pathParts, 1, segments.Length);
        return new(fullRoot, Path.GetFullPath(Path.Combine(pathParts)));
    }

    private static bool IsSafeExistingManual(
        ManualCandidate candidate)
    {
        if (!AllowedManualExtensions.Contains(
                Path.GetExtension(candidate.Path)) ||
            !IsPathUnderRoot(candidate.Path, candidate.Root) ||
            !File.Exists(candidate.Path))
        {
            return false;
        }

        var relativePath = Path.GetRelativePath(
            candidate.Root,
            candidate.Path);
        var current = candidate.Root;
        if (HasReparsePoint(current))
        {
            return false;
        }

        var segments = relativePath.Split(
            [Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar],
            StringSplitOptions.RemoveEmptyEntries);
        foreach (var segment in segments)
        {
            current = Path.Combine(current, segment);
            if (HasReparsePoint(current))
            {
                return false;
            }
        }

        return true;
    }

    private static bool IsPathUnderRoot(
        string path,
        string root)
    {
        var relativePath = Path.GetRelativePath(
            FullDirectory(root),
            Path.GetFullPath(path));
        return !Path.IsPathRooted(relativePath) &&
               !string.Equals(
                   relativePath,
                   "..",
                   StringComparison.Ordinal) &&
               !relativePath.StartsWith(
                   $"..{Path.DirectorySeparatorChar}",
                   StringComparison.Ordinal);
    }

    private static bool HasReparsePoint(string path)
    {
        try
        {
            return (File.GetAttributes(path) &
                    System.IO.FileAttributes.ReparsePoint) != 0;
        }
        catch
        {
            return true;
        }
    }

    private static string FullDirectory(string path) =>
        Path.TrimEndingDirectorySeparator(Path.GetFullPath(path));

    private static string? CurrentHelpLanguage()
    {
        var language =
            Localizer.Language;
        return SupportedHelpLanguages.Contains(language)
            ? language.ToLowerInvariant()
            : null;
    }

    private static CancellationTokenRegistration CancelWith(
        CancellationToken cancellationToken,
        IAsyncInfo operation) =>
        cancellationToken.Register(
            static state => ((IAsyncInfo)state!).Cancel(),
            operation);

    private sealed record ManualCandidate(
        string Root,
        string Path);
}
