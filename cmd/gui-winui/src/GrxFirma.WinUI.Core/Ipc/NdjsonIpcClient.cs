// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;
using System.Text.Json.Serialization;
using GrxFirma.WinUI.Core.Diagnostics;

using GrxFirma.WinUI.Core.Localization;

namespace GrxFirma.WinUI.Core.Ipc;

public sealed class NdjsonIpcClient : IIpcClient
{
    private const int TransportActive = 0;
    private const int TransportInvalid = 1;
    private const int TransportDisposed = 2;

    private static readonly JsonSerializerOptions SerializerOptions = new()
    {
        PropertyNameCaseInsensitive = false,
        MaxDepth = 32,
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
        UnmappedMemberHandling = JsonUnmappedMemberHandling.Skip,
    };

    private readonly Stream _stream;
    private readonly LimitedNdjsonReader _reader;
    private readonly IpcClientOptions _options;
    private readonly IIpcEventSink _eventSink;
    private readonly SemaphoreSlim _requestLock = new(1, 1);
    private readonly SemaphoreSlim _transportCloseLock = new(1, 1);
    private int _effectiveMaximumRequestBytes;
    private int _transportState = TransportActive;
    private bool _streamClosed;

    private NdjsonIpcClient(
        Stream stream,
        IpcClientOptions options,
        IIpcEventSink eventSink)
    {
        _stream = stream;
        _reader = new LimitedNdjsonReader(stream);
        _options = options;
        _eventSink = eventSink;
        _effectiveMaximumRequestBytes = options.MaximumRequestBytes;
    }

    public IpcHello? ServerHello { get; private set; }

    public static async Task<NdjsonIpcClient> ConnectAsync(
        IIpcConnector connector,
        IpcEndpoint endpoint,
        IpcClientOptions? options = null,
        IIpcEventSink? eventSink = null,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(connector);
        ArgumentNullException.ThrowIfNull(endpoint);

        options ??= new IpcClientOptions();
        options.Validate();
        eventSink ??= NullIpcEventSink.Instance;
        WriteEvent(eventSink, new(
            DateTimeOffset.UtcNow,
            IpcClientEventKind.Connecting,
            string.Empty,
            string.Empty,
            string.Empty));

        var stream = await connector
            .ConnectAsync(endpoint, options, cancellationToken)
            .ConfigureAwait(false);
        var client = new NdjsonIpcClient(stream, options, eventSink);

        try
        {
            var helloResult = await client
                .SendAsync<EmptyIpcParameters, IpcHello>(
                    DesktopIpcProtocol.HelloAction,
                    new EmptyIpcParameters(),
                    cancellationToken)
                .ConfigureAwait(false);

            if (!helloResult.IsSuccess ||
                helloResult.Outcome != "success" ||
                helloResult.Data is null ||
                !string.Equals(
                    helloResult.Data.Protocol,
                    DesktopIpcProtocol.Name,
                    StringComparison.Ordinal) ||
                !string.Equals(
                    helloResult.Data.Version,
                    DesktopIpcProtocol.Version,
                    StringComparison.Ordinal) ||
                helloResult.Data.Limits is null ||
                helloResult.Data.Limits.MaxRequestBytes is <= 0
                    or > IpcClientOptions.AbsoluteMaximumFrameBytes ||
                helloResult.Data.Actions is null ||
                !helloResult.Data.Actions.Contains(
                    DesktopIpcProtocol.HelloAction,
                    StringComparer.Ordinal) ||
                helloResult.Data.Capabilities is null ||
                DesktopIpcProtocol.RequiredCapabilities.Except(
                    helloResult.Data.Capabilities,
                    StringComparer.Ordinal).Any())
            {
                throw IpcClientException.ProtocolVersion();
            }

            client.ServerHello = helloResult.Data;
            client._effectiveMaximumRequestBytes = Math.Min(
                options.MaximumRequestBytes,
                helloResult.Data.Limits.MaxRequestBytes);
            WriteEvent(eventSink, new(
                DateTimeOffset.UtcNow,
                IpcClientEventKind.Connected,
                DesktopIpcProtocol.HelloAction,
                helloResult.RequestId,
                helloResult.TraceId));
            return client;
        }
        catch
        {
            await client.DisposeAsync().ConfigureAwait(false);
            throw;
        }
    }

