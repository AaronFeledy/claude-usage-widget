using System.Drawing;

namespace ClaudeUsageWidget.UI;

public partial class UsagePopup
{
    public void PositionNearTray()
    {
        var cursorPos = Cursor.Position;
        var workingArea = Screen.FromPoint(cursorPos).WorkingArea;

        var x = cursorPos.X - Width / 2;
        if (x + Width > workingArea.Right)
            x = workingArea.Right - Width - 8;
        if (x < workingArea.Left)
            x = workingArea.Left + 8;

        var y = cursorPos.Y > workingArea.Top + workingArea.Height / 2
            ? workingArea.Bottom - Height - 8
            : workingArea.Top + 8;

        Location = new Point(x, y);
    }

    private void RecalculateLayout()
    {
        _providerList.Location = new Point(12, 44);
        _settingsPanel.Location = new Point(0, _providerList.Bottom + 8);
        _collapsedHeight = _providerList.Bottom + 12;
        Height = _settingsExpanded ? _collapsedHeight + _settingsPanel.Height : _collapsedHeight;
    }

    protected override void OnPaint(PaintEventArgs e)
    {
        base.OnPaint(e);

        using var pen = new Pen(SeparatorColor, 1);
        e.Graphics.DrawRectangle(pen, 0, 0, Width - 1, Height - 1);
    }

    protected override CreateParams CreateParams
    {
        get
        {
            var cp = base.CreateParams;
            cp.ClassStyle |= 0x00020000;
            return cp;
        }
    }

    protected override void Dispose(bool disposing)
    {
        if (disposing)
        {
            _titleIconBitmap.Dispose();
            _titleIcon.Dispose();
        }
        base.Dispose(disposing);
    }
}
