//go:build linux || darwin

package run

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestPathEndpointRecoveryAndOwnership(t *testing.T) {
	// Keep paths below macOS's Unix socket path limit.
	dir, err := os.MkdirTemp("/tmp", "debounce-test-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	name := filepath.Join(dir, "task.sock")
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: name, Net: "unix"})
	require.NoError(t, err)
	stale.SetUnlinkOnClose(false)
	require.NoError(t, stale.Close())
	first, err := listenPathEndpoint(name)
	require.NoError(t, err, "recover a socket left by a dead coordinator")
	t.Cleanup(func() { _ = first.Close() })
	_, err = listenPathEndpoint(name)
	require.Error(t, err, "a live owner must not be replaced")
	conn, err := net.Dial("unix", name)
	require.NoError(t, err, "failed takeover must leave the live socket intact")
	_ = conn.Close()
	before, err := os.Stat(name + ".lock")
	require.NoError(t, err)
	require.NoError(t, first.Close())
	second, err := listenPathEndpoint(name)
	require.NoError(t, err)
	defer second.Close()
	require.NoError(t, first.Close(), "repeated close must not remove the new owner's socket")
	conn, err = net.Dial("unix", name)
	require.NoError(t, err)
	_ = conn.Close()
	after, err := os.Stat(name + ".lock")
	require.NoError(t, err)
	require.True(t, os.SameFile(before, after), "all owners must lock the same inode")
}

func TestPathEndpointRejectsUnsafeDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0755))
	_, err := listenPathEndpoint(filepath.Join(dir, "task.sock"))
	require.Error(t, err)
}

func TestPathEndpointCrashRecovery(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "debounce-crash-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	name := filepath.Join(dir, "task.sock")
	exe, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestPathEndpointOwnerProcess$")
	cmd.Env = append(os.Environ(), "DEBOUNCE_PATH_TEST="+name)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	line, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "ready\n", line)
	_, err = listenPathEndpoint(name)
	require.Error(t, err, "another process owns the endpoint")
	require.NoError(t, cmd.Process.Kill())
	require.Error(t, cmd.Wait())
	_, err = os.Stat(name)
	require.NoError(t, err, "forced termination leaves a socket to recover")
	listener, err := listenPathEndpoint(name)
	require.NoError(t, err, "kernel releases the dead process's lock")
	require.NoError(t, listener.Close())
}

func TestPathEndpointOwnerProcess(t *testing.T) {
	name := os.Getenv("DEBOUNCE_PATH_TEST")
	if name == "" {
		return
	}
	listener, err := listenPathEndpoint(name)
	require.NoError(t, err)
	defer listener.Close()
	fmt.Println("ready")
	time.Sleep(30 * time.Second)
}

func TestPathEndpointConcurrentElection(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "debounce-race-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	name := filepath.Join(dir, "private", "task.sock")
	const callers = 16
	winners := make(chan net.Listener, callers)
	results := make(chan error, callers)
	start := make(chan struct{})
	var group sync.WaitGroup
	for i := 0; i < callers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			listener, err := listenPathEndpoint(name)
			if err == nil {
				winners <- listener
			}
			results <- err
		}()
	}
	close(start)
	group.Wait()
	close(winners)
	close(results)
	owners := 0
	for listener := range winners {
		owners++
		defer listener.Close()
	}
	require.Equal(t, 1, owners)
	for err := range results {
		if err != nil {
			require.ErrorIs(t, err, unix.EWOULDBLOCK)
		}
	}
}
