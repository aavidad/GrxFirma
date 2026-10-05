// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Security.AccessControl;
using System.Security.Principal;
using Microsoft.Win32;

namespace GrxFirma.WinUI.Services;

internal static class WindowsStartupRegistration
{
    internal const string ValueName = "GrxFirma";
    private const string RunKey = @"Software\Microsoft\Windows\CurrentVersion\Run";
    private const string FixedArgument = "--frontend=winui --start-hidden";

    public static bool IsEnabled
    {
        get
        {
            using var key = Registry.CurrentUser.OpenSubKey(RunKey);
            return string.Equals(
                key?.GetValue(ValueName) as string,
                ExpectedCommand(),
                StringComparison.OrdinalIgnoreCase);
        }
    }

    public static void SetEnabled(bool enabled)
    {
        if (!enabled)
        {
            using var key = Registry.CurrentUser.OpenSubKey(RunKey, true);
            key?.DeleteValue(ValueName, false);
            return;
        }

        var command = ExpectedCommand();
        using var run = Registry.CurrentUser.CreateSubKey(RunKey, true)
            ?? throw new InvalidOperationException(Localizer.Text("winui.inicio.no_se_pudo_abrir_el_inicio_del_usuario"));
        run.SetValue(ValueName, command, RegistryValueKind.String);
    }

    private static string ExpectedCommand()
    {
        var local = Environment.GetFolderPath(
            Environment.SpecialFolder.LocalApplicationData);
        if (string.IsNullOrWhiteSpace(local) ||
            !Path.IsPathFullyQualified(local) ||
            local.StartsWith(@"\\", StringComparison.Ordinal) ||
            local.Contains('%') || local.Contains('"'))
        {
            throw new InvalidOperationException(Localizer.Text("winui.inicio.la_instalacion_por_usuario_no_es_segura"));
        }

        var root = Path.GetFullPath(local);
        var programs = Path.Combine(root, "Programs");
        var app = Path.Combine(programs, "GrxFirma");
        var launcherDir = Path.Combine(app, "DesktopLauncher");
        var launcher = Path.Combine(launcherDir, "grxfirma-gui.exe");
        var marker = Path.Combine(launcherDir, ".grxfirma-install");
        var current = WindowsIdentity.GetCurrent().User
            ?? throw new InvalidOperationException(Localizer.Text("winui.comun.no_se_pudo_identificar_al_usuario"));
        foreach (var path in new[]
                 { root, programs, app, launcherDir, launcher, marker })
        {
            RejectUnsafePath(path, current);
        }
        if (new FileInfo(marker).Length > 64 ||
            File.ReadAllText(marker).Trim() != "GrxFirma:DesktopLauncher")
        {
            throw new InvalidOperationException(Localizer.Text("winui.inicio.el_lanzador_no_esta_instalado"));
        }
        return $"\"{launcher}\" {FixedArgument}";
    }

    private static void RejectUnsafePath(string path, SecurityIdentifier current)
    {
        var attributes = File.GetAttributes(path);
        if ((attributes & FileAttributes.ReparsePoint) != 0)
        {
            throw new InvalidOperationException(Localizer.Text("winui.inicio.la_instalacion_por_usuario_no_es_segura"));
        }
        var isDirectory = (attributes & FileAttributes.Directory) != 0;
        FileSystemSecurity security = isDirectory
            ? new DirectoryInfo(path).GetAccessControl()
            : new FileInfo(path).GetAccessControl();
        var owner = security.GetOwner(typeof(SecurityIdentifier))
            as SecurityIdentifier
            ?? throw new InvalidOperationException(Localizer.Text("winui.inicio.la_instalacion_por_usuario_no_es_segura"));
        if (!owner.Equals(current) &&
            !owner.IsWellKnown(WellKnownSidType.LocalSystemSid) &&
            !owner.IsWellKnown(WellKnownSidType.BuiltinAdministratorsSid))
        {
            throw new InvalidOperationException(Localizer.Text("winui.inicio.la_instalacion_por_usuario_no_es_segura"));
        }
        foreach (FileSystemAccessRule rule in security.GetAccessRules(
                     true, true, typeof(SecurityIdentifier)))
        {
            if (rule.AccessControlType != AccessControlType.Allow ||
                (rule.FileSystemRights &
                 (FileSystemRights.Write | FileSystemRights.Modify |
                  FileSystemRights.FullControl | FileSystemRights.Delete |
                  FileSystemRights.ChangePermissions | FileSystemRights.TakeOwnership)) == 0)
            {
                continue;
            }
            var sid = (SecurityIdentifier)rule.IdentityReference;
            if (!sid.Equals(current) &&
                !sid.IsWellKnown(WellKnownSidType.LocalSystemSid) &&
                !sid.IsWellKnown(WellKnownSidType.BuiltinAdministratorsSid))
            {
                throw new InvalidOperationException(Localizer.Text("winui.inicio.la_instalacion_por_usuario_no_es_segura"));
            }
        }
    }
}
