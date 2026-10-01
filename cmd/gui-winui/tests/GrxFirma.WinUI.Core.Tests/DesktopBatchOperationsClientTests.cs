// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;
using System.Text.Json.Serialization;
using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class DesktopBatchOperationsClientTests
{
    private static readonly JsonSerializerOptions WireOptions = new()
    {
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
    };

    [TestMethod]
    public async Task SignBatch_SendsExactActionAndTypedContract()
    {
        var transport = new RecordingIpcClient();
        var client = new DesktopOperationsClient(transport);
        var parameters = NewParameters();

        await client.SignBatchAsync(parameters);

        Assert.AreEqual("sign_batch", transport.Action);
        Assert.AreSame(parameters, transport.Parameters);
        Assert.AreEqual(
            typeof(BatchSignParameters),
            transport.ParametersType);
        Assert.AreEqual(
            typeof(BatchSignResult),
            transport.DataType);
    }

    [TestMethod]
    public void Request_UsesExactGoWireNames()
    {
        using var document = JsonDocument.Parse(
            JsonSerializer.Serialize(NewParameters(), WireOptions));

        var actual = document.RootElement
            .EnumerateObject()
            .Select(property => property.Name)
            .Order(StringComparer.Ordinal)
            .ToArray();
        var expected = new[]
        {
            "action",
            "additionalCertificateIds",
            "allowInvalidPDF",
            "certificateId",
            "certificateIndex",
            "contactInfo",
            "directoryPath",
            "documentOverrides",
            "extraOptions",
            "format",
            "inputPaths",
            "location",
            "outputDir",
            "overwrite",
            "qrContent",
            "reason",
            "strictCompat",
            "visibleSeal",
        }.Order(StringComparer.Ordinal).ToArray();

        CollectionAssert.AreEqual(expected, actual);
        Assert.AreEqual(
            @"C:\salida",
            document.RootElement
                .GetProperty("outputDir")
                .GetString());
    }

    [TestMethod]
    public void Result_PreservesPartialItemsAndBoundsUntrustedList()
    {
        var payload = JsonSerializer.Serialize(new
        {
            results = Enumerable.Range(0, 200)
                .Select(index => new
                {
                    inputPath = $@"C:\entrada\{index}.pdf",
                    outputPath = $@"C:\salida\{index}_firmado.pdf",
                    format = "pades",
                    ok = index != 1,
                    error = index == 1 ? "firma rechazada" : "",
                }),
            okCount = 199,
            failCount = 1,
        });

        var result = JsonSerializer.Deserialize<BatchSignResult>(payload);

        Assert.IsNotNull(result);
        // La entrada 129 actúa como centinela para que la UI rechace una
        // respuesta que excede el máximo local de 128 documentos.
        Assert.AreEqual(129, result!.Results.Count);
        Assert.AreEqual(199, result.SuccessCount);
        Assert.AreEqual(1, result.FailureCount);
        Assert.IsFalse(result.Results[1].IsSuccess);
        Assert.AreEqual(
            "firma rechazada",
            result.Results[1].Error);
    }

    private static BatchSignParameters NewParameters() =>
        new()
        {
            InputPaths =
            [
                @"C:\entrada\a.pdf",
                @"C:\entrada\b.pdf",
            ],
            DirectoryPath = string.Empty,
            OutputDirectory = @"C:\salida",
            CertificateId = "cert-1",
            CertificateIndex = 0,
            AdditionalCertificateIds = ["cert-2"],
            Format = "pades",
            Action = "cosign",
            Overwrite = "rename",
            VisibleSeal = new VisibleSealParameters
            {
                Page = "1",
                X = 0.1,
                Y = 0.1,
                Width = 0.3,
                Height = 0.1,
                PageWidth = 595.28,
                PageHeight = 841.89,
            },
            AllowInvalidPdf = false,
            StrictCompatibility = false,
            QrContent = "https://example.invalid/verify",
            Reason = "Aprobación",
            Location = "Granada",
            ContactInfo = "Unidad",
            ExtraOptions = new Dictionary<string, string>
            {
                ["profile"] = "baseline",
            },
            DocumentOverrides = [],
        };

    private sealed class RecordingIpcClient : IIpcClient
    {
        public IpcHello? ServerHello => null;
        public string? Action { get; private set; }
        public object? Parameters { get; private set; }
        public Type? ParametersType { get; private set; }
        public Type? DataType { get; private set; }

        public Task<IpcCallResult<TData>> SendAsync<TParameters, TData>(
            string action,
            TParameters parameters,
            CancellationToken cancellationToken = default)
        {
            Action = action;
            Parameters = parameters;
            ParametersType = typeof(TParameters);
            DataType = typeof(TData);
            return Task.FromResult(new IpcCallResult<TData>
            {
                Protocol = DesktopIpcProtocol.Name,
                RequestId = "batch-request",
                TraceId = "batch-trace",
                Action = action,
                IsSuccess = true,
                Outcome = "success",
                Phase = "operation",
                Retryable = false,
            });
        }

        public ValueTask DisposeAsync() => ValueTask.CompletedTask;
    }
}
