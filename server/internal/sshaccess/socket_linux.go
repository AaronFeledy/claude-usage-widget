//go:build linux

package sshaccess

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const socketRelativePath = ".local/share/headroom/ssh/control.sock"

type socketIdentity struct {
	device uint64
	inode  uint64
}

type ownedListener struct {
	*net.UnixListener
	path     string
	identity socketIdentity
	uid      uint32
}

func ValidatePlatform() error { return nil }

func SocketPath(home string) string {
	return filepath.Join(home, filepath.FromSlash(socketRelativePath))
}

// Listen creates the private fixed socket and only admits connections from the
// account that owns the running server.
func Listen(home string) (net.Listener, error) {
	uid := uint32(os.Geteuid())
	sshDir, err := prepareDirectories(home, uid)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(sshDir, "control.sock")
	if err := recoverStaleSocket(path, uid); err != nil {
		return nil, err
	}
	address, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, fmt.Errorf("resolve SSH socket: %w", err)
	}
	listener, err := net.ListenUnix("unix", address)
	if err != nil {
		return nil, fmt.Errorf("listen on SSH socket: %w", err)
	}
	listener.SetUnlinkOnClose(false)
	fail := func(err error) (net.Listener, error) {
		listener.Close()
		os.Remove(path)
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fail(fmt.Errorf("secure SSH socket: %w", err))
	}
	info, err := validateSocket(path, uid)
	if err != nil {
		return fail(err)
	}
	identity, ok := fileIdentity(info)
	if !ok {
		return fail(fmt.Errorf("inspect SSH socket identity"))
	}
	return &ownedListener{UnixListener: listener, path: path, identity: identity, uid: uid}, nil
}

func (listener *ownedListener) Accept() (net.Conn, error) {
	for {
		connection, err := listener.AcceptUnix()
		if err != nil {
			return nil, err
		}
		if trustedPeer(connection, listener.uid, peerUID) {
			return connection, nil
		}
		connection.Close()
	}
}

func trustedPeer(connection *net.UnixConn, expected uint32, lookup func(*net.UnixConn) (uint32, error)) bool {
	uid, err := lookup(connection)
	return err == nil && uid == expected
}

func (listener *ownedListener) Close() error {
	err := listener.UnixListener.Close()
	removeIfIdentity(listener.path, listener.identity)
	return err
}

func dialTrustedSocket(ctx context.Context, home string) (net.Conn, error) {
	uid := uint32(os.Geteuid())
	if _, err := validateTrustedPath(home, uid, false); err != nil {
		return nil, err
	}
	path := SocketPath(home)
	before, err := validateSocket(path, uid)
	if err != nil {
		return nil, err
	}
	identity, ok := fileIdentity(before)
	if !ok {
		return nil, errors.New("invalid SSH socket identity")
	}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	connection, err := dialer.DialContext(ctx, "unix", path)
	if err != nil {
		return nil, err
	}
	unixConnection, ok := connection.(*net.UnixConn)
	if !ok {
		connection.Close()
		return nil, errors.New("invalid SSH socket connection")
	}
	peer, err := peerUID(unixConnection)
	if err != nil || peer != uid {
		connection.Close()
		return nil, errors.New("untrusted SSH socket peer")
	}
	after, err := validateSocket(path, uid)
	afterIdentity, same := fileIdentity(after)
	if err != nil || !same || afterIdentity != identity {
		connection.Close()
		return nil, errors.New("SSH socket changed during connection")
	}
	return connection, nil
}

func prepareDirectories(home string, uid uint32) (string, error) {
	home = filepath.Clean(home)
	if !filepath.IsAbs(home) {
		return "", errors.New("SSH access home must be absolute")
	}
	if err := validateAncestry(home, uid); err != nil {
		return "", fmt.Errorf("untrusted SSH access home ancestry: %w", err)
	}
	if _, err := validateDirectory(home, uid); err != nil {
		return "", fmt.Errorf("untrusted SSH access home: %w", err)
	}
	current := home
	for _, component := range []string{".local", "share", "headroom", "ssh"} {
		current = filepath.Join(current, component)
		if err := os.Mkdir(current, 0o700); err != nil && !os.IsExist(err) {
			return "", fmt.Errorf("create SSH access directory: %w", err)
		}
		if _, err := validateDirectory(current, uid); err != nil {
			return "", fmt.Errorf("untrusted SSH access directory: %w", err)
		}
	}
	if err := os.Chmod(current, 0o700); err != nil {
		return "", fmt.Errorf("secure SSH access directory: %w", err)
	}
	info, err := validateDirectory(current, uid)
	if err != nil || info.Mode().Perm() != 0o700 {
		return "", errors.New("SSH access directory permissions are invalid")
	}
	return current, nil
}

