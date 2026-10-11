package run

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/sys/unix"
)

func userIdentity() (string, error) { return strconv.Itoa(os.Getuid()), nil }

func endpointName(hash string) string {
	// A fixed short parent avoids Darwin's socket path limit and per-session TMPDIRs.
	return filepath.Join("/tmp", "debounce-v2-"+strconv.Itoa(os.Getuid()), hash+".sock")
}

func listenEndpoint(name, _ string) (net.Listener, error) { return listenPathEndpoint(name) }

func dialEndpoint(ctx context.Context, name string) (net.Conn, error) {
	if err := checkEndpointDirectory(filepath.Dir(name)); err != nil {
		return nil, err
	}
	return (&net.Dialer{}).DialContext(ctx, "unix", name)
}

func retryListen(err error) bool {
	return errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EADDRINUSE)
}

func missingEndpoint(err error) bool {
	return errors.Is(err, unix.ECONNREFUSED) || errors.Is(err, os.ErrNotExist)
}

func authorizePeer(conn net.Conn) error {
	return authenticateCoordinator(conn, strconv.Itoa(os.Getuid()))
}

func authenticateCoordinator(conn net.Conn, expectedUser string) error {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return errors.New("expected Unix socket")
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return err
	}
	var credential *unix.Xucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		credential, credErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	}); err != nil {
		return err
	}
	if credErr != nil {
		return credErr
	}
	// Darwin's sys/ucred.h defines XUCRED_VERSION as 0; x/sys does not export it.
	if credential.Version != 0 || strconv.FormatUint(uint64(credential.Uid), 10) != expectedUser {
		return fmt.Errorf("IPC peer belongs to another user or has unsupported credentials")
	}
	return nil
}
