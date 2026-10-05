// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Operations;

// Espera a que el editor del portal tenga la página del PDF lista para situar
// el sello. Nunca se queda colgada: termina al estar lista, al agotar los
// intentos o al vencer el plazo. Un intento que tarde demasiado no se cancela
// (cancelar una petición ya enviada rompe el canal); solo se deja de esperar.
public static class PortalSealPreviewWait
{
    public static async Task<bool> WaitAsync(
        Func<bool> isReady,
        Func<bool> needsRefresh,
        Func<Task> refresh,
        TimeSpan timeout,
        TimeSpan retryDelay,
        int maximumAttempts,
        TimeProvider timeProvider,
        CancellationToken cancellationToken)
    {
        ArgumentNullException.ThrowIfNull(isReady);
        ArgumentNullException.ThrowIfNull(needsRefresh);
        ArgumentNullException.ThrowIfNull(refresh);
        ArgumentNullException.ThrowIfNull(timeProvider);
        ArgumentOutOfRangeException.ThrowIfLessThanOrEqual(timeout, TimeSpan.Zero);
        ArgumentOutOfRangeException.ThrowIfLessThanOrEqual(retryDelay, TimeSpan.Zero);
        ArgumentOutOfRangeException.ThrowIfLessThan(maximumAttempts, 1);

        var started = timeProvider.GetTimestamp();
        TimeSpan Remaining() => timeout - timeProvider.GetElapsedTime(started);
        var attempts = 0;
        while (true)
        {
            cancellationToken.ThrowIfCancellationRequested();
            if (isReady()) return true;
            var remaining = Remaining();
            if (remaining <= TimeSpan.Zero) return false;
            if (needsRefresh())
            {
                if (attempts == maximumAttempts) return false;
                attempts++;
                try
                {
                    await refresh().WaitAsync(remaining, timeProvider, cancellationToken);
                }
                catch (TimeoutException)
                {
                    return isReady();
                }
                if (isReady()) return true;
                remaining = Remaining();
                if (remaining <= TimeSpan.Zero) return false;
            }
            await Task.Delay(
                retryDelay < remaining ? retryDelay : remaining,
                timeProvider,
                cancellationToken);
        }
    }
}
