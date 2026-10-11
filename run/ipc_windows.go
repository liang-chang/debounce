package run

import (
	"context"
	"errors"
	"net"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func userIdentity() (string, error) {
	// Read the SID directly: service/sandbox accounts may have no user profile,
	// which makes os/user.Current fail even though the access token is valid.
	current, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return current.User.Sid.String(), nil
}

func endpointName(hash string) string { return `\\.\pipe\debounce-v2-` + hash }

func listenEndpoint(name, userID string) (net.Listener, error) {
	// ListenPipe retains an exclusive first instance. Restrict clients to this user.
	return winio.ListenPipe(name, &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;" + userID + ")",
	})
}

func dialEndpoint(ctx context.Context, name string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, name)
}

func retryListen(err error) bool {
	// ACCESS_DENIED can also mean a real permission failure: callers retry only
	// briefly, then report an error instead of treating it as a successful trigger.
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_PIPE_BUSY)
}

func missingEndpoint(err error) bool { return errors.Is(err, windows.ERROR_FILE_NOT_FOUND) }

// The listener's DACL performs peer authorization before a connection is accepted.
func authorizePeer(_ net.Conn) error { return nil }

func authenticateCoordinator(conn net.Conn, expectedUser string) error {
	pipe, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return errors.New("expected Windows named pipe")
	}
	// Authenticate before sending command data: a predictable name alone does
	// not prove that its listener belongs to the intended user.
	var pid uint32
	if err := windows.GetNamedPipeServerProcessId(windows.Handle(pipe.Fd()), &pid); err != nil {
		return err
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	if user.User.Sid.String() != expectedUser {
		return errors.New("IPC peer belongs to another user")
	}
	return nil
}