    public async Task<IpcCallResult<TData>> SendAsync<TParameters, TData>(
        string action,
        TParameters parameters,
        CancellationToken cancellationToken = default)
    {
        ThrowIfTransportUnavailable();
        ValidateAction(action);
        ArgumentNullException.ThrowIfNull(parameters);

        var requestId = CorrelationId.NewRequestId();
        var traceId = CorrelationId.NewTraceId();
        var request = new IpcWireRequest<TParameters>
        {
            Protocol = DesktopIpcProtocol.Name,
            RequestId = requestId,
            TraceId = traceId,
            Action = action,
            Parameters = parameters,
        };

        byte[] frame;
        byte[]? serializedRequest = null;
        try
        {
            serializedRequest =
                JsonSerializer.SerializeToUtf8Bytes(request, SerializerOptions);
            if (serializedRequest.Length + 1 > _effectiveMaximumRequestBytes)
            {
                throw IpcClientException.FrameTooLarge();
            }

            frame = GC.AllocateUninitializedArray<byte>(serializedRequest.Length + 1);
            serializedRequest.CopyTo(frame, 0);
            frame[^1] = (byte)'\n';
        }
        catch (IpcClientException)
        {
            throw;
        }
        catch
        {
            throw IpcClientException.Protocol();
        }
        finally
        {
            if (serializedRequest is not null)
            {
                Array.Clear(serializedRequest);
            }
        }

        try
        {
            await _requestLock.WaitAsync(cancellationToken).ConfigureAwait(false);
        }
        catch
        {
            Array.Clear(frame);
            throw;
        }
        var transportTouched = false;
        var requestWritten = false;
        try
        {
            ThrowIfTransportUnavailable();
            using var operationTimeout = new CancellationTokenSource(
                _options.OperationTimeout);
            using var linkedCancellation = CancellationTokenSource.CreateLinkedTokenSource(
                cancellationToken,
                operationTimeout.Token);

            WriteEvent(_eventSink, new(
                DateTimeOffset.UtcNow,
                IpcClientEventKind.RequestStarted,
                action,
                requestId,
                traceId));

            // Desde este punto una cancelación o error puede dejar bytes de la
            // petición/respuesta pendientes. El stream no se reutiliza nunca.
            transportTouched = true;
            await _stream
                .WriteAsync(frame.AsMemory(), linkedCancellation.Token)
                .ConfigureAwait(false);
            await _stream.FlushAsync(linkedCancellation.Token).ConfigureAwait(false);
            requestWritten = true;

            var line = await _reader
                .ReadLineAsync(_options.MaximumResponseBytes, linkedCancellation.Token)
                .ConfigureAwait(false);
            IpcWireResponse<TData>? response;
            try
            {
                response = JsonSerializer.Deserialize<IpcWireResponse<TData>>(
                    line,
                    SerializerOptions);
            }
            catch (JsonException)
            {
                throw IpcClientException.Protocol();
            }

            if (response is null ||
                !string.Equals(response.Protocol, DesktopIpcProtocol.Name, StringComparison.Ordinal) ||
                !string.Equals(response.RequestId, requestId, StringComparison.Ordinal) ||
                !string.Equals(response.TraceId, traceId, StringComparison.Ordinal) ||
                !string.Equals(response.Action, action, StringComparison.Ordinal) ||
                response.Ok is null ||
                response.Retryable is null)
            {
                throw IpcClientException.Protocol();
            }

            var outcome = NormalizeOutcome(response.Outcome);
            var errorCode = NormalizeCode(response.ErrorCode);
            var phase = NormalizeCode(response.Phase);
            if (outcome is null ||
                phase is not ("admission" or "protocol" or "operation") ||
                (!response.Ok.Value && outcome != "failure") ||
                (response.Ok.Value && outcome == "failure") ||
                (!response.Ok.Value && errorCode is null) ||
                (outcome == "success" && errorCode is not null) ||
                (outcome == "partial" &&
                    (!response.Ok.Value || errorCode is null)))
            {
                throw IpcClientException.Protocol();
            }

            var result = new IpcCallResult<TData>
            {
                Protocol = DesktopIpcProtocol.Name,
                RequestId = requestId,
                TraceId = traceId,
                Action = action,
                IsSuccess = response.Ok.Value,
                Outcome = outcome,
                ErrorCode = errorCode,
                Phase = phase,
                Retryable = response.Retryable.Value,
                Data = response.Data,
                Diagnostic = SanitizeDiagnostic(response.Diagnostic),
            };

            WriteEvent(_eventSink, new(
                DateTimeOffset.UtcNow,
                result.IsSuccess
                    ? IpcClientEventKind.RequestCompleted
                    : IpcClientEventKind.RequestFailed,
                action,
                requestId,
                traceId,
                result.ErrorCode));
            return result;
        }
        catch (IpcClientException exception)
        {
            var reportedException = requestWritten &&
                exception.Code == "connection_closed"
                    ? IpcClientException.ClosedAfterRequest()
                    : exception;
            if (transportTouched)
            {
                await InvalidateTransportAsync().ConfigureAwait(false);
            }
            WriteEvent(_eventSink, new(
                DateTimeOffset.UtcNow,
                IpcClientEventKind.RequestFailed,
                action,
                requestId,
                traceId,
                reportedException.Code));
            if (!ReferenceEquals(reportedException, exception))
            {
                throw reportedException;
            }
            throw;
        }
        catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
        {
            if (transportTouched)
            {
                await InvalidateTransportAsync().ConfigureAwait(false);
            }
            throw;
        }
        catch (OperationCanceledException)
        {
            if (transportTouched)
            {
                await InvalidateTransportAsync().ConfigureAwait(false);
            }
            var exception = IpcClientException.Connection();
            WriteEvent(_eventSink, new(
                DateTimeOffset.UtcNow,
                IpcClientEventKind.RequestFailed,
                action,
                requestId,
                traceId,
                exception.Code));
            throw exception;
        }
        catch
        {
            if (transportTouched)
            {
                await InvalidateTransportAsync().ConfigureAwait(false);
            }
            var exception = requestWritten
                ? IpcClientException.ClosedAfterRequest()
                : IpcClientException.Closed();
            WriteEvent(_eventSink, new(
                DateTimeOffset.UtcNow,
                IpcClientEventKind.RequestFailed,
                action,
                requestId,
                traceId,
                exception.Code));
            throw exception;
        }
        finally
        {
            Array.Clear(frame);
            _requestLock.Release();
        }
    }

