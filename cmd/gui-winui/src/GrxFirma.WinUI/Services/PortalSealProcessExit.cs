// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Runtime.InteropServices;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Services;

// El editor de sello del portal es un proceso hijo de vida corta. Al salir
// por la vía normal, Windows descarga Windows.Data.Pdf y su destructor
// estático libera un dispositivo D3D11; con WARP (equipos sin GPU, máquinas
// virtuales, escritorio remoto) eso encola trabajo en el grupo de hilos ya
// cerrado y el proceso termina con c000000d (TppRaiseInvalidParameter). La
// decisión ya está escrita y volcada a disco en result.json, así que el
// proceso termina en cuanto se cierra la ventana, sin esa descarga.
internal static class PortalSealProcessExit
{
    public static void Terminate(PortalSealSession session)
    {
        ArgumentNullException.ThrowIfNull(session);
        TerminateProcess(GetCurrentProcess(), (uint)session.ExitCode);
    }

    [DllImport("kernel32.dll")]
    private static extern nint GetCurrentProcess();

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool TerminateProcess(nint process, uint exitCode);
}
