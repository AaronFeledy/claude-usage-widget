using System.Drawing;
using System.Drawing.Drawing2D;

namespace ClaudeUsageWidget.UI;

public class UsageProgressBar : Panel
{
    private const int CornerRadius = 6;
    private readonly ToolTip _toolTip;
    private float _value;
    private float _burnRatePercent = -1;
    private int _notches;
    private bool _tooltipVisible;

    public UsageProgressBar()
    {
        DoubleBuffered = true;
        Height = 16;
        _toolTip = new ToolTip
        {
            InitialDelay = 0,
            ReshowDelay = 0,
            AutoPopDelay = 5000,
            BackColor = Color.FromArgb(40, 40, 40),
            ForeColor = Color.White,
            OwnerDraw = false
        };
        MouseMove += OnBarMouseMove;
        MouseLeave += (_, _) =>
        {
            _toolTip.Hide(this);
            _tooltipVisible = false;
        };
    }

    public int Notches
    {
        get => _notches;
        set
        {
            _notches = Math.Max(0, value);
            Invalidate();
        }
    }

    public float Value
    {
        get => _value;
        set
        {
            _value = Math.Clamp(value, 0, 100);
            Invalidate();
        }
    }

    public float BurnRatePercent
    {
        get => _burnRatePercent;
        set
        {
            _burnRatePercent = value;
            Invalidate();
        }
    }

    private void OnBarMouseMove(object? sender, MouseEventArgs e)
    {
        if (_burnRatePercent < 0 || _burnRatePercent > 100) return;
        var markerX = (int)((Width - 1) * (_burnRatePercent / 100f));
        var distance = Math.Abs(e.X - markerX);
        if (distance <= 8)
        {
            if (!_tooltipVisible)
            {
                _toolTip.Show($"{_burnRatePercent:F0}%", this, markerX - 10, -20);
                _tooltipVisible = true;
            }
        }
        else if (_tooltipVisible)
        {
            _toolTip.Hide(this);
            _tooltipVisible = false;
        }
    }

    protected override void OnPaint(PaintEventArgs e)
    {
        base.OnPaint(e);
        var g = e.Graphics;
        g.SmoothingMode = SmoothingMode.AntiAlias;
        var rect = new Rectangle(0, 0, Width - 1, Height - 1);
        using (var bgBrush = new SolidBrush(Color.FromArgb(60, 60, 60)))
        using (var path = CreateRoundedRectPath(rect, CornerRadius))
        {
            g.FillPath(bgBrush, path);
        }
        DrawNotches(g);
        DrawFill(g);
        using (var borderPen = new Pen(Color.FromArgb(80, 80, 80), 1))
        using (var borderPath = CreateRoundedRectPath(rect, CornerRadius))
        {
            g.DrawPath(borderPen, borderPath);
        }
        DrawBurnRateMarker(g);
    }

    private void DrawNotches(Graphics g)
    {
        if (_notches <= 0) return;
        using var notchPen = new Pen(Color.FromArgb(40, 255, 255, 255), 1);
        for (var i = 1; i <= _notches; i++)
        {
            var notchX = (int)((Width - 1) * ((float)i / (_notches + 1)));
            g.DrawLine(notchPen, notchX, 2, notchX, Height - 3);
        }
    }

    private void DrawFill(Graphics g)
    {
        if (_value <= 0) return;
        var fillWidth = (int)((Width - 1) * (_value / 100f));
        if (fillWidth <= 0) return;
        var fillRect = new Rectangle(0, 0, fillWidth, Height - 1);
        using var fillBrush = new SolidBrush(GetFillColor(_value, _burnRatePercent));
        using var fillPath = CreateRoundedRectPath(fillRect, CornerRadius, _value >= 100);
        g.FillPath(fillBrush, fillPath);
    }

    private void DrawBurnRateMarker(Graphics g)
    {
        if (_burnRatePercent < 0 || _burnRatePercent > 100) return;
        var markerX = (int)((Width - 1) * (_burnRatePercent / 100f));
        markerX = Math.Clamp(markerX, 2, Width - 3);
        var markerColor = _value > _burnRatePercent ? Color.FromArgb(200, 244, 67, 54) : Color.FromArgb(200, 80, 160, 255);
        using var markerPen = new Pen(markerColor, 2);
        g.DrawLine(markerPen, markerX, 1, markerX, Height - 2);
    }

    private static Color GetFillColor(float utilization, float burnRatePercent = -1)
    {
        if (burnRatePercent >= 0 && utilization < burnRatePercent)
        {
            return utilization switch
            {
                >= 95 => Color.FromArgb(244, 67, 54),
                >= 90 => Color.FromArgb(255, 152, 0),
                >= 85 => Color.FromArgb(255, 193, 7),
                _ => Color.FromArgb(76, 175, 80)
            };
        }
        return utilization switch
        {
            >= 90 => Color.FromArgb(244, 67, 54),
            >= 75 => Color.FromArgb(255, 152, 0),
            >= 50 => Color.FromArgb(255, 193, 7),
            _ => Color.FromArgb(76, 175, 80)
        };
    }

    private static GraphicsPath CreateRoundedRectPath(Rectangle rect, int radius, bool fullRound = true)
    {
        var path = new GraphicsPath();
        var diameter = radius * 2;
        path.AddArc(rect.X, rect.Y, diameter, diameter, 180, 90);
        if (fullRound) path.AddArc(rect.Right - diameter, rect.Y, diameter, diameter, 270, 90);
        else path.AddLine(rect.Right, rect.Y, rect.Right, rect.Y);
        if (fullRound) path.AddArc(rect.Right - diameter, rect.Bottom - diameter, diameter, diameter, 0, 90);
        else path.AddLine(rect.Right, rect.Bottom, rect.Right, rect.Bottom);
        path.AddArc(rect.X, rect.Bottom - diameter, diameter, diameter, 90, 90);
        path.CloseFigure();
        return path;
    }
}
