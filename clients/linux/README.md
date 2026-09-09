# Headroom desktop source moved

The shared Windows and Linux Qt client now lives in [`../desktop`](../desktop/README.md).
Existing CMake commands that use `clients/linux` remain supported by the small
compatibility wrapper in this directory. New development should configure
`clients/desktop` directly.
