// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Operations;

// Un paso del cierre de la aplicación que debe terminar antes de que el
// proceso salga (por ejemplo, cerrar el canal con el motor). Se ejecuta
// fuera del hilo de la interfaz, para que una continuación que espere a ese
// hilo no bloquee la salida, y con un plazo, para que un motor que no
// responde no deje la aplicación colgada al salir.
public static class BoundedShutdownStep
{
    // Devuelve true si el paso terminó (bien o con error) dentro del plazo.
    public static bool Run(Func<Task> step, TimeSpan timeout)
    {
        ArgumentNullException.ThrowIfNull(step);
        ArgumentOutOfRangeException.ThrowIfLessThanOrEqual(timeout, TimeSpan.Zero);
        var task = Task.Run(step);
        try
        {
            return task.Wait(timeout);
        }
        catch (AggregateException)
        {
            // Un fallo al cerrar no impide salir: el paso ya terminó.
            return true;
        }
    }
}
