package contract

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// StageVersion prepares the same stable release on explicitly paired installs.
// The caller supplies only a version, never an origin, URL, asset, or file path.
// Acquisition and validation are the same operations used by native updates.
func (c UpdateClient) StageVersion(ctx context.Context, installRoot, version string) (UpdateResult, error) {
	ctx, cancel := c.operationContext(ctx)
	defer cancel()
	inspection, root, err := trustedUpdateInstall(installRoot)
	if err != nil {
		return UpdateResult{}, err
	}
	CleanupAbandoned(root, time.Now().Add(-time.Hour))
	comparison, err := CompareVersions(version, inspection.Version)
	if err != nil || comparison < 0 {
		return UpdateResult{}, errors.New("paired update requires a stable version at least as new as this installation")
	}
	if version == inspection.Version && inspection.Complete {
		result := updateResult(inspection, "current", "")
		result.Version = version
		return result, nil
	}
	release, pkg, asset, err := c.resolve(ctx, version, inspection)
	if errors.Is(err, errNoCompatibleRelease) {
		return UpdateResult{}, fmt.Errorf("the matching Headroom %s package is unavailable", version)
	}
	if err != nil {
		return UpdateResult{}, err
	}
	if release.Version != version {
		return UpdateResult{}, errors.New("paired release does not match the requested version")
	}
	return c.downloadAndStage(ctx, root, inspection, release, pkg, asset)
}

// LockPairedUpdate serializes coordinators without holding the native install
// transaction lock while staging or awaiting the other operating system.
func LockPairedUpdate(installRoot string) (io.Closer, error) {
	_, root, err := trustedUpdateInstall(installRoot)
	if err != nil {
		return nil, err
	}
	parent, err := ensureOwnedDirectory(root, "pairing", 0700)
	if err != nil {
		return nil, err
	}
	if err = securePrivateDirectory(parent, true); err != nil {
		return nil, err
	}
	return acquireInstallLock(parent, 100*time.Millisecond)
}

// ImmutableManagerExecutable permits read-only progress probes without holding
// the Windows stable launcher open while another process replaces that entry.
func ImmutableManagerExecutable(installRoot string) (string, Inspection, error) {
	inspection, root, err := trustedUpdateInstall(installRoot)
	if err != nil {
		return "", inspection, err
	}
	state, err := readTrustedState(root, true)
	if err != nil {
		return "", inspection, err
	}
	manifest, err := installedManifest(root, *state)
	if err != nil {
		return "", inspection, err
	}
	name := "bundle/bin/headroom-package"
	if inspection.Platform == "windows" {
		name += ".exe"
	}
	record, ok := fileRecord(manifest.Files, name)
	path := installedComponentPath(root, state.VersionPath, name)
	if !ok || record.LinkTarget != "" || validateInstallTargets(root, path) != nil {
		return "", inspection, errors.New("immutable manager is not inventoried")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != record.Size {
		return "", inspection, errors.New("immutable manager is invalid")
	}
	digest, err := digestFile(path)
	if err != nil || digest != record.SHA256 {
		return "", inspection, errors.New("immutable manager failed verification")
	}
	return filepath.Clean(path), inspection, nil
}
