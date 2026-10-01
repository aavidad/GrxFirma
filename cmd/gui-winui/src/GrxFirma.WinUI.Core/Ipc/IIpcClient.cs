// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Ipc;

public interface IIpcClient : IAsyncDisposable
{
    IpcHello? ServerHello { get; }

    Task<IpcCallResult<TData>> SendAsync<TParameters, TData>(
        string action,
        TParameters parameters,
        CancellationToken cancellationToken = default);
}
