using System.Drawing;
using System.Drawing.Drawing2D;

namespace ClaudeUsageWidget.TrayIcon;

/// <summary>
/// Generates dynamic 16x16 tray icons based on usage metrics.
/// </summary>
public static class IconGenerator
{
    // Fill bar colors based on utilization level
    private static readonly Color GreenFill = ColorTranslator.FromHtml("#4CAF50");
    private static readonly Color YellowFill = ColorTranslator.FromHtml("#FFC107");
    private static readonly Color OrangeFill = ColorTranslator.FromHtml("#FF9800");
    private static readonly Color RedFill = ColorTranslator.FromHtml("#F44336");

    // Background and border colors
    private static readonly Color BackgroundColor = ColorTranslator.FromHtml("#333333");
    private static readonly Color BorderColor = ColorTranslator.FromHtml("#555555");
    private static readonly Color DotBorderColor = ColorTranslator.FromHtml("#222222");

    private const int IconSize = 16;
    private const int BorderWidth = 1;
    private const int Padding = 1;

    /// <summary>
    /// Generates a 16x16 tray icon based on usage utilization levels.
    /// </summary>
    /// <param name="currentUtilization">Current session utilization percentage (0-100)</param>
    /// <param name="weeklyUtilization">weekly utilization percentage (0-100)</param>
    /// <returns>A new Icon instance. Caller is responsible for disposing.</returns>
    public static Icon GenerateIcon(float currentUtilization, float weeklyUtilization)
    {
        using var bitmap = new Bitmap(IconSize, IconSize);
        using var graphics = Graphics.FromImage(bitmap);

        // Use no anti-aliasing for crisp pixel edges at 16x16
        graphics.SmoothingMode = SmoothingMode.None;
        graphics.PixelOffsetMode = PixelOffsetMode.None;
        graphics.InterpolationMode = InterpolationMode.NearestNeighbor;

        // Check for rate-limited state (>= 99%)
        if (currentUtilization >= 99f)
        {
            DrawRateLimitedIcon(graphics);
        }
        else
        {
            DrawNormalIcon(graphics, currentUtilization);
        }

        // Draw weekly overlay if needed
        if (weeklyUtilization > 70f)
        {
            DrawWeeklyOverlay(graphics, weeklyUtilization);
        }

        return Icon.FromHandle(bitmap.GetHicon());
    }

    /// <summary>
    /// Generates a gray placeholder icon for loading/error states.
    /// </summary>
    public static Icon GeneratePlaceholderIcon()
    {
        using var bitmap = new Bitmap(IconSize, IconSize);
        using var graphics = Graphics.FromImage(bitmap);

        graphics.SmoothingMode = SmoothingMode.None;

        // Fill with dark background
        graphics.Clear(BackgroundColor);

        // Draw border
        using var borderPen = new Pen(BorderColor, BorderWidth);
        graphics.DrawRectangle(borderPen, 0, 0, IconSize - 1, IconSize - 1);

        // Draw a "?" in the center for loading state
        using var font = new Font("Consolas", 9f, FontStyle.Bold, GraphicsUnit.Pixel);
        using var brush = new SolidBrush(Color.Gray);
        var textSize = graphics.MeasureString("?", font);
        var x = (IconSize - textSize.Width) / 2;
        var y = (IconSize - textSize.Height) / 2;
        graphics.DrawString("?", font, brush, x, y);

        return Icon.FromHandle(bitmap.GetHicon());
    }

    /// <summary>
    /// Generates an error state icon.
    /// </summary>
    public static Icon GenerateErrorIcon()
    {
        using var bitmap = new Bitmap(IconSize, IconSize);
        using var graphics = Graphics.FromImage(bitmap);

        graphics.SmoothingMode = SmoothingMode.None;

        // Fill with dark background
        graphics.Clear(BackgroundColor);

        // Draw border in red
        using var borderPen = new Pen(RedFill, BorderWidth);
        graphics.DrawRectangle(borderPen, 0, 0, IconSize - 1, IconSize - 1);

        // Draw an "X" in the center
        using var xPen = new Pen(RedFill, 2);
        graphics.DrawLine(xPen, 4, 4, 11, 11);
        graphics.DrawLine(xPen, 11, 4, 4, 11);

        return Icon.FromHandle(bitmap.GetHicon());
    }

