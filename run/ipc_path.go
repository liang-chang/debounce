//go:build linux || darwin

package run

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

// Shared with Linux tests; Linux's production endpoint remains an abstract socket.
type pathListener struct {
	*net.UnixListener
	lock *os.File
	once sync.Once
	err  error
}

func (l *pathListener) Close() error {
	l.once.Do(func() {
		// Unlink the socket before releasing ownership, never the lock file:
		// replacing that inode would let two coordinators acquire different locks.
		l.err = errors.Join(l.UnixListener.Close(), l.lock.Close())
	})
	return l.err
}

func checkEndpointDirectory(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	var stat unix.Stat_t
	if err := unix.Lstat(dir, &stat); err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm() != 0700 || stat.Uid != uint32(os.Getuid()) {
		return fmt.Errorf("IPC directory must belong to the current user with mode 0700: %s", dir)
	}
	return nil
}

func listenPathEndpoint(name string) (net.Listener, error) {
	dir := filepath.Dir(name)
	if err := os.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	if err := checkEndpointDirectory(dir); err != nil {
		return nil, err
	}
	// CLOEXEC prevents the executed command from retaining the coordinator's lock.
	fd, err := unix.Open(name+".lock", unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	lock := os.NewFile(uintptr(fd), name+".lock")
	owned := false
	defer func() {
		if !owned {
			_ = lock.Close()
		}
	}()
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return nil, err
	}
	// Only the lock owner may remove a stale socket, including after a crash.
	if info, err := os.Lstat(name); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("IPC endpoint is not a socket: %s", name)
		}
		if err := os.Remove(name); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: name, Net: "unix"})
	if err != nil {
		return nil, err
	}
	listener.SetUnlinkOnClose(true)
	owned = true
	return &pathListener{UnixListener: listener, lock: lock}, nil
}
