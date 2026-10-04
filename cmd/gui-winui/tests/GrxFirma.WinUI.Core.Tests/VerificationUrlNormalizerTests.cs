// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class VerificationUrlNormalizerTests
{
    [TestMethod]
    public void UnicodeAndAceProduceTheSameVerificationAddress()
    {
        using var cases = JsonDocument.Parse(File.ReadAllText(
            Path.Combine(AppContext.BaseDirectory, "verification_urls.json")));
        foreach (var item in cases.RootElement.GetProperty("valid").EnumerateArray())
        {
            var input = item.GetProperty("input").GetString()!;
            var expected = item.GetProperty("expected").GetString()!;
            Assert.IsTrue(VerificationUrlNormalizer.TryNormalize(input, out var actual), input);
            Assert.AreEqual(expected, actual, input);
            Assert.IsTrue(VerificationUrlNormalizer.TryNormalize(actual, out var again), input);
            Assert.AreEqual(actual, again, input);
        }
    }

    [TestMethod]
    public void InvalidIdnaAndUnsafeUrlsAreRejected()
    {
        using var cases = JsonDocument.Parse(File.ReadAllText(
            Path.Combine(AppContext.BaseDirectory, "verification_urls.json")));
        foreach (var item in cases.RootElement.GetProperty("invalid").EnumerateArray())
        {
            Assert.IsFalse(VerificationUrlNormalizer.TryNormalize(item.GetString(), out _), item.GetString());
        }
    }
}
