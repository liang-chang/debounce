//go:build windows || linux || darwin

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type execution struct {
	Tag         string
	Directory   string
	Environment string
	At          time.Time
}

func TestCLIProcess(t *testing.T) {
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{os.Args[0]}, os.Args[i+1:]...)
			main()
			return
		}
	}
}

func TestTargetProcess(t *testing.T) {
	for i, arg := range os.Args {
		if arg != "--" {
			continue
		}
		logDir, mode, tag := os.Args[i+1], os.Args[i+2], os.Args[i+3]
		cwd, err := os.Getwd()
		require.NoError(t, err)
		record := execution{tag, cwd, os.Getenv("DEBOUNCE_TEST_ENV"), time.Now()}
		data, err := json.Marshal(record)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(logDir, tag+".json"), data, 0600))
		if mode == "hold" {
			require.Eventually(t, func() bool { _, err := os.Stat(filepath.Join(logDir, tag+".release")); return err == nil }, 15*time.Second, 10*time.Millisecond)
		}
		if mode == "fail" {
			os.Exit(3)
		}
		fmt.Println("executed-" + tag)
		os.Exit(0)
	}
}

func cli(t *testing.T, key, logDir, mode, tag string, flags ...string) *exec.Cmd {
	t.Helper()
	return cliWithInterval(t, key, logDir, mode, tag, "1", flags...)
}

func cliWithInterval(t *testing.T, key, logDir, mode, tag, seconds string, flags ...string) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	args := []string{"-test.run=^TestCLIProcess$", "--", "--key", key}
	args = append(args, flags...)
	args = append(args, seconds, "s", exe, "-test.run=^TestTargetProcess$", "--", logDir, mode, tag)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return exec.CommandContext(ctx, exe, args...)
}

func start(t *testing.T, cmd *exec.Cmd) *bytes.Buffer {
	t.Helper()
	output := &bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = output, output
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		if t.Failed() {
			t.Logf("coordinator output: %s", output.String())
		}
	})
	return output
}

func submit(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	require.Contains(t, string(output), "Existing coordinator")
	require.Contains(t, string(output), "trigger accepted")
	return string(output)
}

func readExecution(t *testing.T, dir, tag string) execution {
	t.Helper()
	var record execution
	data, err := os.ReadFile(filepath.Join(dir, tag+".json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &record))
	return record
}

func waitExecution(t *testing.T, dir, tag string) {
	t.Helper()
	require.Eventually(t, func() bool { _, err := os.Stat(filepath.Join(dir, tag+".json")); return err == nil }, 8*time.Second, 10*time.Millisecond)
}

func waitCoordinator(t *testing.T, key, dir string) {
	t.Helper()
	var last string
	defer func() {
		if t.Failed() {
			t.Logf("last status: %s", last)
		}
	}()
	require.Eventually(t, func() bool {
		output, err := cli(t, key, dir, "ok", "unused", "--status").CombinedOutput()
		last = fmt.Sprintf("%s (%v)", output, err)
		return err == nil && strings.Contains(string(output), "waiting")
	}, 5*time.Second, 20*time.Millisecond)
}

func TestTrailingEdgeLatestCommandAndExit(t *testing.T) {
	dir := t.TempDir()
	key := dir
	owner := cli(t, key, dir, "ok", "first")
	output := start(t, owner)
	time.Sleep(300 * time.Millisecond)
	_, err := os.Stat(filepath.Join(dir, "first.json"))
	require.True(t, os.IsNotExist(err), "first trigger must wait for the quiet window")
	waitCoordinator(t, key, dir)
	nextDir := t.TempDir()
	next := cli(t, key, dir, "ok", "last")
	next.Dir = nextDir
	submittedAt := time.Now()
	submit(t, next)
	require.NoError(t, owner.Wait(), "%s", output.String())
	record := readExecution(t, dir, "last")
	require.GreaterOrEqual(t, record.At.Sub(submittedAt), 950*time.Millisecond)
	resolved, err := filepath.EvalSymlinks(nextDir)
	require.NoError(t, err)
	require.Equal(t, resolved, record.Directory)
	_, err = os.Stat(filepath.Join(dir, "first.json"))
	require.True(t, os.IsNotExist(err), "superseded command must not execute")
	status, err := cli(t, key, dir, "ok", "unused", "--status").CombinedOutput()
	require.NoError(t, err)
	require.Contains(t, string(status), "No active coordinator")
	// No cooldown remains: a new invocation becomes owner and runs after a new window.
	rerun, err := cli(t, key, dir, "ok", "again").CombinedOutput()
	require.NoError(t, err, "%s", rerun)
	readExecution(t, dir, "again")
}

func TestTriggerDuringExecutionQueuesLatestWithoutInterruption(t *testing.T) {
	dir := t.TempDir()
	key := dir
	owner := cli(t, key, dir, "hold", "running")
	output := start(t, owner)
	t.Cleanup(func() { _ = os.WriteFile(filepath.Join(dir, "running.release"), nil, 0600) })
	waitExecution(t, dir, "running")
	submit(t, cli(t, key, dir, "ok", "superseded"))
	submittedAt := time.Now()
	submit(t, cli(t, key, dir, "ok", "next"))
	time.Sleep(200 * time.Millisecond)
	_, err := os.Stat(filepath.Join(dir, "next.json"))
	require.True(t, os.IsNotExist(err))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "running.release"), nil, 0600))
	require.NoError(t, owner.Wait(), "%s", output.String())
	next := readExecution(t, dir, "next")
	require.GreaterOrEqual(t, next.At.Sub(submittedAt), 950*time.Millisecond)
	_, err = os.Stat(filepath.Join(dir, "superseded.json"))
	require.True(t, os.IsNotExist(err))
	require.Contains(t, output.String(), "executed-running")
	require.Contains(t, output.String(), "executed-next")
}

