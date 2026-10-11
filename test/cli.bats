#!/usr/bin/env bats

setup() {
    task_key="bats-$BATS_TEST_NUMBER-$$"
}

@test "Check fork version" {
    run ./bin/debounce --version
    [ "$status" -eq 0 ]
    [ "$output" == "0.6.0" ]
}

@test "Run command after quiet window" {
    run ./bin/debounce --key "$task_key" 1 s bash -c 'echo completed'
    [ "$status" -eq 0 ]
    [[ "$output" == *completed* ]]
}

@test "Command failure is reported" {
    run ./bin/debounce --key "$task_key" 1 s bash -c 'exit 2'
    [ "$status" -eq 1 ]
    [[ "$output" == *"running command"* ]]
}

@test "Status does not start a coordinator" {
    run ./bin/debounce --key "$task_key" --status 1 s echo ignored
    [ "$status" -eq 0 ]
    [[ "$output" == *"No active coordinator"* ]]
}

@test "Removed file-state flags are rejected" {
    run ./bin/debounce --state-dir /tmp 1 s echo ignored
    [ "$status" -eq 1 ]
    [[ "$output" == *"unknown flag"* ]]
}

@test "All time units parse without executing" {
    for unit in s second seconds m minute minutes h hour hours d day days; do
        run ./bin/debounce --key "$task_key" --status 1 "$unit" echo ignored
        [ "$status" -eq 0 ]
        [[ "$output" == *"No active coordinator"* ]]
    done
}

@test "Invalid input is rejected" {
    run ./bin/debounce invalid s echo ignored
    [ "$status" -eq 1 ]
    [[ "$output" == *"not a valid integer"* ]]
}