    public async ValueTask DisposeAsync()
    {
        if (Interlocked.Exchange(
                ref _transportState,
                TransportDisposed) == TransportDisposed)
        {
            return;
        }

        await CloseTransportOnceAsync().ConfigureAwait(false);
    }

    private void ThrowIfTransportUnavailable()
    {
        switch (Volatile.Read(ref _transportState))
        {
            case TransportActive:
                return;
            case TransportDisposed:
                throw new ObjectDisposedException(nameof(NdjsonIpcClient));
            default:
                throw IpcClientException.Closed();
        }
    }

    private async ValueTask InvalidateTransportAsync()
    {
        if (Interlocked.CompareExchange(
                ref _transportState,
                TransportInvalid,
                TransportActive) != TransportActive)
        {
            return;
        }

        try
        {
            await CloseTransportOnceAsync().ConfigureAwait(false);
        }
        catch
        {
            // El fallo que causó la invalidación conserva la precedencia.
        }
    }

    private async ValueTask CloseTransportOnceAsync()
    {
        await _transportCloseLock.WaitAsync().ConfigureAwait(false);
        try
        {
            if (_streamClosed)
            {
                return;
            }

            _streamClosed = true;
            WriteEvent(_eventSink, new(
                DateTimeOffset.UtcNow,
                IpcClientEventKind.Disconnected,
                string.Empty,
                string.Empty,
                string.Empty));
            await _stream.DisposeAsync().ConfigureAwait(false);
        }
        finally
        {
            _transportCloseLock.Release();
        }
    }

