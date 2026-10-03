// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class PortalSealSessionTests
{
    [TestMethod]
    public void RequestAndResult_KeepDocumentOutOfCommandLineAndCancelOnce()
    {
        var dir = Path.Combine(Path.GetTempPath(), $"grxfirma-portal-{Guid.NewGuid():N}");
        Directory.CreateDirectory(dir);
        try
        {
            var pdf = Path.Combine(dir, "document.pdf");
            var result = Path.Combine(dir, "result.json");
            var request = Path.Combine(dir, "request.json");
            File.WriteAllBytes(pdf, "%PDF-test"u8.ToArray());
            File.WriteAllText(request, JsonSerializer.Serialize(new
            {
                documentPath = pdf, resultPath = result, signerName = "Nombre de prueba",
            }));
            Assert.IsTrue(PortalSealSession.TryLoad(
                ["--portal-seal-request", request], out var session, out var requested));
            Assert.IsTrue(requested);
            Assert.IsNotNull(session);
            Assert.AreEqual("Nombre de prueba", session!.SignerName);
            Assert.IsTrue(session!.Submit("cancel"));
            session!.CancelOnClose();
            Assert.AreEqual("cancel", JsonDocument.Parse(File.ReadAllBytes(result))
                .RootElement.GetProperty("action").GetString());
            Assert.IsFalse(session!.Submit("without"));
        }
        finally { Directory.Delete(dir, true); }
    }

    [TestMethod]
    public void Request_RejectsUnexpectedPathsAndProperties()
    {
        var dir = Path.Combine(Path.GetTempPath(), $"grxfirma-portal-{Guid.NewGuid():N}");
        Directory.CreateDirectory(dir);
        try
        {
            var pdf = Path.Combine(dir, "document.pdf");
            var request = Path.Combine(dir, "request.json");
            File.WriteAllBytes(pdf, "%PDF-test"u8.ToArray());
            File.WriteAllText(request, JsonSerializer.Serialize(new
            {
                documentPath = pdf,
                resultPath = Path.Combine(Path.GetTempPath(), "outside.json"),
                signerName = "Prueba",
            }));
            Assert.IsFalse(PortalSealSession.TryLoad(
                ["--portal-seal-request", request], out _, out _));
            File.WriteAllText(request, JsonSerializer.Serialize(new
            {
                documentPath = pdf, resultPath = Path.Combine(dir, "result.json"),
                signerName = "Prueba", extra = 1,
            }));
            Assert.IsFalse(PortalSealSession.TryLoad(
                ["--portal-seal-request", request], out _, out _));
        }
        finally { Directory.Delete(dir, true); }
    }

    [TestMethod]
    public void Result_SerializesNormalizedPlacementContract()
    {
        var dir = Path.Combine(Path.GetTempPath(), $"grxfirma-portal-{Guid.NewGuid():N}");
        Directory.CreateDirectory(dir);
        try
        {
            var pdf = Path.Combine(dir, "document.pdf");
            var result = Path.Combine(dir, "result.json");
            var request = Path.Combine(dir, "request.json");
            File.WriteAllBytes(pdf, "%PDF-test"u8.ToArray());
            File.WriteAllText(request, JsonSerializer.Serialize(new
            {
                documentPath = pdf, resultPath = result, signerName = "Prueba",
            }));
            Assert.IsTrue(PortalSealSession.TryLoad(
                ["--portal-seal-request", request], out var session, out _));
            Assert.IsNotNull(session);
            Assert.IsTrue(session!.Submit("place", [new VisibleSealPlacementParameters
            {
                Page = 1,
                Rect = new VisibleSealRectParameters
                {
                    X = 0.2, Y = 0.1, Width = 0.3, Height = 0.2,
                },
                Rotation = 45,
            }]));
            using var json = JsonDocument.Parse(File.ReadAllBytes(result));
            var placement = json.RootElement.GetProperty("visibleSealPlacements")[0];
            Assert.AreEqual(1, placement.GetProperty("page").GetInt32());
            Assert.AreEqual(0.3, placement.GetProperty("rect").GetProperty("w").GetDouble());
            Assert.AreEqual(45, placement.GetProperty("rotation").GetInt32());
        }
        finally { Directory.Delete(dir, true); }
    }
}
