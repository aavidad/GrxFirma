// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Net;
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
        await Assert.ThrowsExactlyAsync<InvalidDataException>(() =>
            new OfficialUpdateChecker(large).CheckAsync());
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
}
