// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class DesktopRemoteSigningClientTests
{
    private static readonly JsonSerializerOptions WireOptions = new()
    {
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
    };

    [TestMethod]
    public async Task RemoteSigningMethods_SendExactActions()
    {
        var transport = new RecordingIpcClient();
        var client = new DesktopOperationsClient(transport);

        await client.GetRemoteSigningStatusAsync();
        await client.ConfigureRemoteSigningAsync(" https://firma.example ", " cliente-1 ");
        await client.ConnectRemoteSigningAsync();
        await client.SendRemoteSigningOtpAsync(new string('a', 64));
        await client.DisconnectRemoteSigningAsync();

        CollectionAssert.AreEqual(
            new[] { "csc_status", "csc_configure", "csc_connect", "csc_send_otp", "csc_disconnect" },
            transport.Calls.Select(call => call.Action).ToArray());
        var configure = (RemoteSigningConfigureParameters)transport.Calls[1].Parameters;
        Assert.AreEqual("https://firma.example", configure.ServiceUrl);
        Assert.AreEqual("cliente-1", configure.ClientId);
        using var empty = JsonDocument.Parse(JsonSerializer.Serialize(new RemoteSigningConnectParameters(), WireOptions));
        Assert.AreEqual(0, empty.RootElement.EnumerateObject().Count());
    }

    [TestMethod]
    public void Configure_RejectsWhatTheEngineWouldReject()
    {
        foreach (var url in new[] { "", "http://firma.example", "https://u@firma.example", "https://firma.example/?a=1", "https://firma.example\n" })
        {
            Assert.ThrowsExactly<ArgumentException>(() => RemoteSigningInput.Normalize(url, "cliente"));
        }
        foreach (var id in new[] { "", "con espacio", "ñ", new string('x', 257) })
        {
            Assert.ThrowsExactly<ArgumentException>(() => RemoteSigningInput.Normalize("https://firma.example", id));
        }
    }

    [TestMethod]
    public async Task SendOtp_RejectsCertificateIdsThatAreNotFingerprints()
    {
        var client = new DesktopOperationsClient(new RecordingIpcClient());
        await Assert.ThrowsExactlyAsync<ArgumentException>(() => client.SendRemoteSigningOtpAsync("../x"));
        await Assert.ThrowsExactlyAsync<ArgumentException>(() => client.SendRemoteSigningOtpAsync("ABCDEF"));
    }

    [TestMethod]
    public async Task Sign_SendsSecretsAsBase64AndErasesThem()
    {
        var transport = new RecordingIpcClient();
        var client = new DesktopOperationsClient(transport);
        var pin = Encoding.UTF8.GetBytes("1234");
        var otp = Encoding.UTF8.GetBytes("567890");
        string? wire = null;
        transport.OnSend = parameters => wire = JsonSerializer.Serialize(parameters, parameters.GetType(), WireOptions);

        await client.SignAsync(new SignParameters
        {
            InputPath = @"C:\docs\a.pdf",
            CertificateId = new string('b', 64),
            RemotePin = pin,
            RemoteOtp = otp,
        });

        Assert.IsNotNull(wire);
        using var document = JsonDocument.Parse(wire);
        Assert.AreEqual(Convert.ToBase64String(Encoding.UTF8.GetBytes("1234")), document.RootElement.GetProperty("remotePin").GetString());
        Assert.AreEqual(Convert.ToBase64String(Encoding.UTF8.GetBytes("567890")), document.RootElement.GetProperty("remoteOtp").GetString());
        Assert.IsTrue(pin.All(b => b == 0));
        Assert.IsTrue(otp.All(b => b == 0));
    }

    [TestMethod]
    public async Task Sign_InvalidSecretIsErasedAndNotSent()
    {
        var transport = new RecordingIpcClient();
        var client = new DesktopOperationsClient(transport);
        var pin = Encoding.UTF8.GetBytes("12\n34");

        await Assert.ThrowsExactlyAsync<ArgumentException>(() => client.SignBatchAsync(new BatchSignParameters
        {
            OutputDirectory = @"C:\salida",
            RemotePin = pin,
        }));

        Assert.AreEqual(0, transport.Calls.Count);
        Assert.IsTrue(pin.All(b => b == 0));
    }

    [TestMethod]
    public void Results_AreSanitizedAndNeverCarryTokens()
    {
        var status = JsonSerializer.Deserialize<RemoteSigningStatus>(
            """{"allowed":true,"serviceUrl":"https://firma.example","clientId":"c","discovered":true,"connected":false,"serviceHost":"firma.example\u001b[31m","oauthHost":"auth.example","serviceName":"Prestador","access_token":"x"}""");
        Assert.IsNotNull(status);
        Assert.IsTrue(status.Allowed);
        Assert.IsFalse(status.ServiceHost.Any(char.IsControl));
        Assert.AreEqual("auth.example", status.OAuthHost);
        Assert.IsFalse(typeof(RemoteSigningStatus).GetProperties().Any(p => p.Name.Contains("Token", StringComparison.OrdinalIgnoreCase)));

        var connection = JsonSerializer.Deserialize<RemoteSigningConnection>(
            """{"credentials":[{"certificateId":"ab","subject":"Firmante","issuer":"CA","pin":true,"otp":true,"otpOnline":true}],"omitted":1}""");
        Assert.IsNotNull(connection);
        Assert.AreEqual(1, connection.Credentials.Count);
        Assert.IsTrue(connection.Credentials[0].OtpOnline);
        Assert.AreEqual(1, connection.Omitted);
    }

    [TestMethod]
    public void MessageKey_MapsOnlyRemoteSigningCodes()
    {
        Assert.AreEqual("csc.error.otp_lote", RemoteSigningInput.MessageKey("csc_otp_lote"));
        Assert.IsNull(RemoteSigningInput.MessageKey("sign_failed"));
        Assert.IsNull(RemoteSigningInput.MessageKey("csc_../x"));
        Assert.IsNull(RemoteSigningInput.MessageKey(null));
    }

    private sealed class RecordingIpcClient : IIpcClient
    {
        public IpcHello? ServerHello => null;
        public List<RecordedCall> Calls { get; } = [];
        public Action<object>? OnSend { get; set; }

        public Task<IpcCallResult<TData>> SendAsync<TParameters, TData>(
            string action,
            TParameters parameters,
            CancellationToken cancellationToken = default)
        {
            Calls.Add(new(action, parameters!));
            OnSend?.Invoke(parameters!);
            return Task.FromResult(new IpcCallResult<TData>
            {
                Protocol = DesktopIpcProtocol.Name,
                RequestId = "request-remote-1",
                TraceId = "trace-remote-1",
                Action = action,
                IsSuccess = true,
                Outcome = "success",
                Phase = "operation",
            });
        }

        public ValueTask DisposeAsync() => ValueTask.CompletedTask;
    }

    private sealed record RecordedCall(string Action, object Parameters);
}

[TestClass]
public sealed class RemoteSigningSecretsTests
{
    [TestMethod]
    public void Dispose_ErasesBothSecretsAndEmptyMeansNotGiven()
    {
        var pin = new byte[] { 1, 2, 3 };
        var otp = new byte[] { 4, 5 };
        var secrets = new RemoteSigningSecrets(pin, otp);
        secrets.Dispose();
        Assert.IsTrue(pin.All(b => b == 0));
        Assert.IsTrue(otp.All(b => b == 0));
        using var empty = new RemoteSigningSecrets([], null);
        Assert.IsNull(empty.Pin);
        Assert.IsNull(empty.Otp);
    }
}
