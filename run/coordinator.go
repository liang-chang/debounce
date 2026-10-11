package run

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type scheduled struct {
	request request
	due     time.Time
}

type coordinator struct {
	mu         sync.Mutex
	pending    *scheduled
	running    bool
	closing    bool
	acceptErr  error
	changed    chan struct{}
	handlers   sync.WaitGroup
	acceptDone chan struct{}
}

type commandResult struct {
	output []byte
	err    error
}

func coordinate(listener net.Listener, initial request, debug bool) (bool, []byte, error) {
	c := &coordinator{
		pending:    &scheduled{initial, time.Now().Add(initial.Interval)},
		changed:    make(chan struct{}, 1),
		acceptDone: make(chan struct{}),
	}
	go c.accept(listener)
	defer func() {
		_ = listener.Close()
		<-c.acceptDone
		// Accepted connections get an explicit retry response before this process
		// exits, including connections received just before the idle decision.
		c.handlers.Wait()
	}()
	completed := make(chan commandResult, 1)
	var output bytes.Buffer
	var resultErr error
	for {
		c.mu.Lock()
		if !c.running && (c.pending == nil || c.acceptErr != nil) {
			c.closing = true
			resultErr = errors.Join(resultErr, c.acceptErr)
			c.mu.Unlock()
			return resultErr == nil, output.Bytes(), resultErr
		}
		var timer *time.Timer
		var timeout <-chan time.Time
		if !c.running && c.pending != nil {
			if !time.Now().Before(c.pending.due) {
				current := c.pending.request
				c.pending, c.running = nil, true
				c.mu.Unlock()
				if debug {
					fmt.Println("Quiet window elapsed: executing command")
				}
				go execute(current, completed)
				continue
			}
			timer = time.NewTimer(time.Until(c.pending.due))
			timeout = timer.C
		}
		c.mu.Unlock()
		select {
		case <-c.changed:
		case <-timeout:
		case result := <-completed:
			_, _ = output.Write(result.output)
			resultErr = errors.Join(resultErr, result.err)
			c.mu.Lock()
			c.running = false
			c.mu.Unlock()
		}
		if timer != nil {
			timer.Stop()
		}
	}
}

func execute(req request, completed chan<- commandResult) {
	command := make([]string, len(req.Command))
	for i, arg := range req.Command {
		command[i] = string(arg)
	}
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = string(req.Directory)
	// Environment is intentionally inherited from the first (coordinator) process.
	output, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("running command: %s: %w", strings.Join(command, " "), err)
	}
	completed <- commandResult{output, err}
}

func (c *coordinator) notify() {
	select {
	case c.changed <- struct{}{}:
	default:
	}
}

func (c *coordinator) accept(listener net.Listener) {
	defer close(c.acceptDone)
	for {
		conn, err := listener.Accept()
		if err != nil {
			c.mu.Lock()
			if !c.closing {
				c.acceptErr = fmt.Errorf("accept trigger: %w", err)
			}
			c.mu.Unlock()
			c.notify()
			return
		}
		c.handlers.Add(1)
		go func() {
			defer c.handlers.Done()
			defer conn.Close()
			c.handle(conn)
		}()
	}
}

func (c *coordinator) handle(conn net.Conn) {
	if err := conn.SetDeadline(time.Now().Add(exchangeTimeout)); err != nil {
		return
	}
	reply := response{Version: protocolVersion}
	if err := authorizePeer(conn); err != nil {
		reply.Error = err.Error()
	} else {
		var req request
		err := json.NewDecoder(io.LimitReader(conn, maxMessageSize)).Decode(&req)
		if err == nil {
			err = req.validate()
		}
		if err != nil {
			reply.Error = err.Error()
		} else {
			c.mu.Lock()
			if c.closing {
				reply.Retry = true
			} else {
				reply.State = "waiting"
				if c.running {
					reply.State = "running"
				}
				if !req.Status {
					c.pending = &scheduled{req, time.Now().Add(req.Interval)}
					c.notify()
				}
			}
			c.mu.Unlock()
		}
	}
	_ = json.NewEncoder(conn).Encode(reply)
}
