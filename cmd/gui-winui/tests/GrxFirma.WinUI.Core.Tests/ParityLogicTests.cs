// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;
using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class ParityLogicTests
{
    [TestMethod]
    public void VerificationReport_ContainsOriginalEvidenceAndUtcMetadata()
    {
        var result = JsonSerializer.Deserialize<VerifyResult>("""
            {"valid":false,"reason":"Firma no válida","integrity":{"status":"invalid"}}
            """)!;
        using var report = JsonDocument.Parse(VerificationReport.Serialize(
            result, "C:/signed.p7s", "C:/original.pdf",
            new DateTimeOffset(2026, 10, 3, 15, 0, 0, TimeSpan.FromHours(2))));
        Assert.AreEqual("grxfirma-gui", report.RootElement.GetProperty("source").GetString());
        Assert.AreEqual("C:/original.pdf", report.RootElement.GetProperty("originalPath").GetString());
        Assert.AreEqual("2026-10-03T13:00:00.0000000+00:00",
            report.RootElement.GetProperty("generatedAt").GetString());
        Assert.IsFalse(report.RootElement.GetProperty("result").GetProperty("valid").GetBoolean());
        Assert.AreEqual("invalid", report.RootElement.GetProperty("result")
            .GetProperty("integrity").GetProperty("status").GetString());
    }

    [TestMethod]
    public void Tsa_RequiresHttpsWithoutCredentialsOrFragment()
    {
        Assert.IsTrue(TsaConfiguration.TryNormalize(true, " https://tsa.example.test/path ", out var url));
        Assert.AreEqual("https://tsa.example.test/path", url);
        Assert.IsFalse(TsaConfiguration.TryNormalize(true, "http://tsa.example.test", out _));
        Assert.IsFalse(TsaConfiguration.TryNormalize(true, "https://user@tsa.example.test", out _));
        Assert.IsFalse(TsaConfiguration.TryNormalize(true, "https://tsa.example.test/#x", out _));
        Assert.IsFalse(TsaConfiguration.TryNormalize(true, "", out _));
        Assert.IsTrue(TsaConfiguration.TryNormalize(false, "", out _));
        Assert.IsTrue(TsaConfiguration.TryNormalize(false, "http://old.example.test", out _));
    }

    [TestMethod]
    public void TsaSettings_UseQtWireKeysAcrossSaveSnapshot()
    {
        var settings = JsonSerializer.Deserialize<DesktopSettingsDocument>("""
            {"idioma":"es","tsaEnabled":true,"tsaUrl":"https://tsa.example.test/"}
            """)!;
        var snapshot = settings.CreateSafeSaveSnapshot();
        Assert.AreEqual(true, snapshot.TsaEnabled);
        Assert.AreEqual("https://tsa.example.test/", snapshot.TsaUrl);
        using var json = JsonDocument.Parse(JsonSerializer.Serialize(snapshot));
        Assert.IsTrue(json.RootElement.GetProperty("tsaEnabled").GetBoolean());
        Assert.AreEqual("https://tsa.example.test/", json.RootElement.GetProperty("tsaUrl").GetString());
    }

    [TestMethod]
    public void StructuredFilters_CombineAllCriteria()
    {
        var certificate = new CertificateInfo
        {
            Nif = "12345678Z", Organization = "Entidad", Type = "representacion",
        };
        Assert.IsTrue(CertificateStructuredFilter.Matches(certificate, true, true,
            new HashSet<string> { "representacion", "sello" }));
        Assert.IsFalse(CertificateStructuredFilter.Matches(certificate, true, true,
            new HashSet<string> { "sello" }));
        Assert.IsFalse(CertificateStructuredFilter.Matches(certificate with { Organization = "" }, true, true,
            new HashSet<string>()));
        Assert.IsFalse(CertificateStructuredFilter.Matches(certificate with { Nif = "" }, true, false,
            new HashSet<string>()));
    }

    [TestMethod]
    public void SupportPlan_HasThreeStepsAndSingleDestination()
    {
        foreach (var goal in new[] { "sign", "verify", "certificate", "sign-failure", "verify-failure", "support" })
        {
            var plan = SupportAssistantPlan.ForGoal(goal);
            Assert.AreEqual(3, plan.StepKeys.Count);
            Assert.IsTrue(plan.Destination is "sign" or "verify" or "certificates" or "diagnostics");
        }
        var text = SupportAssistantPlan.ExportText("goal", "step", "summary\r\ninjected",
            "owner", "meaning", "action", ["first", "second"]);
        StringAssert.Contains(text, "summary  injected");
        StringAssert.Contains(text, "1. first");
        Assert.IsFalse(RestServerLaunch.IsUsableToken("short"));
        Assert.IsTrue(RestServerLaunch.IsUsableToken(new string('a', 64)));
        Assert.IsTrue(RestServerLaunch.IsUsableToken(RestServerLaunch.CreateToken()));
        CollectionAssert.AreEqual(new[] { "--rest", "--rest-addr", "127.0.0.1:63118" },
            RestServerLaunch.Arguments.ToArray());
    }
}
