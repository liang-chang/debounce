//go:build !linux && !windows && !darwin

package run

import (
	"context"
	"fmt"
	"net"
	"runtime"
)

func unsupported() error {
	return fmt.Errorf("debounce IPC is not supported on %s; only Linux, Windows and macOS are implemented", runtime.GOOS)
}
func userIdentity() (string, error)                              { return "", unsupported() }
func endpointName(hash string) string                            { return hash }
func listenEndpoint(_, _ string) (net.Listener, error)           { return nil, unsupported() }
func dialEndpoint(_ context.Context, _ string) (net.Conn, error) { return nil, unsupported() }
func retryListen(_ error) bool                                   { return false }
func missingEndpoint(_ error) bool                               { return false }
func authorizePeer(_ net.Conn) error                             { return unsupported() }
func authenticateCoordinator(_ net.Conn, _ string) error         { return unsupported() }
