using System.Diagnostics;

namespace ClaudeUsageWidget.Services;

internal static class WslPathResolver
{
    public static string? ResolveHomeRelativePath(params string[] segments)
    {
        var info = TryGetDefaultWslInfo(includeCodexHome: false);
        if (info == null || string.IsNullOrWhiteSpace(info.HomePath))
            return null;

        return CombineUncPath(info.DistroName, info.HomePath, segments);
    }

    public static string? ResolveCodexAuthPath()
    {
        var info = TryGetDefaultWslInfo(includeCodexHome: true);
        if (info == null)
            return null;

        if (!string.IsNullOrWhiteSpace(info.CodexHomePath))
            return CombineUncPath(info.DistroName, info.CodexHomePath, "auth.json");

        if (!string.IsNullOrWhiteSpace(info.HomePath))
            return CombineUncPath(info.DistroName, info.HomePath, ".codex", "auth.json");

        return null;
    }

    public static string? ResolveOpenCodeAuthPath()
    {
        return ResolveHomeRelativePath(".local", "share", "opencode", "auth.json");
    }

    private static WslInfo? TryGetDefaultWslInfo(bool includeCodexHome)
    {
        try
        {
            using var process = new Process
            {
                StartInfo = new ProcessStartInfo
                {
                    FileName = "wsl.exe",
                    RedirectStandardOutput = true,
                    RedirectStandardError = true,
                    UseShellExecute = false,
                    CreateNoWindow = true
                }
            };

            process.StartInfo.ArgumentList.Add("sh");
            process.StartInfo.ArgumentList.Add("-lc");
            process.StartInfo.ArgumentList.Add(includeCodexHome
                ? "printf '%s\n%s\n%s' \"$WSL_DISTRO_NAME\" \"$HOME\" \"$CODEX_HOME\""
                : "printf '%s\n%s' \"$WSL_DISTRO_NAME\" \"$HOME\"");

            process.Start();
            var output = process.StandardOutput.ReadToEnd();
            process.WaitForExit(3000);

            if (process.ExitCode != 0)
                return null;

            var lines = output.Replace("\r", string.Empty)
                .Split('\n', StringSplitOptions.None);

            if (lines.Length < 2 || string.IsNullOrWhiteSpace(lines[0]) || string.IsNullOrWhiteSpace(lines[1]))
                return null;

            return new WslInfo(lines[0].Trim(), lines[1].Trim(), includeCodexHome && lines.Length > 2 ? lines[2].Trim() : null);
        }
        catch
        {
            return null;
        }
    }

    private static string CombineUncPath(string distroName, string linuxBasePath, params string[] segments)
    {
        var normalizedBasePath = linuxBasePath.Replace('/', '\\').TrimEnd('\\');
        if (!normalizedBasePath.StartsWith('\\'))
            normalizedBasePath = "\\" + normalizedBasePath;

        var basePath = $@"\\wsl.localhost\{distroName}{normalizedBasePath}";
        return Path.Combine(new[] { basePath }.Concat(segments).ToArray());
    }

    private sealed record WslInfo(string DistroName, string HomePath, string? CodexHomePath);
}
