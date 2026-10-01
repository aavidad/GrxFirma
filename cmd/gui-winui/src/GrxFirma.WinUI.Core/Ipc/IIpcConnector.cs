// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Ipc;

public interface IIpcConnector
{
    Task<Stream> ConnectAsync(
        IpcEndpoint endpoint,
        IpcClientOptions options,
        CancellationToken cancellationToken);
}
