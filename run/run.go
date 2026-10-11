package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/liang-chang/debounce/types"
	is_age "github.com/oalders/is/age"
)

func Run(args *types.DebounceCommand) (bool, []byte, error) {
	userID, err := userIdentity()
	if err != nil {
		return false, nil, err
	}
	if len(args.Command) == 0 {
		return false, nil, errors.New("command is missing")
	}
	duration, err := is_age.StringToDuration(args.Quantity, args.Unit)
	if err != nil {
		return false, nil, err
	}
	taskID, err := GenerateTaskID(args)
	if err != nil {
		return false, nil, err
	}
	req := request{Version: protocolVersion, Interval: duration.Abs(), Status: args.Status}
	if !args.Status {
		cwd, err := os.Getwd()
		if err != nil {
			return false, nil, err
		}
		executable, err := exec.LookPath(args.Command[0])
		if err != nil {
			return false, nil, fmt.Errorf("resolve command: %w", err)
		}
		executable, err = filepath.Abs(executable)
		if err != nil {
			return false, nil, err
		}
		req.Command, req.Directory = commandBytes(args.Command), []byte(cwd)
		req.Command[0] = []byte(executable)
	}
	if err := req.validate(); err != nil {
		return false, nil, err
	}
	encoded, err := json.Marshal(req)
	if err != nil {
		return false, nil, err
	}
	if len(encoded)+1 > maxMessageSize {
		return false, nil, errors.New("trigger exceeds 1 MiB IPC limit")
	}
	endpoint := scopedEndpoint(userID, taskID)
	if args.Debug {
		fmt.Printf("IPC endpoint: %s\n", endpoint)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		conn, dialErr := dialEndpoint(ctx, endpoint)
		cancel()
		if dialErr == nil {
			if err := authenticateCoordinator(conn, userID); err != nil {
				_ = conn.Close()
				return false, nil, fmt.Errorf("verify coordinator identity: %w", err)
			}
			reply, err := exchange(conn, req)
			if err != nil {
				// Once sent, the request might have been accepted. Do not blindly replay
				// across coordinator death and execute a command twice.
				return false, nil, fmt.Errorf("trigger not confirmed (it may have been accepted): %w", err)
			}
			if !reply.Retry {
				if args.Status {
					fmt.Printf("Existing coordinator: %s\n", reply.State)
				} else {
					fmt.Printf("Existing coordinator: %s; trigger accepted\n", reply.State)
				}
				return true, nil, nil
			}
		} else {
			if args.Status {
				if missingEndpoint(dialErr) {
					fmt.Println("No active coordinator")
					return true, nil, nil
				}
				return false, nil, fmt.Errorf("query coordinator: %w", dialErr)
			}
			listener, err := listenEndpoint(endpoint, userID)
			if err == nil {
				if args.Debug {
					fmt.Println("Coordinator started: waiting for quiet window")
				}
				return coordinate(listener, req, args.Debug)
			}
			if !retryListen(err) {
				return false, nil, fmt.Errorf("create coordinator: %w", err)
			}
		}
		if time.Now().After(deadline) {
			return false, nil, errors.New("cannot connect to or start coordinator within 5 seconds")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
