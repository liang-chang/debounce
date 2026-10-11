package run_test

import (
	"testing"

	"github.com/liang-chang/debounce/run"
	"github.com/liang-chang/debounce/types"
	"github.com/stretchr/testify/require"
)

func TestStateKeyIdentity(t *testing.T) {
	a := &types.DebounceCommand{Command: []string{"tool", "a b", "c"}}
	b := &types.DebounceCommand{Command: []string{"tool", "a", "b c"}}
	first, err := run.GenerateTaskID(a)
	require.NoError(t, err)
	second, err := run.GenerateTaskID(b)
	require.NoError(t, err)
	require.NotEqual(t, first, second, "argument boundaries must be preserved")
	a.Key, b.Key = "../explicit:task", "../explicit:task"
	a.Local = true
	first, err = run.GenerateTaskID(a)
	require.NoError(t, err)
	second, err = run.GenerateTaskID(b)
	require.NoError(t, err)
	require.Equal(t, first, second, "explicit keys ignore command and --local")
	require.Regexp(t, `^[a-f0-9]{64}$`, first)
	a.Key, a.Local = "", false
	a.Command = []string{b.Key}
	first, err = run.GenerateTaskID(a)
	require.NoError(t, err)
	require.NotEqual(t, first, second, "explicit and automatic keys have separate namespaces")
}

func TestStateKeyPreservesUnixArgumentBytes(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		a := &types.DebounceCommand{Command: []string{"tool", string([]byte{0xff})}}
		b := &types.DebounceCommand{Command: []string{"tool", string([]byte{0xfe})}}
		if explicit {
			a.Key, b.Key = a.Command[1], b.Command[1]
		}
		first, err := run.GenerateTaskID(a)
		require.NoError(t, err)
		second, err := run.GenerateTaskID(b)
		require.NoError(t, err)
		require.NotEqual(t, first, second, "JSON must not replace distinct invalid UTF-8 bytes with the same character")
	}
}

func TestLocalStateKey(t *testing.T) {
	args := &types.DebounceCommand{Command: []string{"tool"}, Local: true}
	firstDir, secondDir := t.TempDir(), t.TempDir()
	t.Chdir(firstDir)
	first, err := run.GenerateTaskID(args)
	require.NoError(t, err)
	t.Chdir(secondDir)
	second, err := run.GenerateTaskID(args)
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	args.Local = false
	first, err = run.GenerateTaskID(args)
	require.NoError(t, err)
	t.Chdir(firstDir)
	second, err = run.GenerateTaskID(args)
	require.NoError(t, err)
	require.Equal(t, first, second)
}

func TestInvalidRunDoesNotExecute(t *testing.T) {
	for _, args := range []*types.DebounceCommand{
		{Quantity: "1", Unit: "s"},
		{Quantity: "0", Unit: "s", Command: []string{"go"}},
		{Quantity: "1", Unit: "invalid", Command: []string{"go"}},
	} {
		success, output, err := run.Run(args)
		require.Error(t, err)
		require.False(t, success)
		require.Empty(t, output)
	}
}
