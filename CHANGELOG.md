# Changes

## 0.6.0 (unreleased)

- First independently maintained release under `github.com/liang-chang/debounce`.
- Replace successful-execution cooldown with cross-process trailing-edge debounce.
- Run the latest accepted command after a quiet window; merge triggers during
  execution into one pending next run without interrupting the running command.
- Add explicit task keys, status queries and user-scoped IPC coordination.
- Use Windows named pipes and Linux abstract Unix sockets; add experimental
  macOS pathname sockets with file-lock ownership and stale-socket recovery.
- Remove upstream file-based cache and `--cache-dir` behavior.
- Primarily tested on Windows; Linux tested in WSL2 Debian. macOS has only been
  cross-compiled locally, with no Mac runtime testing by the maintainer.

## Upstream history (oalders/debounce)

## 0.5.2 2025-10-27

- Create cache dir in correct location if it does not already exist

## 0.5.1 2024-10-09

- Don't print usage message when a command exits non-zero

## 0.5.0 2024-09-29

- Add --local flag

## 0.4.0 - 2024-09-26

- Add days/day/d to available units of time

## 0.3.0 - 2024-09-25

- Add --status flag
- add a --cache-dir param

## 0.2.0 - 2024-09-13

- Obfuscate cache file names

## 0.1.0 - 2024-09-02

- First release upon an unsuspecting world.
