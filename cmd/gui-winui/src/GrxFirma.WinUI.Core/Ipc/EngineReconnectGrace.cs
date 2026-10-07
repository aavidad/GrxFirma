// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Ipc;

/// <summary>
/// Plazo de gracia tras perder el motor local. Al actualizar con la aplicación
/// abierta, el instalador detiene el motor y vuelve a abrir GrxFirma en pocos
/// segundos: mientras dura el plazo se muestra un aviso informativo y solo si
/// vence sin recuperar el canal se muestra el error de conexión.
/// </summary>
public static class EngineReconnectGrace
{
    public static readonly TimeSpan Window = TimeSpan.FromSeconds(30);
    public static readonly TimeSpan RetryInterval = TimeSpan.FromSeconds(2);

    /// <summary>
    /// Repite <paramref name="tryRecover"/> hasta que devuelve true o vence el
    /// plazo. El plazo y el token solo limitan la espera: un intento en curso
    /// nunca se cancela, porque cancelar una petición IPC ya enviada invalida la
    /// conexión. Un intento que falla con excepción cuenta como no recuperado.
    /// </summary>
    public static async Task<bool> WaitForRecoveryAsync(
        Func<Task<bool>> tryRecover,
        TimeProvider clock,
        CancellationToken cancellationToken,
        TimeSpan? window = null,
        TimeSpan? retryInterval = null)
    {
        ArgumentNullException.ThrowIfNull(tryRecover);
        ArgumentNullException.ThrowIfNull(clock);
        var deadline = clock.GetUtcNow() + (window ?? Window);
        var interval = retryInterval ?? RetryInterval;
        while (true)
        {
            var remaining = deadline - clock.GetUtcNow();
            if (remaining <= TimeSpan.Zero) return false;
            var attempt = SafeAttemptAsync(tryRecover);
            var finished = await Task.WhenAny(
                attempt,
                Task.Delay(remaining, clock, cancellationToken)).ConfigureAwait(false);
            cancellationToken.ThrowIfCancellationRequested();
            if (finished != attempt) return false;
            if (await attempt.ConfigureAwait(false)) return true;
            remaining = deadline - clock.GetUtcNow();
            if (remaining <= TimeSpan.Zero) return false;
            await Task.Delay(remaining < interval ? remaining : interval, clock, cancellationToken)
                .ConfigureAwait(false);
        }
    }

    private static async Task<bool> SafeAttemptAsync(Func<Task<bool>> tryRecover)
    {
        try
        {
            return await tryRecover().ConfigureAwait(false);
        }
        catch (Exception)
        {
            return false;
        }
    }
}
