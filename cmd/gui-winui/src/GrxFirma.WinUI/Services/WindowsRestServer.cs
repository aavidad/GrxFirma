// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Diagnostics;
using System.Net.Security;
using System.Security.Cryptography;
using System.Security.Cryptography.X509Certificates;
using System.Text.Json;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Services;

internal sealed class WindowsRestServer : IDisposable
{
    private Process? _process;
    public string? Token { get; private set; }
    public bool IsOwnedRunning => _process is { HasExited: false };

    public async Task<bool> IsHealthyAsync(CancellationToken cancellationToken)
    {
        using var handler = new HttpClientHandler
        {
            ServerCertificateCustomValidationCallback = (_, certificate, _, errors) =>
                IsExpectedLocalCertificate(certificate, errors),
        };
        using var client = new HttpClient(handler) { Timeout = TimeSpan.FromSeconds(2) };
        try
        {
            using var response = await client.GetAsync(
                "https://127.0.0.1:63118/health", HttpCompletionOption.ResponseHeadersRead,
                cancellationToken);
            if (!response.IsSuccessStatusCode || response.Content.Headers.ContentLength > 512)
                return false;
            await using var stream = await response.Content.ReadAsStreamAsync(cancellationToken);
            var buffer = new byte[513];
            var count = 0;
            while (count < buffer.Length)
            {
                var read = await stream.ReadAsync(buffer.AsMemory(count), cancellationToken);
                if (read == 0) break;
                count += read;
            }
            if (count > 512) return false;
            using var json = JsonDocument.Parse(buffer.AsMemory(0, count));
            return json.RootElement.ValueKind == JsonValueKind.Object &&
                json.RootElement.TryGetProperty("ok", out var ok) &&
                ok.ValueKind == JsonValueKind.True &&
                json.RootElement.TryGetProperty("service", out var service) &&
                service.ValueKind == JsonValueKind.String &&
                service.GetString() == "GrxFirma REST local";
        }
        catch (HttpRequestException) { return false; }
        catch (TaskCanceledException) when (!cancellationToken.IsCancellationRequested) { return false; }
        catch (JsonException) { return false; }
    }

    private static bool IsExpectedLocalCertificate(
        X509Certificate2? certificate, SslPolicyErrors errors)
    {
        if (certificate is null ||
            errors.HasFlag(SslPolicyErrors.RemoteCertificateNotAvailable) ||
            errors.HasFlag(SslPolicyErrors.RemoteCertificateNameMismatch)) return false;
        try
        {
            var path = Path.Combine(
                Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData),
                "GrxFirma", "tls", "websocket-localhost.crt.pem");
            using var expected = X509Certificate2.CreateFromPemFile(path);
            var now = DateTime.UtcNow;
            return now >= certificate.NotBefore.ToUniversalTime() &&
                now <= certificate.NotAfter.ToUniversalTime() &&
                CryptographicOperations.FixedTimeEquals(
                    certificate.RawData, expected.RawData);
        }
        catch (IOException) { return false; }
        catch (UnauthorizedAccessException) { return false; }
        catch (CryptographicException) { return false; }
        catch (ArgumentException) { return false; }
    }

    public async Task StartAsync(CancellationToken cancellationToken)
    {
        if (IsOwnedRunning) return;
        if (await IsHealthyAsync(cancellationToken))
            throw new InvalidOperationException("REST_PORT_IN_USE");
        var baseDirectory = AppContext.BaseDirectory;
        var candidates = new[]
        {
            Path.Combine(baseDirectory, "grxfirma.exe"),
            Path.GetFullPath(Path.Combine(baseDirectory, "..", "CLI", "grxfirma.exe")),
        };
        var executable = candidates.FirstOrDefault(File.Exists);
        if (executable is null)
            throw new FileNotFoundException("REST_ENGINE_NOT_FOUND");

        var token = RestServerLaunch.CreateToken();
        var info = new ProcessStartInfo(executable)
        {
            UseShellExecute = false,
            CreateNoWindow = true,
            RedirectStandardOutput = false,
            RedirectStandardError = false,
        };
        foreach (var argument in RestServerLaunch.Arguments) info.ArgumentList.Add(argument);
        info.Environment.Remove("AUTOFIRMAV2_REST_TOKEN");
        info.Environment[RestServerLaunch.TokenEnvironmentName] = token;
        var process = Process.Start(info) ?? throw new InvalidOperationException("REST_START_FAILED");
        _process?.Dispose();
        _process = process;
        Token = token;
        try
        {
            for (var attempt = 0; attempt < 15; attempt++)
            {
                await Task.Delay(200, cancellationToken);
                if (process.HasExited) break;
                if (await IsHealthyAsync(cancellationToken)) return;
            }
        }
        catch
        {
            Stop();
            throw;
        }
        Stop();
        throw new InvalidOperationException("REST_HEALTH_TIMEOUT");
    }

    public void Stop()
    {
        var process = _process;
        _process = null;
        Token = null;
        if (process is null) return;
        try
        {
            if (!process.HasExited) process.Kill(entireProcessTree: true);
        }
        catch (InvalidOperationException) { }
        finally { process.Dispose(); }
    }

    public void Dispose() => Stop();
}
