namespace ClaudeUsageWidget.TrayIcon;

public sealed class NativeIconLease : IDisposable
{
    private readonly IntPtr _handle;
    private readonly Action<IntPtr> _destroy;
    private int _disposed;

    public NativeIconLease(IntPtr handle, Action<IntPtr> destroy)
    {
        _handle = handle;
        _destroy = destroy;
    }

    public void Dispose()
    {
        if (Interlocked.Exchange(ref _disposed, 1) != 0 || _handle == IntPtr.Zero)
        {
            return;
        }
        _destroy(_handle);
    }
}
