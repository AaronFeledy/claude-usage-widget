using ClaudeUsageWidget.TrayIcon;

internal sealed class IconOwnershipTests
{
    public Task Test_NativeIconLeaseDisposesHandleExactlyOnce()
    {
        var destroyed = 0;
        using var lease = new NativeIconLease(new IntPtr(42), handle =>
        {
            if (handle != new IntPtr(42)) throw new InvalidOperationException("wrong handle");
            destroyed++;
        });

        lease.Dispose();
        lease.Dispose();

        AssertEqual(1, destroyed);
        return Task.CompletedTask;
    }

    private static void AssertEqual<T>(T expected, T actual)
    {
        if (!EqualityComparer<T>.Default.Equals(expected, actual))
        {
            throw new InvalidOperationException($"Expected {expected}, got {actual}");
        }
    }
}
