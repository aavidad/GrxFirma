// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Ipc;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class LaunchOptionsTests
{
    [TestMethod]
    public void Parse_AcceptsLauncherArguments()
    {
        var ok = WinUiLaunchOptions.TryParse(
            [
                "--ipc-socket",
                @"\\.\pipe\UNIT_TEST_ONLY",
                "--backend-pid",
                "1234",
            ],
            out var options,
            out var error);

        Assert.IsTrue(ok, error);
        Assert.IsTrue(options.HasBackendEndpoint);
        Assert.AreEqual((uint)1234, options.BackendProcessId);
    }

    [TestMethod]
    public void Parse_RejectsPipeWithoutExpectedBackendPid()
    {
        var ok = WinUiLaunchOptions.TryParse(
            ["--ipc-socket", @"\\.\pipe\UNIT_TEST_ONLY"],
            out var options,
            out var error);

        Assert.IsFalse(ok);
        Assert.IsFalse(options.HasBackendEndpoint);
        Assert.IsFalse(string.IsNullOrWhiteSpace(error));
        Assert.IsFalse(error.Contains("UNIT_TEST_ONLY", StringComparison.Ordinal));
    }

    [TestMethod]
    public void Parse_StartHiddenIsFixedFlagAndCannotBeRepeated()
    {
        var arguments = new[]
        {
            "--ipc-socket", @"\\.\pipe\UNIT_TEST_ONLY",
            "--backend-pid", "1234", "--start-hidden",
        };
        Assert.IsTrue(WinUiLaunchOptions.TryParse(
            arguments, out var options, out _));
        Assert.IsTrue(options.StartHidden);
        Assert.IsFalse(WinUiLaunchOptions.TryParse(
            [..arguments, "--start-hidden"], out _, out _));
        Assert.IsFalse(WinUiLaunchOptions.TryParse(
            [..arguments, "--approve-signature"], out _, out _));
    }

    [TestMethod]
    public void LocalPipeValidation_RejectsRemoteAndNestedNames()
    {
        Assert.IsTrue(NamedPipeIpcConnector.TryGetLocalPipeName(
            @"\\.\pipe\UNIT_TEST_ONLY",
            out var validName));
        Assert.AreEqual("UNIT_TEST_ONLY", validName);

        Assert.IsFalse(NamedPipeIpcConnector.TryGetLocalPipeName(
            @"\\remote\pipe\UNIT_TEST_ONLY",
            out _));
        Assert.IsFalse(NamedPipeIpcConnector.TryGetLocalPipeName(
            @"\\.\pipe\nested\name",
            out _));
    }
}
