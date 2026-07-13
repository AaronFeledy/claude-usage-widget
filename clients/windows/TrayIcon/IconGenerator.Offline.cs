using System.Drawing;

namespace ClaudeUsageWidget.TrayIcon;

public static partial class IconGenerator
{
    public static Icon GenerateOfflineIcon(string providerName = "Claude", bool includeBadge = true)
    {
        using var bitmap = new Bitmap(IconSize, IconSize);
        using var graphics = Graphics.FromImage(bitmap);

        graphics.SmoothingMode = System.Drawing.Drawing2D.SmoothingMode.None;
        graphics.Clear(BackgroundColor);

        using var borderPen = new Pen(ColorTranslator.FromHtml("#64B5F6"), BorderWidth);
        graphics.DrawRectangle(borderPen, 0, 0, IconSize - 1, IconSize - 1);
        using var slashPen = new Pen(ColorTranslator.FromHtml("#64B5F6"), 2);
        graphics.DrawLine(slashPen, 4, 11, 11, 4);
        graphics.DrawLine(slashPen, 3, 7, 6, 7);
        graphics.DrawLine(slashPen, 10, 7, 13, 7);

        if (includeBadge)
            DrawProviderBadge(graphics, providerName);

        return Icon.FromHandle(bitmap.GetHicon());
    }
}
