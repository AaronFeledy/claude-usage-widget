using System.Drawing;

namespace ClaudeUsageWidget.TrayIcon;

/// <summary>Bundled official provider favicons. Loaded once, shared for the process lifetime.</summary>
internal static class ProviderIcons
{
    private static readonly Dictionary<string, Bitmap> Images = LoadImages();

    public static Bitmap? Get(string providerName) => Images.GetValueOrDefault(providerName);

    private static Dictionary<string, Bitmap> LoadImages()
    {
        var images = new Dictionary<string, Bitmap>(StringComparer.OrdinalIgnoreCase);
        foreach (var name in new[] { "Claude", "Codex", "Cursor", "Grok" })
        {
            using var stream = typeof(ProviderIcons).Assembly.GetManifestResourceStream($"ProviderIcons.{name.ToLowerInvariant()}.png");
            if (stream == null) continue;
            using var source = Image.FromStream(stream);
            images[name] = new Bitmap(source);
        }
        return images;
    }
}
