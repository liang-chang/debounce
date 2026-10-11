//go:build windows || linux || darwin

package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testEndpoint(t *testing.T) (string, string) {
	t.Helper()
	userID, err := userIdentity()
	require.NoError(t, err)
	sum := sha256.Sum256([]byte(t.TempDir()))
	return scopedEndpoint(userID, hex.EncodeToString(sum[:])), userID
}

func TestEndpointExclusiveOwnershipAndRelease(t *testing.T) {
	name, userID := testEndpoint(t)
	first, err := listenEndpoint(name, userID)
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })
	second, err := listenEndpoint(name, userID)
	if second != nil {
		_ = second.Close()
	}
	require.Error(t, err, "two listeners must not own the same identity")
	require.True(t, retryListen(err))
	require.NoError(t, first.Close())
	second, err = listenEndpoint(name, userID)
	require.NoError(t, err, "closing the owner must release the kernel endpoint")
	require.NoError(t, second.Close())
}

func TestClosingCoordinatorRejectsTriggerForRetry(t *testing.T) {
	name, userID := testEndpoint(t)
	listener, err := listenEndpoint(name, userID)
	require.NoError(t, err)
	c := &coordinator{closing: true, changed: make(chan struct{}, 1), acceptDone: make(chan struct{})}
	go c.accept(listener)
	t.Cleanup(func() { _ = listener.Close(); <-c.acceptDone; c.handlers.Wait() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := dialEndpoint(ctx, name)
	require.NoError(t, err)
	reply, err := exchange(conn, request{Version: protocolVersion, Status: true})
	require.NoError(t, err)
	require.True(t, reply.Retry, "shutdown must not acknowledge a request it cannot handle")
}

func TestProtocolFailureIsNotAnAcknowledgement(t *testing.T) {
	client, server := net.Pipe()
	go func() { _ = server.Close() }()
	_, err := exchange(client, request{Version: protocolVersion, Status: true})
	require.Error(t, err)
	require.Error(t, (request{Version: protocolVersion + 1, Status: true}).validate())
}

func TestCoordinatorIdentityBeforeSending(t *testing.T) {
	name, userID := testEndpoint(t)
	listener, err := listenEndpoint(name, userID)
	require.NoError(t, err)
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() { conn, _ := listener.Accept(); accepted <- conn }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := dialEndpoint(ctx, name)
	require.NoError(t, err)
	defer conn.Close()
	server := <-accepted
	require.NotNil(t, server)
	defer server.Close()
	require.Error(t, authenticateCoordinator(conn, "different-user"))
	require.NoError(t, authenticateCoordinator(conn, userID))
}
