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
        if (y < workingArea.Top + 8)
            y = workingArea.Top + 8;

        Location = new Point(x, y);
    }

    private void RecalculateLayout()
    {
        _providerList.Location = new Point(12, 44);

        var maxHeight = Screen.FromPoint(Cursor.Position).WorkingArea.Height - 16;
        var settingsHeight = _settingsExpanded ? _settingsPanel.Height : 0;
        var availableForList = maxHeight - _providerList.Top - 12 - settingsHeight;
        var naturalListHeight = _providerList.PreferredSize.Height;

        if (availableForList > 0 && naturalListHeight > availableForList)
        {
            _providerList.AutoSize = false;
            _providerList.AutoScroll = true;
            _providerList.Height = availableForList;
        }
        else
        {
            _providerList.AutoScroll = false;
            _providerList.AutoSize = true;
        }

        _settingsPanel.Location = new Point(0, _providerList.Bottom + 8);
        _collapsedHeight = _providerList.Bottom + 12;
        var desired = _settingsExpanded ? _collapsedHeight + _settingsPanel.Height : _collapsedHeight;
        Height = Math.Min(desired, maxHeight);
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
