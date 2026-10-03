// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Net;
using System.Text.Json;
using System.Text.RegularExpressions;

namespace GrxFirma.WinUI.Core.Operations;

public sealed record OfficialRelease(string Version, string Url);

// Únicamente consulta metadatos: nunca descarga ni ejecuta una release.
public sealed class OfficialUpdateChecker
{
    public const string LatestApiUrl =
        "https://api.github.com/repos/aavidad/GrxFirma/releases/latest";
    private const string ReleasePrefix =
        "https://github.com/aavidad/GrxFirma/releases/tag/";
    public const int MaximumResponseBytes = 1024 * 1024;
    public static readonly TimeSpan MaximumDuration = TimeSpan.FromSeconds(10);
    private static readonly Regex SemVer = new(
        @"^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$",
        RegexOptions.CultureInvariant | RegexOptions.NonBacktracking);
    private readonly HttpClient _http;

    public OfficialUpdateChecker(HttpClient? http = null)
    {
        _http = http ?? new HttpClient(new HttpClientHandler
        {
            AllowAutoRedirect = false,
            UseDefaultCredentials = false,
        });
    }

    public static bool IsNewer(string current, string latest)
    {
        if (!TryVersion(current, out var a, out var aPre, allowShort: true) ||
            !TryVersion(latest, out var b, out var bPre) || bPre) return false;
        for (var i = 0; i < 3; i++)
            if (a[i] != b[i]) return b[i] > a[i];
        return aPre && !bPre;
    }

    private static bool TryVersion(string value, out long[] parts, out bool prerelease, bool allowShort = false)
    {
        parts = new long[3];
        prerelease = false;
        if (allowShort && Regex.IsMatch(value, @"^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$", RegexOptions.CultureInvariant))
            value += ".0";
        var match = SemVer.Match(value);
        if (!match.Success) return false;
        for (var i = 0; i < 3; i++)
            if (!long.TryParse(match.Groups[i + 1].Value, out parts[i])) return false;
        prerelease = match.Groups[4].Success;
        if (prerelease && match.Groups[4].Value.Split('.').Any(identifier =>
            identifier.Length > 1 && identifier[0] == '0' &&
            identifier.All(char.IsAsciiDigit))) return false;
        return true;
    }

    public static bool IsOfficialReleaseUrl(string url)
    {
        if (!url.StartsWith(ReleasePrefix, StringComparison.Ordinal) ||
            !Uri.TryCreate(url, UriKind.Absolute, out var uri) ||
            uri.AbsoluteUri != url || uri.UserInfo.Length != 0 ||
            uri.Port != 443 || uri.Query.Length != 0 || uri.Fragment.Length != 0)
            return false;
        var tag = url[ReleasePrefix.Length..];
        return !tag.Contains('/') && SemVer.IsMatch(tag);
    }

    public static bool IsReleaseForVersion(string url, string version) =>
        IsOfficialReleaseUrl(url) &&
        string.Equals(url, ReleasePrefix + version, StringComparison.Ordinal) &&
        TryVersion(version, out _, out var prerelease) && !prerelease;

    public static OfficialRelease? ParseRelease(ReadOnlySpan<byte> bytes)
    {
        if (bytes.Length > MaximumResponseBytes) throw new InvalidDataException("Release response too large");
        using var document = JsonDocument.Parse(bytes.ToArray());
        var root = document.RootElement;
        if (root.ValueKind != JsonValueKind.Object ||
            !root.TryGetProperty("tag_name", out var tagProperty) ||
            tagProperty.ValueKind != JsonValueKind.String ||
            !root.TryGetProperty("html_url", out var urlProperty) ||
            urlProperty.ValueKind != JsonValueKind.String)
            throw new InvalidDataException("Invalid release response");
        foreach (var name in new[] { "prerelease", "draft" })
            if (root.TryGetProperty(name, out var flag) &&
                flag.ValueKind is not (JsonValueKind.True or JsonValueKind.False))
                throw new InvalidDataException("Invalid release flag");
        if ((root.TryGetProperty("prerelease", out var pre) && pre.GetBoolean()) ||
            (root.TryGetProperty("draft", out var draft) && draft.GetBoolean()))
            return null;
        var tag = tagProperty.GetString() ?? string.Empty;
        var url = urlProperty.GetString() ?? string.Empty;
        if (!IsReleaseForVersion(url, tag))
            throw new InvalidDataException("Invalid official release");
        return new OfficialRelease(tag, url);
    }

    public async Task<OfficialRelease?> CheckAsync(CancellationToken cancellationToken = default)
    {
        using var timeout = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        timeout.CancelAfter(MaximumDuration);
        using var request = new HttpRequestMessage(HttpMethod.Get, LatestApiUrl);
        request.Headers.TryAddWithoutValidation("Accept", "application/vnd.github+json");
        request.Headers.TryAddWithoutValidation("User-Agent", "GrxFirma-UpdateCheck");
        request.Headers.TryAddWithoutValidation("X-GitHub-Api-Version", "2022-11-28");
        using var response = await _http.SendAsync(request,
            HttpCompletionOption.ResponseHeadersRead, timeout.Token).ConfigureAwait(false);
        if (response.StatusCode == HttpStatusCode.NotFound) return null;
        if (response.StatusCode != HttpStatusCode.OK)
            throw new HttpRequestException("Release service unavailable");
        if (response.Content.Headers.ContentLength > MaximumResponseBytes)
            throw new InvalidDataException("Release response too large");
        await using var stream = await response.Content.ReadAsStreamAsync(timeout.Token).ConfigureAwait(false);
        using var body = new MemoryStream();
        var buffer = new byte[8192];
        while (true)
        {
            var count = await stream.ReadAsync(buffer, timeout.Token).ConfigureAwait(false);
            if (count == 0) break;
            if (body.Length + count > MaximumResponseBytes)
                throw new InvalidDataException("Release response too large");
            body.Write(buffer, 0, count);
        }
        return ParseRelease(body.ToArray());
    }
}

public sealed class UpdateNoticeSchedule(TimeProvider clock)
{
    public static readonly TimeSpan Interval = TimeSpan.FromHours(5);
    public static bool IsDueSince(DateTimeOffset? last, DateTimeOffset now) =>
        last is null || now - last.Value >= Interval;
    private DateTimeOffset _next = DateTimeOffset.MinValue;
    private string? _dismissedVersion;

    public bool IsDue => clock.GetUtcNow() >= _next;
    public void MarkAttempt() => _next = clock.GetUtcNow() + Interval;
    // «Ahora no» oculta esa versión hasta el próximo arranque; una más nueva sí se avisa.
    public bool ShouldShow(string version) =>
        !string.Equals(version, _dismissedVersion, StringComparison.Ordinal);
    public void Dismiss(string version) => _dismissedVersion = version;
}
