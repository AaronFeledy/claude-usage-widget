using System.Drawing;
using System.Drawing.Drawing2D;

namespace ClaudeUsageWidget.TrayIcon;

public static partial class IconGenerator
{
    private static void DrawProviderBadge(Graphics graphics, string providerName)
    {
        var image = ProviderIcons.Get(providerName);
        if (image == null) return;
        var state = graphics.Save();
        try
        {
            // A real provider mark, rasterized from its official favicon.
            var badgeRect = new Rectangle(8, 8, 8, 8);
            using var background = new SolidBrush(Color.FromArgb(245, 18, 18, 18));
            graphics.FillRectangle(background, badgeRect);
            graphics.InterpolationMode = InterpolationMode.HighQualityBicubic;
            graphics.PixelOffsetMode = PixelOffsetMode.HighQuality;
            graphics.DrawImage(image, badgeRect);
        }
        finally { graphics.Restore(state); }
    }
}
