using System.Drawing;
using System.Runtime.InteropServices;

namespace ClaudeUsageWidget.TrayIcon;

public sealed class OwnedGeneratedIcon : IDisposable
{
    private readonly NativeIconLease _lease;
    private int _disposed;

    public OwnedGeneratedIcon(Icon icon)
    {
        Icon = icon;
        _lease = new NativeIconLease(icon.Handle, handle => DestroyIcon(handle));
    }

    public Icon Icon { get; }

    public void Dispose()
    {
        if (Interlocked.Exchange(ref _disposed, 1) != 0)
        {
            return;
        }
        _lease.Dispose();
        Icon.Dispose();
    }

    [DllImport("user32.dll", CharSet = CharSet.Auto)]
    private static extern bool DestroyIcon(IntPtr handle);
}
