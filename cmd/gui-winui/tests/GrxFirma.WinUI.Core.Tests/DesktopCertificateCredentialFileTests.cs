// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Security.Cryptography;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class DesktopCertificateCredentialFileTests
{
    [TestMethod]
    public async Task ReadAsync_ReadsSupportedCredentialExactly()
    {
        var path = Path.Combine(
            Path.GetTempPath(),
            $"grxfirma-credential-{Guid.NewGuid():N}.p12");
        var expected = Enumerable.Range(0, 4096)
            .Select(index => unchecked((byte)index))
            .ToArray();
        byte[]? actual = null;
        try
        {
            await File.WriteAllBytesAsync(path, expected);

            actual = await DesktopCertificateCredentialFile.ReadAsync(path);

            CollectionAssert.AreEqual(expected, actual);
            Assert.AreEqual(
                Path.GetFileName(path),
                DesktopCertificateCredentialFile.SafeDisplayName(path));
        }
        finally
        {
            CryptographicOperations.ZeroMemory(expected);
            if (actual is not null)
            {
                CryptographicOperations.ZeroMemory(actual);
            }
            File.Delete(path);
        }
    }

    [TestMethod]
    public async Task ReadAsync_RejectsUnsupportedEmptyAndOversizedFiles()
    {
        var root = Path.Combine(
            Path.GetTempPath(),
            $"grxfirma-credential-tests-{Guid.NewGuid():N}");
        Directory.CreateDirectory(root);
        var unsupported = Path.Combine(root, "credential.txt");
        var empty = Path.Combine(root, "credential.pfx");
        var oversized = Path.Combine(root, "oversized.p12");
        try
        {
            await File.WriteAllTextAsync(unsupported, "not-a-credential");
            await File.WriteAllBytesAsync(empty, []);
            await using (var stream = new FileStream(
                oversized,
                FileMode.CreateNew,
                FileAccess.Write,
                FileShare.None))
            {
                stream.SetLength(
                    DesktopOperationsClient.MaximumCredentialBytes + 1L);
            }

            await Assert.ThrowsExactlyAsync<InvalidDataException>(
                () => DesktopCertificateCredentialFile.ReadAsync(
                    unsupported));
            await Assert.ThrowsExactlyAsync<InvalidDataException>(
                () => DesktopCertificateCredentialFile.ReadAsync(empty));
            await Assert.ThrowsExactlyAsync<InvalidDataException>(
                () => DesktopCertificateCredentialFile.ReadAsync(
                    oversized));
        }
        finally
        {
            Directory.Delete(root, recursive: true);
        }
    }

    [TestMethod]
    [DataRow("credential.p12", true)]
    [DataRow("credential.PFX", true)]
    [DataRow("credential.pem", true)]
    [DataRow("credential.cer", true)]
    [DataRow("credential.crt", true)]
    [DataRow("credential.txt", false)]
    [DataRow("", false)]
    public void IsSupportedPath_RecognizesOnlyCredentialExtensions(
        string path,
        bool expected)
    {
        Assert.AreEqual(
            expected,
            DesktopCertificateCredentialFile.IsSupportedPath(path));
    }
}
