// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text;
using System.Text.Json;
using GrxFirma.WinUI.Core.Ipc;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class IpcClientTests
{
    [TestMethod]
    public async Task Connect_PerformsVersionedHelloBeforeReturningClient()
    {
        var stream = new ScriptedDuplexStream();
        var connector = new FakeConnector(stream);

        await using var client = await NdjsonIpcClient.ConnectAsync(
            connector,
            new IpcEndpoint(@"\\.\pipe\UNIT_TEST_ONLY", 123));

        Assert.IsNotNull(client.ServerHello);
        Assert.AreEqual(DesktopIpcProtocol.Name, client.ServerHello!.Protocol);
        Assert.AreEqual("hello", stream.Actions.Single());
        Assert.AreEqual(DesktopIpcProtocol.Name, stream.Protocols.Single());
        StringAssert.StartsWith(stream.RequestIds.Single(), "winui-");
        StringAssert.StartsWith(stream.TraceIds.Single(), "trace-");
    }

    [TestMethod]
    public async Task Failure_DoesNotWriteParamsDataOrRawErrorToEventSink()
    {
        const string sentinel = "PRIVATE_VALUE_MUST_NOT_BE_LOGGED";
        var stream = new ScriptedDuplexStream(sentinel);
        var connector = new FakeConnector(stream);
        var sink = new RecordingEventSink();

        await using var client = await NdjsonIpcClient.ConnectAsync(
            connector,
            new IpcEndpoint(@"\\.\pipe\UNIT_TEST_ONLY", 123),
            eventSink: sink);
        var result = await client.SendAsync<object, object>(
            "test_failure",
            new { privateValue = sentinel });

        Assert.IsFalse(result.IsSuccess);
        Assert.AreEqual("safe_failure", result.ErrorCode);
        Assert.AreEqual("No se pudo completar la operación de prueba.", result.SafeUserMessage);

        var serializedEvents = JsonSerializer.Serialize(sink.Events);
        Assert.IsFalse(serializedEvents.Contains(sentinel, StringComparison.Ordinal));
        Assert.IsFalse(serializedEvents.Contains("privateValue", StringComparison.Ordinal));
        Assert.IsFalse(serializedEvents.Contains("\"data\"", StringComparison.Ordinal));
    }

    [TestMethod]
    public async Task ResponseWithDifferentCorrelationId_IsRejected()
    {
        var stream = new ScriptedDuplexStream(mismatchCorrelation: true);
        var connector = new FakeConnector(stream);

        await Assert.ThrowsExactlyAsync<IpcClientException>(
            async () =>
            {
                await using var ignored = await NdjsonIpcClient.ConnectAsync(
                    connector,
                    new IpcEndpoint(@"\\.\pipe\UNIT_TEST_ONLY", 123));
            });
    }

    [TestMethod]
    public async Task ProtocolFailureAfterHello_InvalidatesTransportAndPreventsReuse()
    {
        var stream = new ScriptedDuplexStream(
            mismatchCorrelationAfterHello: true);
        var connector = new FakeConnector(stream);

        await using var client = await NdjsonIpcClient.ConnectAsync(
            connector,
            new IpcEndpoint(@"\\.\pipe\UNIT_TEST_ONLY", 123));

        var exception = await Assert.ThrowsExactlyAsync<IpcClientException>(
            () => client.SendAsync<object, object>("test_success", new { }));
        Assert.AreEqual("protocol_violation", exception.Code);
        Assert.AreEqual("protocol", exception.Phase);
        Assert.AreEqual("app_local", exception.LikelyOwner);
        Assert.IsTrue(stream.IsDisposed);

        var actionCount = stream.Actions.Count;
        var closed = await Assert.ThrowsExactlyAsync<IpcClientException>(
            () => client.SendAsync<object, object>("test_success", new { }));
        Assert.AreEqual("connection_closed", closed.Code);
        Assert.AreEqual("admission", closed.Phase);
        Assert.AreEqual("unknown", closed.LikelyOwner);
        Assert.AreEqual(actionCount, stream.Actions.Count);
    }

    [TestMethod]
    public async Task ClosedAfterRequest_DoesNotClaimAdmissionFailure()
    {
        var stream = new ScriptedDuplexStream(closeAction: "test_close");
        await using var client = await NdjsonIpcClient.ConnectAsync(
            new FakeConnector(stream),
            new IpcEndpoint(@"\\.\pipe\UNIT_TEST_ONLY", 123));

        var exception = await Assert.ThrowsExactlyAsync<IpcClientException>(
            () => client.SendAsync<object, object>("test_close", new { }));

        Assert.AreEqual("connection_closed", exception.Code);
        Assert.AreEqual("protocol", exception.Phase);
        Assert.AreEqual("unknown", exception.LikelyOwner);
    }

    [TestMethod]
    public async Task CancellationAfterWriting_InvalidatesTransportAndPreventsReuse()
    {
        var stream = new ScriptedDuplexStream(blockAction: "test_block");
        var connector = new FakeConnector(stream);

        await using var client = await NdjsonIpcClient.ConnectAsync(
            connector,
            new IpcEndpoint(@"\\.\pipe\UNIT_TEST_ONLY", 123));
        using var cancellation = new CancellationTokenSource(
            TimeSpan.FromMilliseconds(100));

        try
        {
            await client.SendAsync<object, object>(
                "test_block",
                new { },
                cancellation.Token);
            Assert.Fail("La petición bloqueada debía cancelarse.");
        }
        catch (OperationCanceledException)
        {
        }

        Assert.IsTrue(stream.IsDisposed);
        var actionCount = stream.Actions.Count;
        var closed = await Assert.ThrowsExactlyAsync<IpcClientException>(
            () => client.SendAsync<object, object>("test_success", new { }));
        Assert.AreEqual("connection_closed", closed.Code);
        Assert.AreEqual(actionCount, stream.Actions.Count);
    }

    [TestMethod]
    public async Task HelloWithDifferentVersion_IsRejected()
    {
        var stream = new ScriptedDuplexStream(helloVersion: "2.0");
        var connector = new FakeConnector(stream);

        var exception = await Assert.ThrowsExactlyAsync<IpcClientException>(
            async () =>
            {
                await using var ignored = await NdjsonIpcClient.ConnectAsync(
                    connector,
                    new IpcEndpoint(@"\\.\pipe\UNIT_TEST_ONLY", 123));
            });

        Assert.AreEqual("unsupported_protocol", exception.Code);
    }

    [TestMethod]
    public async Task HelloWithoutRequiredCapability_IsRejected()
    {
        var stream = new ScriptedDuplexStream(
            helloCapabilities:
            [
                "correlation",
                "guided-diagnostic-steps",
                "stable-result-metadata",
            ]);
        var connector = new FakeConnector(stream);

        var exception = await Assert.ThrowsExactlyAsync<IpcClientException>(
            async () =>
            {
                await using var ignored = await NdjsonIpcClient.ConnectAsync(
                    connector,
                    new IpcEndpoint(@"\\.\pipe\UNIT_TEST_ONLY", 123));
            });

        Assert.AreEqual("unsupported_protocol", exception.Code);
    }

    [TestMethod]
    public async Task HelloWithInconsistentStableMetadata_IsRejected()
    {
        var stream = new ScriptedDuplexStream(helloOutcome: "failure");
        var connector = new FakeConnector(stream);

        var exception = await Assert.ThrowsExactlyAsync<IpcClientException>(
            async () =>
            {
                await using var ignored = await NdjsonIpcClient.ConnectAsync(
                    connector,
                    new IpcEndpoint(@"\\.\pipe\UNIT_TEST_ONLY", 123));
            });

        Assert.AreEqual("protocol_violation", exception.Code);
    }

    [TestMethod]
    public async Task HelloWithPartialOutcome_IsRejectedEvenWhenMetadataIsCoherent()
    {
        var stream = new ScriptedDuplexStream(
            helloOutcome: "partial",
            helloErrorCode: "partial_failure");
        var connector = new FakeConnector(stream);

        var exception = await Assert.ThrowsExactlyAsync<IpcClientException>(
            async () =>
            {
                await using var ignored = await NdjsonIpcClient.ConnectAsync(
                    connector,
                    new IpcEndpoint(@"\\.\pipe\UNIT_TEST_ONLY", 123));
            });

        Assert.AreEqual("unsupported_protocol", exception.Code);
    }

    [TestMethod]
    public async Task HelloWithUnknownPhase_IsRejected()
    {
        var stream = new ScriptedDuplexStream(helloPhase: "unexpected");
        var connector = new FakeConnector(stream);

        var exception = await Assert.ThrowsExactlyAsync<IpcClientException>(
            async () =>
            {
                await using var ignored = await NdjsonIpcClient.ConnectAsync(
                    connector,
                    new IpcEndpoint(@"\\.\pipe\UNIT_TEST_ONLY", 123));
            });

        Assert.AreEqual("protocol_violation", exception.Code);
    }

    [TestMethod]
    public void CorrelationIds_AreAsciiBoundedAndNotRepeated()
    {
        var ids = Enumerable.Range(0, 128)
            .Select(_ => CorrelationId.NewRequestId())
            .ToArray();

        Assert.AreEqual(ids.Length, ids.Distinct(StringComparer.Ordinal).Count());
        Assert.IsTrue(ids.All(id =>
            id.Length < 128 &&
            id.All(character => character <= 0x7f)));
    }

    [TestMethod]
    public void IpcExceptions_AttributeOnlyObservedLocalPhaseAndOwner()
    {
        var cases = new[]
        {
            (
                Exception: IpcClientException.Configuration(),
                Phase: "admission",
                Owner: "app_local"),
            (
                Exception: IpcClientException.Connection(),
                Phase: "admission",
                Owner: "unknown"),
            (
                Exception: IpcClientException.ServerIdentity(),
                Phase: "admission",
                Owner: "environment"),
            (
                Exception: IpcClientException.Protocol(),
                Phase: "protocol",
                Owner: "app_local"),
            (
                Exception: IpcClientException.ProtocolVersion(),
                Phase: "protocol",
                Owner: "app_local"),
        };

        foreach (var item in cases)
        {
            Assert.AreEqual(item.Phase, item.Exception.Phase);
            Assert.AreEqual(item.Owner, item.Exception.LikelyOwner);
        }
    }

    private sealed class FakeConnector(Stream stream) : IIpcConnector
    {
        public Task<Stream> ConnectAsync(
            IpcEndpoint endpoint,
            IpcClientOptions options,
            CancellationToken cancellationToken) =>
            Task.FromResult(stream);
    }

    private sealed class RecordingEventSink : IIpcEventSink
    {
        public List<IpcClientEvent> Events { get; } = [];

        public void Write(IpcClientEvent clientEvent) => Events.Add(clientEvent);
    }

    private sealed class ScriptedDuplexStream : Stream
    {
        private readonly string? _unsafeFailureText;
        private readonly bool _mismatchCorrelation;
        private readonly string _helloVersion;
        private readonly IReadOnlyList<string> _helloCapabilities;
        private readonly string _helloOutcome;
        private readonly string _helloErrorCode;
        private readonly string _helloPhase;
        private readonly bool _mismatchCorrelationAfterHello;
        private readonly string? _blockAction;
        private readonly string? _closeAction;
        private byte[] _response = [];
        private int _responseOffset;
        private bool _blockReads;

        public ScriptedDuplexStream(
            string? unsafeFailureText = null,
            bool mismatchCorrelation = false,
            string helloVersion = DesktopIpcProtocol.Version,
            IReadOnlyList<string>? helloCapabilities = null,
            string helloOutcome = "success",
            bool mismatchCorrelationAfterHello = false,
            string? blockAction = null,
            string? closeAction = null,
            string helloErrorCode = "",
            string helloPhase = "operation")
        {
            _unsafeFailureText = unsafeFailureText;
            _mismatchCorrelation = mismatchCorrelation;
            _helloVersion = helloVersion;
            _helloCapabilities = helloCapabilities ??
                DesktopIpcProtocol.RequiredCapabilities.ToArray();
            _helloOutcome = helloOutcome;
            _helloErrorCode = helloErrorCode;
            _helloPhase = helloPhase;
            _mismatchCorrelationAfterHello = mismatchCorrelationAfterHello;
            _blockAction = blockAction;
            _closeAction = closeAction;
        }

        public List<string> Actions { get; } = [];
        public List<string> Protocols { get; } = [];
        public List<string> RequestIds { get; } = [];
        public List<string> TraceIds { get; } = [];
        public bool IsDisposed { get; private set; }

        public override bool CanRead => true;
        public override bool CanSeek => false;
        public override bool CanWrite => true;
        public override long Length => throw new NotSupportedException();
        public override long Position
        {
            get => throw new NotSupportedException();
            set => throw new NotSupportedException();
        }

        public override void Flush()
        {
        }

        public override Task FlushAsync(CancellationToken cancellationToken) =>
            Task.CompletedTask;

        public override int Read(byte[] buffer, int offset, int count)
        {
            var available = Math.Min(count, _response.Length - _responseOffset);
            if (available <= 0)
            {
                return 0;
            }

            _response.AsSpan(_responseOffset, available)
                .CopyTo(buffer.AsSpan(offset, available));
            _responseOffset += available;
            return available;
        }

        public override ValueTask<int> ReadAsync(
            Memory<byte> buffer,
            CancellationToken cancellationToken = default)
        {
            if (_blockReads)
            {
                return WaitUntilCanceledAsync(cancellationToken);
            }

            var available = Math.Min(buffer.Length, _response.Length - _responseOffset);
            if (available <= 0)
            {
                return ValueTask.FromResult(0);
            }

            _response.AsMemory(_responseOffset, available).CopyTo(buffer);
            _responseOffset += available;
            return ValueTask.FromResult(available);
        }

        public override void Write(byte[] buffer, int offset, int count) =>
            ProcessRequest(buffer.AsSpan(offset, count));

        public override ValueTask WriteAsync(
            ReadOnlyMemory<byte> buffer,
            CancellationToken cancellationToken = default)
        {
            ProcessRequest(buffer.Span);
            return ValueTask.CompletedTask;
        }

        private void ProcessRequest(ReadOnlySpan<byte> frame)
        {
            var json = Encoding.UTF8.GetString(frame).TrimEnd('\r', '\n');
            using var document = JsonDocument.Parse(json);
            var root = document.RootElement;
            var action = root.GetProperty("action").GetString()!;
            var protocol = root.GetProperty("protocol").GetString()!;
            var requestId = root.GetProperty("requestId").GetString()!;
            var traceId = root.GetProperty("traceId").GetString()!;
            Actions.Add(action);
            Protocols.Add(protocol);
            RequestIds.Add(requestId);
            TraceIds.Add(traceId);

            if (string.Equals(action, _blockAction, StringComparison.Ordinal))
            {
                _response = [];
                _responseOffset = 0;
                _blockReads = true;
                return;
            }

            if (string.Equals(action, _closeAction, StringComparison.Ordinal))
            {
                _response = [];
                _responseOffset = 0;
                return;
            }

            object? data = action == "hello"
                ? new
                {
                    protocol = DesktopIpcProtocol.Name,
                    version = _helloVersion,
                    build = new { version = "test", target = "windows/amd64" },
                    capabilities = _helloCapabilities,
                    actions = new[] { "hello", "test_failure" },
                    limits = new
                    {
                        maxRequestBytes = IpcClientOptions.AbsoluteMaximumFrameBytes,
                        maxConcurrentConnections = 10,
                        defaultTimeoutSeconds = 30,
                        longOperationTimeoutSeconds = 300,
                    },
                    idleTimeoutSeconds = 60,
                }
                : null;
            object response = action == "test_failure"
                ? (object)new
                {
                    protocol,
                    requestId,
                    traceId,
                    ok = false,
                    action,
                    errorCode = "safe_failure",
                    phase = "operation",
                    retryable = false,
                    outcome = "failure",
                    error = _unsafeFailureText,
                    diagnostic = new
                    {
                        category = "app_local",
                        userMessage = "No se pudo completar la operación de prueba.",
                        expertMessage = "Detalle controlado.",
                        likelyOwner = "app_local",
                        responsibilityMessage = "Responsabilidad local.",
                        suggestedAction = "Solicite soporte.",
                        userCanResolveDirectly = false,
                        steps = Array.Empty<object>(),
                    },
                }
                : new
                {
                    protocol,
                    requestId =
                        _mismatchCorrelation ||
                        (_mismatchCorrelationAfterHello && action != "hello")
                            ? "winui-mismatch"
                            : requestId,
                    traceId,
                    ok = true,
                    action,
                    errorCode = action == "hello" ? _helloErrorCode : "",
                    phase = action == "hello" ? _helloPhase : "operation",
                    retryable = false,
                    outcome = _helloOutcome,
                    data,
                };

            _response = Encoding.UTF8.GetBytes(JsonSerializer.Serialize(response) + "\n");
            _responseOffset = 0;
        }

        private static async ValueTask<int> WaitUntilCanceledAsync(
            CancellationToken cancellationToken)
        {
            await Task.Delay(Timeout.InfiniteTimeSpan, cancellationToken);
            return 0;
        }

        protected override void Dispose(bool disposing)
        {
            IsDisposed = true;
            base.Dispose(disposing);
        }

        public override long Seek(long offset, SeekOrigin origin) =>
            throw new NotSupportedException();

        public override void SetLength(long value) =>
            throw new NotSupportedException();
    }
}
