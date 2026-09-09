using System.Drawing;

namespace ClaudeUsageWidget.UI;

public partial class UsagePopup
{
    private bool _draggingProvider;
    private ProviderUsagePanel? _dropTarget;
    private bool _dropAfter;

    private void EnableProviderReordering()
    {
        _providerList.AllowDrop = true;
        _providerList.DragOver += ProviderDragOver;
        _providerList.DragDrop += ProviderDragDrop;
        _providerList.DragLeave += (_, _) => ClearDropIndicator();
        _providerList.Paint += (_, e) =>
        {
            if (_dropTarget == null) return;
            using var pen = new Pen(Color.FromArgb(217, 119, 87), 3);
            var y = _dropAfter ? _dropTarget.Bottom + 3 : Math.Max(1, _dropTarget.Top - 3);
            e.Graphics.DrawLine(pen, 0, y, _providerList.ClientSize.Width, y);
        };
        foreach (var panel in _providerList.Controls.OfType<ProviderUsagePanel>())
        {
            panel.ReorderRequested += (_, _) =>
            {
                _draggingProvider = true;
                try { panel.DoDragDrop(panel, DragDropEffects.Move); }
                finally { _draggingProvider = false; ClearDropIndicator(); }
            };
            WireDropSurface(panel);
        }
    }

    private void WireDropSurface(Control control)
    {
        control.AllowDrop = true;
        control.DragOver += ProviderDragOver;
        control.DragDrop += ProviderDragDrop;
        foreach (Control child in control.Controls) WireDropSurface(child);
        control.ControlAdded += (_, e) => { if (e.Control != null) WireDropSurface(e.Control); };
    }

    private void ProviderDragOver(object? sender, DragEventArgs e)
    {
        if (e.Data?.GetData(typeof(ProviderUsagePanel)) is not ProviderUsagePanel source
            || source.Parent != _providerList)
        {
            e.Effect = DragDropEffects.None;
            return;
        }
        e.Effect = DragDropEffects.Move;
        var point = _providerList.PointToClient(new Point(e.X, e.Y));
        var panels = _providerList.Controls.OfType<ProviderUsagePanel>().OrderBy(p => p.Top).ToArray();
        _dropTarget = panels.FirstOrDefault(p => point.Y < p.Bottom) ?? panels.Last();
        _dropAfter = point.Y >= _dropTarget.Top + _dropTarget.Height / 2;
        // Scroll tall provider lists when dragging near an edge.
        if (_providerList.AutoScroll)
        {
            var current = -_providerList.AutoScrollPosition.Y;
            if (point.Y < 24) _providerList.AutoScrollPosition = new Point(0, Math.Max(0, current - 12));
            else if (point.Y > _providerList.ClientSize.Height - 24) _providerList.AutoScrollPosition = new Point(0, current + 12);
        }
        _providerList.Invalidate();
    }

    private void ProviderDragDrop(object? sender, DragEventArgs e)
    {
        ProviderDragOver(sender, e);
        if (_settingsService == null || _dropTarget == null
            || e.Data?.GetData(typeof(ProviderUsagePanel)) is not ProviderUsagePanel source
            || source == _dropTarget || source.Parent != _providerList) return;
        var order = _settingsService.Settings.ProviderOrder.ToList();
        order.Remove(source.ProviderName);
        var index = order.IndexOf(_dropTarget.ProviderName) + (_dropAfter ? 1 : 0);
        order.Insert(index, source.ProviderName);
        _settingsService.SetProviderOrder(order);
        ApplyProviderOrder();
        LoadSettingsToControls();
        OnSettingsChanged?.Invoke(this, EventArgs.Empty);
        ClearDropIndicator();
    }

    private void ClearDropIndicator()
    {
        _dropTarget = null;
        _providerList.Invalidate();
    }
}
