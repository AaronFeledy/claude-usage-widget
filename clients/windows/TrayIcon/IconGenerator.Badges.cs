using System.Drawing;

namespace ClaudeUsageWidget.TrayIcon;

public static partial class IconGenerator
{
    private static void DrawProviderBadge(Graphics graphics, string providerName)
    {
        var badgeRect = new Rectangle(10, 10, 6, 6);
        using var backgroundBrush = new SolidBrush(Color.FromArgb(235, 18, 18, 18));
        graphics.FillRectangle(backgroundBrush, badgeRect);
        switch (providerName)
        {
            case "Codex": DrawCodexBadge(graphics, badgeRect); break;
            case "Cursor": DrawCursorBadge(graphics, badgeRect); break;
            case "Grok": DrawGrokBadge(graphics, badgeRect); break;
            default: DrawClaudeBadge(graphics, badgeRect); break;
        }
    }

    private static void DrawClaudeBadge(Graphics graphics, Rectangle rect)
    {
        using var brush = new SolidBrush(ClaudeTerracotta);
        var cx = rect.Left + 3f;
        var cy = rect.Top + 3f;
        graphics.FillPolygon(brush,
        [
            new PointF(cx, rect.Top + 0.3f), new PointF(cx + 0.9f, cy - 0.9f),
            new PointF(rect.Right - 0.3f, cy), new PointF(cx + 0.9f, cy + 0.9f),
            new PointF(cx, rect.Bottom - 0.3f), new PointF(cx - 0.9f, cy + 0.9f),
            new PointF(rect.Left + 0.3f, cy), new PointF(cx - 0.9f, cy - 0.9f)
        ]);
    }

    private static void DrawCodexBadge(Graphics graphics, Rectangle rect)
    {
        using var aquaBrush = new SolidBrush(ColorTranslator.FromHtml("#10A37F"));
        using var darkBrush = new SolidBrush(ColorTranslator.FromHtml("#0B6B54"));
        graphics.FillRectangle(aquaBrush, rect.Left + 1, rect.Top + 1, 2, 2);
        graphics.FillRectangle(aquaBrush, rect.Left + 3, rect.Top + 3, 2, 2);
        graphics.FillRectangle(darkBrush, rect.Left + 3, rect.Top + 1, 2, 2);
        graphics.FillRectangle(darkBrush, rect.Left + 1, rect.Top + 3, 2, 2);
    }

    private static void DrawCursorBadge(Graphics graphics, Rectangle rect)
    {
        using var blackBrush = new SolidBrush(Color.Black);
        using var whiteBrush = new SolidBrush(Color.White);
        graphics.FillRectangle(whiteBrush, rect.Left + 1, rect.Top + 1, 4, 4);
        graphics.FillRectangle(blackBrush, rect.Left + 3, rect.Top + 1, 2, 4);
        graphics.FillRectangle(blackBrush, rect.Left + 1, rect.Top + 3, 4, 2);
    }

    private static void DrawGrokBadge(Graphics graphics, Rectangle rect)
    {
        using var whiteBrush = new SolidBrush(Color.White);
        graphics.FillRectangle(whiteBrush, rect.Left + 1, rect.Top + 1, 2, 4);
        graphics.FillRectangle(whiteBrush, rect.Left + 1, rect.Top + 1, 4, 1);
        graphics.FillRectangle(whiteBrush, rect.Left + 1, rect.Top + 4, 4, 1);
        graphics.FillRectangle(whiteBrush, rect.Left + 3, rect.Top + 2, 1, 2);
        graphics.FillRectangle(whiteBrush, rect.Left + 4, rect.Top + 3, 1, 1);
    }
}
