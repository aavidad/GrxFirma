// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.RegularExpressions;

namespace GrxFirma.WinUI.Core.Operations;

public static class ReleaseNotesSections
{
    private static readonly Regex Heading = new(
        @"^## (?<version>[0-9]+\.[0-9]+\.[0-9]+) — [0-9]{4}-[0-9]{2}-[0-9]{2}\s*$",
        RegexOptions.CultureInvariant | RegexOptions.NonBacktracking,
        TimeSpan.FromMilliseconds(250));
    private static readonly Regex VersionPattern = new(
        @"^[0-9]+\.[0-9]+\.[0-9]+$",
        RegexOptions.CultureInvariant | RegexOptions.NonBacktracking,
        TimeSpan.FromMilliseconds(250));

    public static bool TryVersion(string? text, out Version version)
    {
        version = new Version(0, 0, 0);
        if (text is not null && text.Length <= 64 &&
            VersionPattern.IsMatch(text) && Version.TryParse(text, out var parsed) &&
            parsed is not null)
        {
            version = parsed;
            return true;
        }
        return false;
    }

    public static string Select(string source, string installed, string? lastSeen = null)
    {
        if (source.Length > 64 * 1024 || !TryVersion(installed, out var current))
        {
            return string.Empty;
        }
        if (lastSeen is not null && !TryVersion(lastSeen, out _))
        {
            return string.Empty;
        }

        try
        {
            var sections = new List<(Version Version, string Text)>();
            Version? sectionVersion = null;
            var section = new System.Text.StringBuilder();
            void Flush()
            {
                if (sectionVersion is not null && sections.Count < 64)
                {
                    var clean = Sanitize(section.ToString());
                    if (clean.Length > 0)
                    {
                        sections.Add((sectionVersion, clean));
                    }
                }
                section.Clear();
                sectionVersion = null;
            }
            foreach (var line in source.Split('\n'))
            {
                if (line.StartsWith("## ", StringComparison.Ordinal))
                {
                    Flush();
                    var match = Heading.Match(line.Trim());
                    if (match.Success && TryVersion(match.Groups["version"].Value, out var parsed))
                    {
                        sectionVersion = parsed;
                        section.AppendLine(line.Trim());
                    }
                }
                else if (sectionVersion is not null && section.Length < 48 * 1024)
                {
                    section.AppendLine(line);
                }
            }
            Flush();
            var previous = lastSeen is null ? null : Version.Parse(lastSeen);
            return string.Join("\n\n", sections
                .Where(item => item.Version.CompareTo(current) <= 0 &&
                    (previous is null || item.Version.CompareTo(previous) > 0))
                .OrderByDescending(item => item.Version)
                .Select(item => item.Text));
        }
        catch (RegexMatchTimeoutException)
        {
            return string.Empty;
        }
    }

    private static string Sanitize(string source)
    {
        const RegexOptions options = RegexOptions.CultureInvariant |
            RegexOptions.NonBacktracking;
        var timeout = TimeSpan.FromMilliseconds(250);
        var clean = Regex.Replace(source, @"<!--[\s\S]*?-->",
            string.Empty, options, timeout);
        clean = Regex.Replace(clean, @"!?\[([^\x5D\r\n]*)\]\([^)]*\)",
            "$1", options, timeout);
        clean = Regex.Replace(clean, @"<[^>\n]*>",
            string.Empty, options, timeout);
        return clean.Trim();
    }
}
