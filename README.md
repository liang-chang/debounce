# debounce

Cross-process **trailing-edge debounce** for Windows, Linux and macOS, based on
[oalders/debounce](https://github.com/oalders/debounce) and independently maintained
by [liang-chang](https://github.com/liang-chang). This version replaces upstream's
successful-execution cooldown with a quiet-window scheduler. macOS support is experimental.

[中文说明](README.zh-CN.md)

## Installation

Download an archive for your OS and architecture from
[Releases](https://github.com/liang-chang/debounce/releases), extract it and add
the executable's directory to `PATH`. Windows archives use ZIP; Linux/macOS use
tar.gz. Linux DEB/RPM packages are also produced.

With Go 1.24 or newer:

```text
go install github.com/liang-chang/debounce@latest
debounce --version
```

The first independent version is `0.6.0` (tag `v0.6.0`). Report issues at
[liang-chang/debounce/issues](https://github.com/liang-chang/debounce/issues).

## Build

Requires Go 1.24 or newer. No CGO, external lock utility, or background service
installation is required.

```powershell
go build -o debounce.exe .
.\debounce.exe --key repo-check 3 s git status --short
```

On Linux/macOS: `go build -o debounce .`.

Upstream releases do **not** contain this implementation. The original license
files and upstream release history are retained for attribution.

## Lifecycle

1. The first invocation becomes the coordinator and stays in the foreground.
   It waits for a full quiet window before executing anything.
2. Another invocation with the same identity submits its latest command and
   resets the window. After acknowledgement it prints
   `Existing coordinator: waiting; trigger accepted` (or `running`)
   and exits with 0.
3. After the quiet window, the coordinator executes the **last accepted command**.
   Ordering is by acceptance at the coordinator, not by client process start time.
4. New triggers during execution never interrupt that command. They are merged
   into one pending next run. It starts after both the current command finishes
   and the pending quiet window expires.
5. When no command or pending trigger remains, the coordinator closes its IPC
   endpoint and exits. A new invocation then starts a fresh quiet window.

For a 3-second window, triggers at seconds 0, 1 and 2 execute once at about
second 5. There is **no success cooldown** after that execution.

## Usage and identity

```text
debounce [flags] <integer> <unit> <command> [arguments...]
```

Units: `s/second/seconds`, `m/minute/minutes`, `h/hour/hours`, `d/day/days`.
A day is 24 hours. Quantities must be between 1 and 36500. Flags go before the
quantity; `--` can separate the target command.

- `--key KEY`: explicit logical task identity. Same key shares a coordinator,
  even across commands and working directories.
- Without `--key`: versioned structured command/argument identity, preserving
  argument boundaries and raw Unix bytes.
- `--local`: adds the cleaned, symlink-resolved working directory to automatic
  identity. It does not affect an explicit key.
- `--status`: reports `waiting`, `running` or `No active coordinator`
  without submitting a trigger or changing the timer. Supply the same identity.
- `--debug`: reports the IPC endpoint and coordinator transitions.
- `--version`, `--help`: version and usage.

The current user's UID/SID scopes all identities. There is no `--namespace`.
`--state-dir` and the upstream `--cache-dir` are removed.

Different intervals on the same identity are allowed; the last accepted trigger
supplies both the next command and its quiet-window duration.

## Command execution and results

Commands are executed directly, without an implicit shell. Use `cmd.exe /c`
for CMD builtins or `pwsh.exe -NoProfile -File job.ps1` for a script.
The executable is resolved by the submitting process; its absolute path,
arguments and working directory are sent to the coordinator.

Environment variables are **not transmitted**. Every child inherits the
coordinator's environment, which came from the first invocation. Later callers'
environment changes do not apply.

Target output is buffered and printed by the **coordinator's terminal**, when
the coordinator exits. Submitting clients receive an acknowledgement, not the
target's eventual result. The coordinator exits with 1 if any command or IPC
service operation failed; otherwise it exits with 0. A command failure does not
discard already accepted next-round triggers or automatically retry that command.

A request received during orderly shutdown gets a retry response; the caller
reconnects or competes to become the next coordinator. If delivery happened but
acknowledgement failed, the client reports an **uncertain outcome** and exits
nonzero, rather than automatically replaying a potentially accepted command.

## Platform support and boundaries

- **Linux:** abstract Unix-domain stream socket. No `.sock` file is created.
  The endpoint is removed by the kernel once all socket references close.
  Peer credentials are checked against the current UID. Coordination is within
  the same Linux network namespace.
- **Windows:** exclusive named-pipe listener via `github.com/Microsoft/go-winio`.
  Its ACL permits the current user's SID. User identity comes from the process
  token, so an account does not need a profile directory. Clients also verify
  the named-pipe server's process-token SID before sending command data.
- **macOS (experimental):** pathname Unix-domain socket in the current user's
  private `0700` directory `/tmp/debounce-v2-<uid>`, with peer UID verification.
  A per-task `flock` is held for the coordinator's lifetime. Only the lock owner
  may remove a stale socket after a crash. Normal shutdown removes the socket
  before releasing the lock; empty `.lock` files remain to preserve lock identity.
  Do not delete this directory or its lock files while invocations are active.
- Other systems return an explicit unsupported-platform error.

Linux/Windows do not persist lock, stamp or socket files. macOS retains empty
lock files and may leave socket files after a crash. Requests are held in memory.
A crash, forced termination or reboot can lose pending work. A running target
may outlive its coordinator; this version does not guarantee orphan-process
exclusion, automatic recovery or exactly-once execution across crashes.

The first caller must remain alive until the work finishes. This is not a
detached scheduler, distributed lock, or file watcher. Use distinct keys for
independent repositories/tasks. All cooperating invocations should use this
same implementation/protocol.

A trigger is limited to 1 MiB. IPC exchanges have a 5-second deadline; startup
and shutdown connection races retry for up to 5 seconds.

## Validation and licenses

```text
go test ./...
go vet ./...
```

Integration tests launch separate processes and cover timer reset, latest-command
selection, queued execution, environment/cwd behavior, independent keys, failures,
automatic exit and concurrent coordinator election. Linux CI also uses the race
detector. Cross-compilation is not a substitute for runtime IPC tests.

Upstream Apache-2.0/MIT license files remain. Third-party IPC dependencies and
their licenses are in `THIRD-PARTY-NOTICES`.

## Platform testing notes

This project is primarily used and validated on **Windows**.

- **Windows:** Go tests and static checks are run locally.
- **Linux:** Linux test binaries are run in WSL2 Debian to validate debounce,
  concurrent coordination and command execution. WSL results do not cover every
  environment on standalone Linux hosts.
- **macOS:** The maintainer has no Mac and has **not performed local runtime
  testing on a Mac**. Local validation consists of cross-compilation and testing
  the shared pathname-socket/file-lock logic in WSL. This does not replace macOS
  peer-identity checks and runtime validation, so support remains experimental.

GitHub Actions tests have passed on Windows, Linux and macOS. macOS still lacks
local usage validation by the maintainer and remains experimental.
