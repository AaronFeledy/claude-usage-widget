using System.Drawing;

namespace ClaudeUsageWidget.TrayIcon;

public enum ApiIconKind
{
    Unauthorized,
    Malformed,
    Error
}

public static partial class IconGenerator
{
    public static Icon GenerateApiStateIcon(string providerName = "Claude", ApiIconKind kind = ApiIconKind.Error, bool includeBadge = true)
    {
        using var bitmap = new Bitmap(IconSize, IconSize);
        using var graphics = Graphics.FromImage(bitmap);
        graphics.SmoothingMode = System.Drawing.Drawing2D.SmoothingMode.None;
        graphics.Clear(BackgroundColor);

        var color = kind switch
        {
            ApiIconKind.Unauthorized => ColorTranslator.FromHtml("#B388FF"),
            ApiIconKind.Malformed => ColorTranslator.FromHtml("#FFB74D"),
            _ => ColorTranslator.FromHtml("#E0E0E0")
        };
        using var borderPen = new Pen(color, BorderWidth);
        graphics.DrawRectangle(borderPen, 0, 0, IconSize - 1, IconSize - 1);
        using var markerPen = new Pen(color, 2);
        graphics.DrawLine(markerPen, 4, 4, 11, 4);
        graphics.DrawLine(markerPen, 4, 8, 11, 8);
        graphics.DrawLine(markerPen, 4, 12, 8, 12);

        if (includeBadge)
            DrawProviderBadge(graphics, providerName);

        return Icon.FromHandle(bitmap.GetHicon());
    }
}
