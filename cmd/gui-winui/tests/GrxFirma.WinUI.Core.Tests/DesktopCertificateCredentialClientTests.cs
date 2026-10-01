// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Reflection;
using System.Text.Json;
using System.Text.Json.Serialization;
using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class DesktopCertificateCredentialClientTests
{
    private static readonly JsonSerializerOptions WireOptions = new()
    {
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
    };

    [TestMethod]
    public async Task ModernCredentialMethods_UseExactActionsAndTypes()
    {
        var transport = new RecordingIpcClient();
        var client = new DesktopOperationsClient(transport);
        var credential = new byte[] { 1, 2, 3 };
        var password = new byte[] { 4, 5, 6 };

        await client.GetCertificateImportOptionsAsync();
        await client.ImportCertificateToStoreAsync(new()
        {
            CredentialB64 = credential,
            PasswordB64 = password,
            TargetId = "windows-current-user",
        });
        await client.UseTemporaryCertificateAsync(new()
        {
            CredentialB64 = credential,
            PasswordB64 = password,
        });
        await client.RemoveTemporaryCertificateAsync(new()
        {
            CertificateId = "temporary-1",
        });
        await client.ClearTemporaryCertificatesAsync();

        CollectionAssert.AreEqual(
            new[]
            {
                "certificate_access_options",
                "import_certificate_to_store",
                "use_temporary_certificate",
                "remove_temporary_certificate",
                "clear_temporary_certificates",
            },
            transport.Calls.Select(call => call.Action).ToArray());
        CollectionAssert.AreEqual(
            new[]
            {
                typeof(CertificateAccessOptionsParameters),
                typeof(ImportCertificateToStoreParameters),
                typeof(UseTemporaryCertificateParameters),
                typeof(RemoveTemporaryCertificateParameters),
                typeof(ClearTemporaryCertificatesParameters),
            },
            transport.Calls
                .Select(call => call.ParametersType)
                .ToArray());
        CollectionAssert.AreEqual(
            new[]
            {
                typeof(CertificateAccessOptionsResult),
                typeof(string),
                typeof(TemporaryCertificateResult),
                typeof(string),
                typeof(string),
            },
            transport.Calls.Select(call => call.DataType).ToArray());
    }

    [TestMethod]
    public void CredentialDtos_SerializeBuffersAsBase64WithoutLegacyPassword()
    {
        var persistent = new ImportCertificateToStoreParameters
        {
            CredentialB64 = new byte[] { 0, 1, 2, 3 },
            PasswordB64 = new byte[] { 0xC3, 0xB1 },
            TargetId = "nss:profile",
        };
        using var persistentJson = JsonDocument.Parse(
            JsonSerializer.Serialize(persistent, WireOptions));

        Assert.AreEqual(
            "AAECAw==",
            persistentJson.RootElement
                .GetProperty("credentialB64")
                .GetString());
        Assert.AreEqual(
            "w7E=",
            persistentJson.RootElement
                .GetProperty("passwordB64")
                .GetString());
        Assert.AreEqual(
            "nss:profile",
            persistentJson.RootElement
                .GetProperty("targetId")
                .GetString());
        Assert.IsFalse(
            persistentJson.RootElement.TryGetProperty(
                "password",
                out _));

        var temporary = new UseTemporaryCertificateParameters
        {
            CredentialB64 = new byte[] { 4, 5, 6 },
            PasswordB64 = [],
        };
        using var temporaryJson = JsonDocument.Parse(
            JsonSerializer.Serialize(temporary, WireOptions));
        Assert.AreEqual(
            "BAUG",
            temporaryJson.RootElement
                .GetProperty("credentialB64")
                .GetString());
        Assert.AreEqual(
            string.Empty,
            temporaryJson.RootElement
                .GetProperty("passwordB64")
                .GetString());

        Assert.AreEqual(
            typeof(byte[]),
            typeof(ImportCertificateToStoreParameters)
                .GetProperty(
                    nameof(
                        ImportCertificateToStoreParameters
                            .CredentialB64))!
                .PropertyType);
        Assert.AreEqual(
            typeof(byte[]),
            typeof(ImportCertificateToStoreParameters)
                .GetProperty(
                    nameof(
                        ImportCertificateToStoreParameters
                            .PasswordB64))!
                .PropertyType);
        Assert.IsFalse(
            typeof(ImportCertificateToStoreParameters)
                .GetProperties(BindingFlags.Instance |
                    BindingFlags.Public)
                .Any(property => string.Equals(
                    property.GetCustomAttribute<
                        JsonPropertyNameAttribute>()?.Name,
                    "password",
                    StringComparison.Ordinal)));
    }

    [TestMethod]
    public async Task CredentialMethods_RejectUnsafeSizesBeforeTransport()
    {
        var transport = new RecordingIpcClient();
        var client = new DesktopOperationsClient(transport);

        await Assert.ThrowsExactlyAsync<ArgumentException>(() =>
            client.UseTemporaryCertificateAsync(new()
            {
                CredentialB64 = [],
                PasswordB64 = [],
            }));
        await Assert.ThrowsExactlyAsync<ArgumentException>(() =>
            client.UseTemporaryCertificateAsync(new()
            {
                CredentialB64 = new byte[
                    DesktopOperationsClient.MaximumCredentialBytes + 1],
                PasswordB64 = [],
            }));
        await Assert.ThrowsExactlyAsync<ArgumentException>(() =>
            client.ImportCertificateToStoreAsync(new()
            {
                CredentialB64 = [1],
                PasswordB64 = new byte[
                    DesktopOperationsClient.MaximumPasswordBytes + 1],
                TargetId = "windows-current-user",
            }));
        await Assert.ThrowsExactlyAsync<ArgumentException>(() =>
            client.ImportCertificateToStoreAsync(new()
            {
                CredentialB64 = [1],
                PasswordB64 = [],
                TargetId = " ",
            }));
        await Assert.ThrowsExactlyAsync<ArgumentException>(() =>
            client.RemoveTemporaryCertificateAsync(new()
            {
                CertificateId = string.Empty,
            }));

        Assert.AreEqual(0, transport.Calls.Count);
    }

    [TestMethod]
    public void AccessOptions_AreBoundedAndSanitized()
    {
        var options = JsonSerializer.Deserialize<
            CertificateAccessOptionsResult>(
            JsonSerializer.Serialize(new
            {
                managers = Enumerable.Range(0, 500)
                    .Select(index => new
                    {
                        id = $"manager-{index}\u0000",
                        label = new string('M', 400) + "\u0001",
                    }),
                importTargets = Enumerable.Range(0, 500)
                    .Select(index => new
                    {
                        id = $"target-{index}\u0000",
                        label = new string('T', 400) + "\u0001",
                        browser = "firefox\u0000",
                        recommended = index == 0,
                    }),
                detectedBrowser = "firefox\u0000",
                preferredManager = "manager-0\u0000",
                preferredTarget = "target-0\u0000",
            }));

        Assert.IsNotNull(options);
        Assert.AreEqual(128, options.Managers.Count);
        Assert.AreEqual(128, options.ImportTargets.Count);
        Assert.AreEqual(256, options.ImportTargets[0].Label.Length);
        Assert.IsTrue(options.ImportTargets[0].Recommended);
        Assert.IsFalse(options.ImportTargets.Any(target =>
            target.Id.Any(char.IsControl) ||
            target.Label.Any(char.IsControl) ||
            target.Browser.Any(char.IsControl)));
        Assert.IsFalse(options.DetectedBrowser.Any(char.IsControl));
    }

    private sealed class RecordingIpcClient : IIpcClient
    {
        public IpcHello? ServerHello => null;
        public List<RecordedCall> Calls { get; } = [];

        public Task<IpcCallResult<TData>> SendAsync<TParameters, TData>(
            string action,
            TParameters parameters,
            CancellationToken cancellationToken = default)
        {
            Calls.Add(new(
                action,
                typeof(TParameters),
                typeof(TData),
                parameters!,
                cancellationToken));
            return Task.FromResult(new IpcCallResult<TData>
            {
                Protocol = DesktopIpcProtocol.Name,
                RequestId = "request-certificate-1",
                TraceId = "trace-certificate-1",
                Action = action,
                IsSuccess = true,
                Outcome = "success",
                Phase = "operation",
            });
        }

        public ValueTask DisposeAsync() => ValueTask.CompletedTask;
    }

    private sealed record RecordedCall(
        string Action,
        Type ParametersType,
        Type DataType,
        object Parameters,
        CancellationToken CancellationToken);
}