    private static void ValidateAction(string action)
    {
        if (string.IsNullOrWhiteSpace(action) ||
            action.Length > 64 ||
            action[0] is < 'a' or > 'z' ||
            action.Any(character =>
                character is not ('_' or >= 'a' and <= 'z' or >= '0' and <= '9')))
        {
            throw IpcClientException.Configuration();
        }
    }

    private static void WriteEvent(
        IIpcEventSink sink,
        IpcClientEvent clientEvent)
    {
        try
        {
            sink.Write(clientEvent);
        }
        catch
        {
            // La telemetría opcional nunca puede interrumpir ni alterar IPC.
        }
    }

    private static string? NormalizeOutcome(string? outcome) =>
        outcome switch
        {
            "success" => "success",
            "failure" => "failure",
            "partial" => "partial",
            _ => null,
        };

    private static string? NormalizeCode(string? value)
    {
        if (string.IsNullOrWhiteSpace(value))
        {
            return null;
        }

        var normalized = value.Trim().ToLowerInvariant();
        return normalized.Length <= 64 &&
               normalized.All(character =>
                   character is '_' or '-' or >= 'a' and <= 'z' or >= '0' and <= '9')
            ? normalized
            : null;
    }

    private static OperationDiagnostic? SanitizeDiagnostic(OperationDiagnostic? diagnostic)
    {
        if (diagnostic is null)
        {
            return null;
        }

        var steps = (diagnostic.Steps ?? [])
            .Take(64)
            .Select(step => step with
            {
                Code = SafeIpcText.Clean(step.Code, 64, "unknown"),
                Label = SafeIpcText.Clean(step.Label, 160, "Paso observado"),
                Owner = SafeIpcText.Clean(step.Owner, 64, "unknown"),
                UserMessage = SafeIpcText.Clean(step.UserMessage, 512, string.Empty),
                SuggestedAction = SafeIpcText.Clean(
                    step.SuggestedAction,
                    512,
                    string.Empty),
                EvidenceRef = SafeIpcText.Clean(step.EvidenceRef, 160, string.Empty),
            })
            .ToArray();

        return diagnostic with
        {
            Category = SafeIpcText.Clean(diagnostic.Category, 64, "unknown"),
            FailureCode = SafeIpcText.Clean(diagnostic.FailureCode, 64, string.Empty),
            UserMessage = SafeIpcText.Clean(
                diagnostic.UserMessage,
                512,
                "La operación no se pudo completar."),
            ExpertMessage = SafeIpcText.Clean(
                diagnostic.ExpertMessage,
                4096,
                CatalogLocalizer.Shared.TranslateVisibleText("No hay detalle técnico disponible.")),
            LikelyOwner = SafeIpcText.Clean(
                diagnostic.LikelyOwner,
                64,
                "unknown"),
            ResponsibilityMessage = SafeIpcText.Clean(
                diagnostic.ResponsibilityMessage,
                512,
                CatalogLocalizer.Shared.TranslateVisibleText("No se ha podido determinar el responsable probable.")),
            SuggestedAction = SafeIpcText.Clean(
                diagnostic.SuggestedAction,
                512,
                CatalogLocalizer.Shared.TranslateVisibleText("Vuelva a intentarlo o abra una incidencia si el problema continúa.")),
            Steps = steps,
        };
    }
}
