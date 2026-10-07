// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Net;
using System.Net.Sockets;
using System.Text;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class OfficialUpdateCheckerTests
{
    private sealed class Clock(DateTimeOffset now) : TimeProvider
    {
        public DateTimeOffset Now { get; set; } = now;
        public override DateTimeOffset GetUtcNow() => Now;
    }

    private sealed class Handler(Func<HttpRequestMessage, CancellationToken,
        Task<HttpResponseMessage>> send) : HttpMessageHandler
    {
        protected override Task<HttpResponseMessage> SendAsync(
            HttpRequestMessage request, CancellationToken cancellationToken) =>
            send(request, cancellationToken);
    }

    private static byte[] Release(string tag = "v0.0.108", string? url = null,
        string flags = "") => Encoding.UTF8.GetBytes(
            "{\"tag_name\":\"" + tag + "\",\"html_url\":\"" +
            (url ?? "https://github.com/aavidad/GrxFirma/releases/tag/" + tag) +
            "\"" + flags + "}");

    [TestMethod]
    public void ComparesVersionsAndDailyCycle()
    {
        Assert.IsTrue(OfficialUpdateChecker.IsNewer("0.0.105", "v0.0.108"));
        Assert.IsTrue(OfficialUpdateChecker.IsNewer("1.2", "1.2.1"));
        Assert.IsTrue(OfficialUpdateChecker.IsNewer("1.2.3-rc1", "1.2.3"));
        Assert.IsFalse(OfficialUpdateChecker.IsNewer("v1.2.3", "1.2.3"));
        Assert.IsFalse(OfficialUpdateChecker.IsNewer("dev", "2.0.0"));
        Assert.IsFalse(OfficialUpdateChecker.IsNewer("1.0.0", "2.0.0-rc1"));
        var clock = new Clock(new DateTimeOffset(2026, 10, 3, 8, 0, 0, TimeSpan.Zero));
        var schedule = new UpdateNoticeSchedule(clock);
        Assert.IsTrue(schedule.IsDue);
        schedule.MarkAttempt();
        clock.Now = clock.Now.AddHours(4).AddMinutes(59);
        Assert.IsFalse(schedule.IsDue);
        clock.Now = clock.Now.AddMinutes(1);
        Assert.IsTrue(schedule.IsDue);
        Assert.IsFalse(UpdateNoticeSchedule.IsDueSince(clock.Now.AddMinutes(-1), clock.Now));
        Assert.IsTrue(UpdateNoticeSchedule.IsDueSince(clock.Now.AddHours(-5), clock.Now));
        schedule.Dismiss("v0.0.108");
        Assert.IsFalse(schedule.ShouldShow("v0.0.108"));
        Assert.IsTrue(schedule.ShouldShow("v0.0.109"));
        Assert.IsTrue(new UpdateNoticeSchedule(clock).ShouldShow("v0.0.108"));
    }

    [TestMethod]
    public void ParsesOnlyOfficialStableRelease()
    {
        Assert.AreEqual("v0.0.108", OfficialUpdateChecker.ParseRelease(Release())?.Version);
        Assert.IsFalse(OfficialUpdateChecker.IsReleaseForVersion(
            "https://github.com/aavidad/GrxFirma/releases/tag/v0.0.109", "v0.0.108"));
        Assert.IsNull(OfficialUpdateChecker.ParseRelease(Release(flags: ",\"draft\":true")));
        Assert.IsNull(OfficialUpdateChecker.ParseRelease(Release(flags: ",\"prerelease\":true")));
        Assert.IsNull(OfficialUpdateChecker.ParseRelease(Release(tag: "v1.0.0-rc1")));
        foreach (var bytes in new[] {
            Release(url: "https://github.com/aavidad/GrxFirma/releases/download/v0.0.108/file.exe"),
            Release(url: "https://github.com.evil/aavidad/GrxFirma/releases/tag/v0.0.108"),
            Release(url: "https://github.com/aavidad/GrxFirma/releases/tag/v0.0.109"),
            Release(tag: "v01.2.3"), Release(tag: "v1.2.3-01"), Release(flags: ",\"draft\":\"false\""),
            new byte[OfficialUpdateChecker.MaximumResponseBytes + 1] })
            Assert.ThrowsExactly<InvalidDataException>(() => OfficialUpdateChecker.ParseRelease(bytes));
    }

    [TestMethod]
    public async Task InjectedHttpHasFixedRequestAndBodyLimit()
    {
        using var client = new HttpClient(new Handler((request, _) => {
            Assert.AreEqual(OfficialUpdateChecker.LatestApiUrl, request.RequestUri?.ToString());
            Assert.AreEqual(HttpMethod.Get, request.Method);
            return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
                { Content = new ByteArrayContent(Release()) });
        }));
        Assert.AreEqual("v0.0.108", (await new OfficialUpdateChecker(client).CheckAsync())?.Version);
        using var large = new HttpClient(new Handler((_, _) => Task.FromResult(
            new HttpResponseMessage(HttpStatusCode.OK) {
                Content = new ByteArrayContent(new byte[OfficialUpdateChecker.MaximumResponseBytes + 1])
            })));
        var tooLarge = await Assert.ThrowsExactlyAsync<OfficialUpdateCheckException>(() =>
            new OfficialUpdateChecker(large).CheckAsync());
        Assert.AreEqual(UpdateCheckFailureKind.InvalidResponse, tooLarge.Failure.Kind);
        Assert.IsInstanceOfType<InvalidDataException>(tooLarge.InnerException);
        Assert.AreEqual(TimeSpan.FromSeconds(10), OfficialUpdateChecker.MaximumDuration);
    }
    [TestMethod]
    public async Task InjectedNetworkCancellationDoesNotHang()
    {
        var called = false;
        using var client = new HttpClient(new Handler(async (request, token) =>
        {
            called = true;
            Assert.AreEqual(OfficialUpdateChecker.LatestApiUrl, request.RequestUri?.ToString());
            await Task.Delay(Timeout.InfiniteTimeSpan, token);
            return new HttpResponseMessage(HttpStatusCode.OK);
        }));
        using var cancellation = new CancellationTokenSource(TimeSpan.FromMilliseconds(50));
        await Assert.ThrowsExactlyAsync<TaskCanceledException>(() =>
            new OfficialUpdateChecker(client).CheckAsync(cancellation.Token));
        Assert.IsTrue(called);
    }

    [TestMethod]
    public void HandlerSendsWindowsCredentialsOnlyToTheSystemProxy()
    {
        using var handler = OfficialUpdateChecker.CreateHandler();
        Assert.IsFalse(handler.UseDefaultCredentials);
        Assert.IsFalse(handler.AllowAutoRedirect);
        Assert.IsTrue(handler.UseProxy);
        Assert.IsNull(handler.Proxy);
        Assert.AreSame(CredentialCache.DefaultCredentials, handler.DefaultProxyCredentials);
    }

    [TestMethod]
    public async Task HttpStatusFailuresAreClassifiedWithTheirCode()
    {
        foreach (var (status, kind) in new[] {
            (HttpStatusCode.ProxyAuthenticationRequired, UpdateCheckFailureKind.ProxyAuthenticationRequired),
            (HttpStatusCode.Forbidden, UpdateCheckFailureKind.HttpStatus),
            (HttpStatusCode.ServiceUnavailable, UpdateCheckFailureKind.HttpStatus) })
        {
            using var client = new HttpClient(new Handler((_, _) =>
                Task.FromResult(new HttpResponseMessage(status))));
            var error = await Assert.ThrowsExactlyAsync<OfficialUpdateCheckException>(() =>
                new OfficialUpdateChecker(client).CheckAsync());
            Assert.AreEqual(kind, error.Failure.Kind);
            Assert.AreEqual((int)status, error.Failure.StatusCode);
        }
        using var notFound = new HttpClient(new Handler((_, _) =>
            Task.FromResult(new HttpResponseMessage(HttpStatusCode.NotFound))));
        Assert.IsNull(await new OfficialUpdateChecker(notFound).CheckAsync());
    }

    [TestMethod]
    public async Task OwnTimeoutIsReportedAsTimeoutNotAsCancellation()
    {
        using var client = new HttpClient(new Handler((_, _) =>
            throw new TaskCanceledException("timeout", new TimeoutException())));
        var error = await Assert.ThrowsExactlyAsync<OfficialUpdateCheckException>(() =>
            new OfficialUpdateChecker(client).CheckAsync());
        Assert.AreEqual(UpdateCheckFailureKind.Timeout, error.Failure.Kind);
        Assert.AreEqual("request_timeout", error.Failure.LogCode);
    }

    [TestMethod]
    public void ClassifiesTransportFailuresWithoutKeepingRemoteText()
    {
        const string proxy = "http://usuario:secreto@proxy.example:4070/";
        var cases = new (Exception Error, UpdateCheckFailureKind Kind, string Log)[]
        {
            (new HttpRequestException(HttpRequestError.ProxyTunnelError,
                $"The proxy tunnel request to proxy '{proxy}' failed with status code '407'."),
                UpdateCheckFailureKind.ProxyAuthenticationRequired, "proxy_auth_required 407"),
            (new HttpRequestException(HttpRequestError.ProxyTunnelError,
                $"The proxy tunnel request to proxy '{proxy}' failed with status code '502'."),
                UpdateCheckFailureKind.Proxy, "proxy_error 502"),
            (new HttpRequestException(HttpRequestError.ProxyTunnelError, proxy),
                UpdateCheckFailureKind.Proxy, "proxy_error"),
            (new HttpRequestException(HttpRequestError.Unknown, proxy, null, HttpStatusCode.ProxyAuthenticationRequired),
                UpdateCheckFailureKind.ProxyAuthenticationRequired, "proxy_auth_required 407"),
            (new HttpRequestException(HttpRequestError.NameResolutionError, proxy,
                new SocketException((int)SocketError.HostNotFound)),
                UpdateCheckFailureKind.NameResolution, "dns_error"),
            (new HttpRequestException(HttpRequestError.ConnectionError, proxy,
                new SocketException((int)SocketError.HostNotFound)),
                UpdateCheckFailureKind.NameResolution, "dns_error"),
            (new HttpRequestException(HttpRequestError.ConnectionError, proxy,
                new SocketException((int)SocketError.ConnectionRefused)),
                UpdateCheckFailureKind.Network, "network_error"),
            (new HttpRequestException(HttpRequestError.ConnectionError, proxy,
                new SocketException((int)SocketError.TimedOut)),
                UpdateCheckFailureKind.Timeout, "request_timeout"),
            (new HttpRequestException(HttpRequestError.ConnectionError, proxy),
                UpdateCheckFailureKind.Network, "network_error"),
            (new HttpRequestException(HttpRequestError.SecureConnectionError, proxy,
                new System.Security.Authentication.AuthenticationException(proxy)),
                UpdateCheckFailureKind.Tls, "tls_error"),
            (new HttpRequestException(proxy, new System.Security.Authentication.AuthenticationException(proxy)),
                UpdateCheckFailureKind.Tls, "tls_error"),
            (new HttpRequestException(proxy), UpdateCheckFailureKind.Network, "network_error"),
            (new System.Text.Json.JsonException(proxy), UpdateCheckFailureKind.InvalidResponse, "invalid_response"),
            (new InvalidOperationException(proxy), UpdateCheckFailureKind.Unknown, "unknown_error"),
        };
        foreach (var (error, kind, log) in cases)
        {
            var failure = UpdateCheckFailure.Classify(error);
            Assert.AreEqual(kind, failure.Kind, error.Message);
            Assert.AreEqual(log, failure.LogCode);
            Assert.IsFalse(failure.LogCode.Contains("proxy.example", StringComparison.Ordinal));
            Assert.IsFalse(failure.LogCode.Contains("secreto", StringComparison.Ordinal));
            Assert.IsTrue(failure.MessageKey.StartsWith("winui.actualizaciones.error_", StringComparison.Ordinal));
        }
        Assert.AreEqual("winui.actualizaciones.error_proxy",
            new UpdateCheckFailure(UpdateCheckFailureKind.ProxyAuthenticationRequired, 407).MessageKey);
        var wrapped = new OfficialUpdateCheckException(new(UpdateCheckFailureKind.HttpStatus, 503));
        Assert.AreEqual("http_status 503", wrapped.Message);
        Assert.AreSame(wrapped.Failure, UpdateCheckFailure.Classify(new AggregateException(wrapped)));
    }
}
