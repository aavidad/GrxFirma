// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;
using System.Text.Json.Serialization;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class PostSignVerificationTests
{
    private const string Input = @"C:\qa\contenido.pdf";
    // Una extensión no demuestra el formato: ASiC también podía usar .p7s.
    private const string Output = @"C:\qa\resultado.p7s";

    [TestMethod]
    [DataRow("CAdES")]
    [DataRow("cades")]
    [DataRow(" CADES ")]
    public void InitialCades_UsesExactOriginal(string format)
    {
        var parameters = PostSignVerification.Create(Output, Input, format, "sign");
        Assert.AreEqual(Output, parameters.InputPath);
        Assert.AreEqual(Input, parameters.OriginalPath);
    }

    [TestMethod]
    [DataRow("PAdES")]
    [DataRow("XAdES")]
    [DataRow("XMLdSig")]
    [DataRow("FacturaE")]
    [DataRow("ODF")]
    [DataRow("OOXML")]
    [DataRow("ASiC-XAdES")]
    public void EmbeddedFormats_DoNotRequestDetachedVerification(string format)
    {
        foreach (var action in new[] { "sign", "cosign", "countersign" })
        {
            var parameters = PostSignVerification.Create(Output, Input, format, action);
            Assert.AreEqual(Output, parameters.InputPath);
            Assert.IsNull(parameters.OriginalPath, $"{format}/{action}");
        }
    }

    [TestMethod]
    [DataRow("cosign")]
    [DataRow("countersign")]
    [DataRow("unknown")]
    public void CadesSubsequentActions_DoNotUseContainerAsContent(string action)
    {
        var parameters = PostSignVerification.Create(Output, Input, "CAdES", action);
        Assert.IsNull(parameters.OriginalPath);
    }

    [TestMethod]
    [DataRow(null)]
    [DataRow("")]
    [DataRow("nuevo-formato")]
    public void MissingOrFutureMetadata_DoesNotGuessFromFileName(string? format)
    {
        var parameters = PostSignVerification.Create(Output, Input, format, "sign");
        Assert.IsNull(parameters.OriginalPath);
    }

    [TestMethod]
    public void EmbeddedVerification_DoesNotReadOrRequireAnOriginal()
    {
        var parameters = PostSignVerification.Create(Output, string.Empty, "PAdES", "sign");
        Assert.IsNull(parameters.OriginalPath);
        Assert.ThrowsExactly<ArgumentException>(() =>
            PostSignVerification.Create(Output, string.Empty, "CAdES", "sign"));
        Assert.ThrowsExactly<ArgumentException>(() =>
            PostSignVerification.Create(string.Empty, Input, "PAdES", "sign"));
    }

    [TestMethod]
    public void WireContract_OmitsUnusedOriginal_AndPreservesLegacyResults()
    {
        var options = new JsonSerializerOptions
        {
            DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
        };
        var embedded = PostSignVerification.Create(Output, Input, "ASiC-XAdES", "sign");
        using var document = JsonDocument.Parse(JsonSerializer.Serialize(embedded, options));
        Assert.IsFalse(document.RootElement.TryGetProperty("originalPath", out _));
        var legacy = JsonSerializer.Deserialize<SignResult>("""{"OutputPath":"signed.p7s"}""")!;
        Assert.AreEqual("signed.p7s", legacy.OutputPath);
        Assert.IsNull(legacy.Format);
        var current = JsonSerializer.Deserialize<SignResult>("""{"OutputPath":"signed.p7s","format":"ASiC-XAdES"}""")!;
        Assert.AreEqual("ASiC-XAdES", current.Format);
    }
}
