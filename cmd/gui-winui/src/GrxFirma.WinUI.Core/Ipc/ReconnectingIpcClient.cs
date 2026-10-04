// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Ipc;

/// <summary>
/// Mantiene viva la conexión con el motor local. NdjsonIpcClient invalida su
/// canal para siempre tras una cancelación, un tiempo agotado o un error a
/// mitad de petición (el canal puede quedar con bytes pendientes). Sin este
/// envoltorio, la aplicación se quedaba sin motor hasta reiniciarla.
///
/// Antes de cada petición, si el canal anterior quedó inservible, abre uno
/// nuevo con el saludo de protocolo completo. Nunca repite una petición que ya
/// se envió: una firma interrumpida no se reintenta sola.
/// </summary>
public sealed class ReconnectingIpcClient : IIpcClient
{
    private readonly Func<CancellationToken, Task<NdjsonIpcClient>> _connect;
    private readonly SemaphoreSlim _reconnectLock = new(1, 1);
    private NdjsonIpcClient _current;
    private int _disposed;

    public ReconnectingIpcClient(
        NdjsonIpcClient initial,
        Func<CancellationToken, Task<NdjsonIpcClient>> connect)
    {
        _current = initial ?? throw new ArgumentNullException(nameof(initial));
        _connect = connect ?? throw new ArgumentNullException(nameof(connect));
    }

    public IpcHello? ServerHello => Volatile.Read(ref _current).ServerHello;

    /// <summary>Número de reconexiones realizadas; útil para diagnóstico y pruebas.</summary>
    public int Reconnections { get; private set; }

    public async Task<IpcCallResult<TData>> SendAsync<TParameters, TData>(
        string action,
        TParameters parameters,
        CancellationToken cancellationToken = default)
    {
        var client = await EnsureUsableAsync(cancellationToken).ConfigureAwait(false);
        return await client
            .SendAsync<TParameters, TData>(action, parameters, cancellationToken)
            .ConfigureAwait(false);
    }

    private async Task<NdjsonIpcClient> EnsureUsableAsync(CancellationToken cancellationToken)
    {
        ObjectDisposedException.ThrowIf(Volatile.Read(ref _disposed) != 0, this);
        var client = Volatile.Read(ref _current);
        if (client.IsTransportUsable)
        {
            return client;
        }

        await _reconnectLock.WaitAsync(cancellationToken).ConfigureAwait(false);
        try
        {
            ObjectDisposedException.ThrowIf(Volatile.Read(ref _disposed) != 0, this);
            client = Volatile.Read(ref _current);
            if (client.IsTransportUsable)
            {
                return client;
            }

            var fresh = await _connect(cancellationToken).ConfigureAwait(false);
            var stale = Interlocked.Exchange(ref _current, fresh);
            Reconnections++;
            await stale.DisposeAsync().ConfigureAwait(false);
            return fresh;
        }
        finally
        {
            _reconnectLock.Release();
        }
    }

    public async ValueTask DisposeAsync()
    {
        if (Interlocked.Exchange(ref _disposed, 1) != 0)
        {
            return;
        }

        await Volatile.Read(ref _current).DisposeAsync().ConfigureAwait(false);
    }
}
