package run

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

func userIdentity() (string, error) { return strconv.Itoa(os.Getuid()), nil }

func endpointName(hash string) string { return "@debounce-v2-" + hash }

func listenEndpoint(name, _ string) (net.Listener, error) {
	// Abstract sockets have no filesystem entry and vanish when all references close.
	return net.Listen("unix", name)
}

func dialEndpoint(ctx context.Context, name string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", name)
}

func retryListen(err error) bool { return errors.Is(err, unix.EADDRINUSE) }

func missingEndpoint(err error) bool {
	return errors.Is(err, unix.ECONNREFUSED) || errors.Is(err, unix.ENOENT)
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
	var credential *unix.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		credential, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return err
	}
	if credErr != nil {
		return credErr
	}
	// Abstract names have no filesystem ACL; verify the peer's actual user.
	if strconv.FormatUint(uint64(credential.Uid), 10) != expectedUser {
		return fmt.Errorf("IPC peer belongs to another user")
	}
	return nil
}