    /// <summary>
    /// Draws the normal fill bar icon.
    /// </summary>
    private static void DrawNormalIcon(Graphics graphics, float utilization)
    {
        // Fill entire icon with background
        graphics.Clear(BackgroundColor);

        // Draw outer border
        using var borderPen = new Pen(BorderColor, BorderWidth);
        graphics.DrawRectangle(borderPen, 0, 0, IconSize - 1, IconSize - 1);

        // Calculate fill bar dimensions (inside border + padding)
        // Border is 1px, padding is 1px, so fill starts at x=2, y=2
        var fillStartX = BorderWidth + Padding;
        var fillStartY = BorderWidth + Padding;
        var fillWidth = IconSize - (2 * (BorderWidth + Padding));
        var fillMaxHeight = IconSize - (2 * (BorderWidth + Padding));

        // Calculate fill height based on utilization (fills from bottom to top)
        var fillHeight = (int)Math.Round(fillMaxHeight * Math.Clamp(utilization, 0f, 100f) / 100f);

        if (fillHeight > 0)
        {
            // Get fill color based on utilization level
            var fillColor = GetFillColor(utilization);

            // Fill from bottom to top
            var fillY = fillStartY + (fillMaxHeight - fillHeight);

            using var fillBrush = new SolidBrush(fillColor);
            graphics.FillRectangle(fillBrush, fillStartX, fillY, fillWidth, fillHeight);
        }
    }

    /// <summary>
    /// Draws the rate-limited state icon (red background with white "!").
    /// </summary>
    private static void DrawRateLimitedIcon(Graphics graphics)
    {
        // Fill entire icon with red
        graphics.Clear(RedFill);

        // Draw darker border for contrast
        using var borderPen = new Pen(DotBorderColor, BorderWidth);
        graphics.DrawRectangle(borderPen, 0, 0, IconSize - 1, IconSize - 1);

        // Draw white "!" in the center
        using var font = new Font("Consolas", 10f, FontStyle.Bold, GraphicsUnit.Pixel);
        using var brush = new SolidBrush(Color.White);

        // Center the exclamation mark
        var textSize = graphics.MeasureString("!", font);
        var x = (IconSize - textSize.Width) / 2;
        var y = (IconSize - textSize.Height) / 2 - 1; // Slight adjustment for visual centering
        graphics.DrawString("!", font, brush, x, y);
    }

    /// <summary>
    /// Draws the weekly utilization overlay dot in the top-right corner.
    /// </summary>
    private static void DrawWeeklyOverlay(Graphics graphics, float weeklyUtilization)
    {
        // Dot is 4x4, positioned in top-right corner with 1px margin
        const int dotSize = 4;
        const int dotX = IconSize - dotSize - 1;
        const int dotY = 1;

        // Choose dot color based on weekly utilization
        var dotColor = weeklyUtilization >= 85f ? RedFill : YellowFill;

        // Draw dot border first (1px dark border)
        using var borderBrush = new SolidBrush(DotBorderColor);
        graphics.FillRectangle(borderBrush, dotX - 1, dotY - 1, dotSize + 2, dotSize + 2);

        // Draw the colored dot
        using var dotBrush = new SolidBrush(dotColor);
        graphics.FillRectangle(dotBrush, dotX, dotY, dotSize, dotSize);
    }

    /// <summary>
    /// Gets the fill color based on utilization level.
    /// </summary>
    private static Color GetFillColor(float utilization)
    {
        return utilization switch
        {
            >= 90f => RedFill,
            >= 75f => OrangeFill,
            >= 50f => YellowFill,
            _ => GreenFill
        };
    }
}
