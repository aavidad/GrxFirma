// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Runtime.InteropServices;

namespace GrxFirma.WinUI.Services;

// Al salir por la vía normal, Windows descarga Windows.Data.Pdf y su
// destructor estático (CPdfStatics) libera el dispositivo D3D11 que guarda
// mientras vive el proceso; liberar nuestros PdfDocument no lo suelta. Con
// WARP (equipos sin GPU: máquinas virtuales, VDI, escritorio remoto) esa
// liberación encola trabajo en el grupo de hilos ya cerrado y el proceso
// termina con c000000d (TppRaiseInvalidParameter), evento .NET 1026.
//
// Por eso, una vez hecho el cierre ordenado (decisión del portal escrita, o
// bandeja retirada y canal con el motor cerrado), el proceso termina sin
// descargar DLL. Lo que GrxFirma guarda en disco se escribe en el momento y
// sin búfer pendiente (ajustes a través del motor, registros con
// File.AppendAllText), así que no queda nada por volcar.
internal static class ImmediateProcessExit
{
    public static void Terminate(int exitCode) =>
        TerminateProcess(GetCurrentProcess(), unchecked((uint)exitCode));

    [DllImport("kernel32.dll")]
    private static extern nint GetCurrentProcess();

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool TerminateProcess(nint process, uint exitCode);
}