func validateTrustedPath(home string, uid uint32, includeSocket bool) (os.FileInfo, error) {
	home = filepath.Clean(home)
	if !filepath.IsAbs(home) {
		return nil, errors.New("SSH access home must be absolute")
	}
	if err := validateAncestry(home, uid); err != nil {
		return nil, err
	}
	current := home
	if _, err := validateDirectory(current, uid); err != nil {
		return nil, err
	}
	for _, component := range []string{".local", "share", "headroom", "ssh"} {
		current = filepath.Join(current, component)
		if _, err := validateDirectory(current, uid); err != nil {
			return nil, err
		}
	}
	sshInfo, err := os.Lstat(current)
	if err != nil || sshInfo.Mode().Perm() != 0o700 {
		return nil, errors.New("SSH access directory permissions are invalid")
	}
	if includeSocket {
		return validateSocket(filepath.Join(current, "control.sock"), uid)
	}
	return os.Lstat(current)
}

func validateAncestry(path string, uid uint32) error {
	volume := filepath.VolumeName(path)
	current := string(os.PathSeparator)
	if volume != "" {
		current = volume + string(os.PathSeparator)
	}
	relative := strings.TrimPrefix(filepath.Clean(path), current)
	for _, component := range strings.Split(relative, string(os.PathSeparator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("home ancestry is not made of real directories")
		}
		owner := fileUID(info)
		if owner != 0 && owner != uid {
			return errors.New("home ancestry has an untrusted owner")
		}
		writable := info.Mode().Perm()&0o022 != 0
		stickyRoot := owner == 0 && info.Mode()&os.ModeSticky != 0
		if writable && !stickyRoot {
			return errors.New("home ancestry is group or other writable")
		}
	}
	return nil
}

func validateDirectory(path string, uid uint32) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("path is not a real directory")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return nil, errors.New("directory is group or other writable")
	}
	if fileUID(info) != uid {
		return nil, errors.New("directory has a different owner")
	}
	return info, nil
}

func validateSocket(path string, uid uint32) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeSocket == 0 {
		return nil, errors.New("SSH endpoint is not a socket")
	}
	if info.Mode().Perm() != 0o600 || fileUID(info) != uid {
		return nil, errors.New("SSH socket permissions or owner are invalid")
	}
	return info, nil
}

func recoverStaleSocket(path string, uid uint32) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect SSH socket: %w", err)
	}
	if _, err := validateSocket(path, uid); err != nil {
		return fmt.Errorf("refuse unsafe SSH socket collision: %w", err)
	}
	identity, ok := fileIdentity(info)
	if !ok {
		return errors.New("inspect SSH socket identity")
	}
	connection, dialErr := net.DialTimeout("unix", path, 500*time.Millisecond)
	if dialErr == nil {
		connection.Close()
		return errors.New("SSH access socket is already active")
	}
	if !errors.Is(dialErr, syscall.ECONNREFUSED) {
		return fmt.Errorf("SSH access socket collision: %w", dialErr)
	}
	current, err := os.Lstat(path)
	currentIdentity, valid := fileIdentity(current)
	if err != nil || !valid || currentIdentity != identity {
		return errors.New("SSH access socket changed during recovery")
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove stale SSH socket: %w", err)
	}
	return nil
}

func removeIfIdentity(path string, identity socketIdentity) {
	info, err := os.Lstat(path)
	current, ok := fileIdentity(info)
	if err == nil && ok && current == identity {
		_ = os.Remove(path)
	}
}

func fileUID(info os.FileInfo) uint32 {
	if info == nil {
		return ^uint32(0)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ^uint32(0)
	}
	return stat.Uid
}

func fileIdentity(info os.FileInfo) (socketIdentity, bool) {
	if info == nil {
		return socketIdentity{}, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return socketIdentity{}, false
	}
	return socketIdentity{device: uint64(stat.Dev), inode: stat.Ino}, true
}

func peerUID(connection *net.UnixConn) (uint32, error) {
	raw, err := connection.SyscallConn()
	if err != nil {
		return 0, err
	}
	var credential *syscall.Ucred
	var controlErr error
	if err := raw.Control(func(fd uintptr) {
		credential, controlErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return 0, err
	}
	if controlErr != nil {
		return 0, controlErr
	}
	if credential == nil {
		return 0, errors.New("missing SSH socket peer credentials")
	}
	return credential.Uid, nil
}
