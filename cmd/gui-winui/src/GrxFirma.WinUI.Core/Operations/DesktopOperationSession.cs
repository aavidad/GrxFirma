// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Diagnostics.CodeAnalysis;
using GrxFirma.WinUI.Core.Ipc;

namespace GrxFirma.WinUI.Core.Operations;

/// <summary>
/// Publica, sin asumir su ciclo de vida, el cliente de operaciones que ya ha
/// superado la negociación IPC. La aplicación sigue siendo la única
/// propietaria del transporte y la responsable de cerrarlo.
/// </summary>
public sealed class DesktopOperationSession
{
    private const int MaximumCertificateIdCharacters = 1024;

    private readonly object _gate = new();
    private IIpcClient? _transport;
    private DesktopOperationsClient? _operations;
    private IReadOnlySet<string> _advertisedActions =
        new HashSet<string>(StringComparer.Ordinal);
    private readonly HashSet<string> _temporaryCertificateIds =
        new(StringComparer.Ordinal);

    public event EventHandler? AvailabilityChanged;

    public bool IsConnected
    {
        get
        {
            lock (_gate)
            {
                return _operations is not null;
            }
        }
    }

    public bool Supports(string action)
    {
        if (string.IsNullOrWhiteSpace(action))
        {
            return false;
        }

        lock (_gate)
        {
            return _operations is not null &&
                _advertisedActions.Contains(action);
        }
    }

    public bool TryGetOperations(
        string requiredAction,
        [NotNullWhen(true)] out DesktopOperationsClient? operations)
    {
        lock (_gate)
        {
            if (_operations is null ||
                !_advertisedActions.Contains(requiredAction))
            {
                operations = null;
                return false;
            }

            operations = _operations;
            return true;
        }
    }

    /// <summary>
    /// Conserva únicamente durante la vida de esta sesión la identidad que el
    /// motor confirmó para una credencial temporal. Así, las páginas creadas
    /// tras navegar comparten el mismo inventario sin persistirlo en disco.
    /// </summary>
    public void TrackTemporaryCertificate(string certificateId)
    {
        ValidateCertificateId(certificateId);

        lock (_gate)
        {
            _temporaryCertificateIds.Add(certificateId);
        }
    }

    public bool IsTemporaryCertificateTracked(string certificateId)
    {
        if (string.IsNullOrWhiteSpace(certificateId))
        {
            return false;
        }

        lock (_gate)
        {
            return _temporaryCertificateIds.Contains(certificateId);
        }
    }

    public bool UntrackTemporaryCertificate(string certificateId)
    {
        if (string.IsNullOrWhiteSpace(certificateId))
        {
            return false;
        }

        lock (_gate)
        {
            return _temporaryCertificateIds.Remove(certificateId);
        }
    }

    public void ClearTrackedTemporaryCertificates()
    {
        lock (_gate)
        {
            _temporaryCertificateIds.Clear();
        }
    }

    public void Attach(IIpcClient transport)
    {
        ArgumentNullException.ThrowIfNull(transport);

        var hello = transport.ServerHello ??
            throw new InvalidOperationException(
                "El transporte debe completar hello antes de publicarse.");
        var actions = new HashSet<string>(
            hello.Actions ?? [],
            StringComparer.Ordinal);

        lock (_gate)
        {
            if (_transport is not null &&
                !ReferenceEquals(_transport, transport))
            {
                throw new InvalidOperationException(
                    "Ya existe otro transporte IPC publicado.");
            }

            if (ReferenceEquals(_transport, transport))
            {
                return;
            }

            _transport = transport;
            _operations = new DesktopOperationsClient(transport);
            _advertisedActions = actions;
        }

        NotifyAvailabilityChanged();
    }

    public void Detach(IIpcClient transport)
    {
        ArgumentNullException.ThrowIfNull(transport);

        lock (_gate)
        {
            if (!ReferenceEquals(_transport, transport))
            {
                return;
            }

            _transport = null;
            _operations = null;
            _advertisedActions =
                new HashSet<string>(StringComparer.Ordinal);
            _temporaryCertificateIds.Clear();
        }

        NotifyAvailabilityChanged();
    }

    private static void ValidateCertificateId(string certificateId)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(certificateId);
        if (certificateId.Length > MaximumCertificateIdCharacters)
        {
            throw new ArgumentException(
                "El identificador del certificado supera el límite permitido.",
                nameof(certificateId));
        }
    }

    private void NotifyAvailabilityChanged()
    {
        var handlers = AvailabilityChanged;
        if (handlers is null)
        {
            return;
        }

        foreach (EventHandler handler in handlers.GetInvocationList())
        {
            try
            {
                handler(this, EventArgs.Empty);
            }
            catch
            {
                // Un consumidor visual defectuoso no puede despublicar ni
                // invalidar un canal IPC que ya ha sido autenticado.
            }
        }
    }
}
