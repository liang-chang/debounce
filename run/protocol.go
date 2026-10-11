package run

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"
	"time"
)

const (
	protocolVersion = 1
	maxMessageSize  = 1024 * 1024
	exchangeTimeout = 5 * time.Second
)

type request struct {
	Version   int
	Command   [][]byte
	Directory []byte
	Interval  time.Duration
	Status    bool
}

type response struct {
	Version int
	State   string
	Retry   bool
	Error   string
}

func (r request) validate() error {
	if r.Version != protocolVersion {
		return errors.New("incompatible IPC protocol")
	}
	if r.Status {
		return nil
	}
	if r.Interval <= 0 || r.Interval > 36500*24*time.Hour {
		return errors.New("invalid debounce interval")
	}
	if len(r.Command) == 0 || len(r.Command[0]) == 0 {
		return errors.New("command is missing")
	}
	if !filepath.IsAbs(string(r.Command[0])) || !filepath.IsAbs(string(r.Directory)) {
		return errors.New("command and working directory must be absolute paths")
	}
	for _, arg := range append(append([][]byte{}, r.Command...), r.Directory) {
		if strings.ContainsRune(string(arg), 0) {
			return errors.New("NUL in command or directory")
		}
	}
	return nil
}

func exchange(conn net.Conn, r request) (response, error) {
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(exchangeTimeout)); err != nil {
		return response{}, err
	}
	if err := json.NewEncoder(conn).Encode(r); err != nil {
		return response{}, err
	}
	var reply response
	if err := json.NewDecoder(io.LimitReader(conn, maxMessageSize)).Decode(&reply); err != nil {
		return reply, err
	}
	if reply.Version != protocolVersion {
		return reply, errors.New("incompatible IPC response")
	}
	if reply.Error != "" {
		return reply, fmt.Errorf("coordinator rejected request: %s", reply.Error)
	}
	if !reply.Retry && reply.State != "waiting" && reply.State != "running" {
		return reply, errors.New("invalid IPC response")
	}
	return reply, nil
}