func TestDifferentKeysAndFirstEnvironment(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DEBOUNCE_TEST_ENV", "first-environment")
	owner := cli(t, dir, dir, "hold", "first")
	output := start(t, owner)
	t.Cleanup(func() { _ = os.WriteFile(filepath.Join(dir, "first.release"), nil, 0600) })
	waitExecution(t, dir, "first")
	other, err := cli(t, dir+"other", dir, "ok", "independent").CombinedOutput()
	require.NoError(t, err, "%s", other)
	t.Setenv("DEBOUNCE_TEST_ENV", "later-environment")
	submit(t, cli(t, dir, dir, "ok", "queued"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "first.release"), nil, 0600))
	require.NoError(t, owner.Wait(), "%s", output.String())
	require.Equal(t, "first-environment", readExecution(t, dir, "queued").Environment)
}

func TestFailureExitsAndNextInvocationCanRun(t *testing.T) {
	dir := t.TempDir()
	output, err := cli(t, dir, dir, "fail", "failed").CombinedOutput()
	require.Error(t, err, "%s", output)
	require.Contains(t, string(output), "running command")
	output, err = cli(t, dir, dir, "ok", "retry").CombinedOutput()
	require.NoError(t, err, "%s", output)
	readExecution(t, dir, "retry")
}

func TestRemovedFlags(t *testing.T) {
	for _, flag := range []string{"--state-dir=x", "--cache-dir=x", "--namespace=x"} {
		output, err := cli(t, t.TempDir(), t.TempDir(), "ok", "unused", flag).CombinedOutput()
		require.Error(t, err)
		require.Contains(t, string(output), "unknown flag")
	}
}

func TestConcurrentFirstCallersElectOneCoordinator(t *testing.T) {
	dir := t.TempDir()
	var commands []*exec.Cmd
	var outputs []*bytes.Buffer
	for i := 0; i < 6; i++ {
		cmd := cliWithInterval(t, dir, dir, "ok", fmt.Sprintf("candidate-%d", i), "3")
		outputs = append(outputs, start(t, cmd))
		commands = append(commands, cmd)
	}
	accepted := 0
	for i, cmd := range commands {
		require.NoError(t, cmd.Wait(), "%s", outputs[i].String())
		if strings.Contains(outputs[i].String(), "trigger accepted") {
			accepted++
		}
	}
	require.Equal(t, 5, accepted)
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	require.NoError(t, err)
	require.Len(t, files, 1, "only the final command in the burst should execute")
}
