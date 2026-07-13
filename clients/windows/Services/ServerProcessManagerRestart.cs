namespace ClaudeUsageWidget.Services;

public sealed partial class ServerProcessManager
{
    private void OnOwnedProcessExited(object? sender, EventArgs args)
    {
        if (sender is not IManagedServerProcess process || _disposed.IsCancellationRequested)
        {
            return;
        }
        _monitor = RestartAfterExitAsync(process, _generation, _disposed.Token);
    }

    private async Task RestartAfterExitAsync(IManagedServerProcess exitedProcess, int generation, CancellationToken cancellationToken)
    {
        try
        {
            if (_options.MaxRestartAttempts <= 0 || cancellationToken.IsCancellationRequested)
            {
                return;
            }
            await _dependencies.Delay.DelayAsync(Backoff(0), cancellationToken).ConfigureAwait(false);
            await _lifecycle.WaitAsync(cancellationToken).ConfigureAwait(false);
            try
            {
                if (!ReferenceEquals(_process, exitedProcess) || generation != _generation)
                {
                    return;
                }
                await DisposeOwnedProcessAsync(cancellationToken).ConfigureAwait(false);
                _started = null;
                var result = await StartOwnedAsync(cancellationToken).ConfigureAwait(false);
                if (result.State == ServerProcessState.Started)
                {
                    return;
                }
                SetRestartFailed();
            }
            finally
            {
                _lifecycle.Release();
            }
        }
        catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
        {
        }
        catch
        {
            await RecordRestartFailedAsync().ConfigureAwait(false);
        }
    }

    private void SetRestartFailed()
    {
        _started = new ServerProcessResult(ServerProcessState.Failed, EffectiveBaseUrl, false, ServerProcessError.RestartFailed, "Automatic local server restart failed.");
    }

    private async Task RecordRestartFailedAsync()
    {
        await _lifecycle.WaitAsync().ConfigureAwait(false);
        try
        {
            SetRestartFailed();
        }
        finally
        {
            _lifecycle.Release();
        }
    }
}
