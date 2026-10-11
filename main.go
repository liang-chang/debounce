package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/alecthomas/kong"
	"github.com/liang-chang/debounce/run"
	"github.com/liang-chang/debounce/types"
)

const vstring = "0.6.0"

type CLI struct { //nolint:govet
	Debug    bool             `help:"Print debugging info to screen"`
	Version  kong.VersionFlag `help:"Print version to screen"`
	Status   bool             `help:"Query the active coordinator without triggering a command"`
	Local    bool             `help:"Localize debounce to current working directory"`
	Key      string           `help:"Explicit task identity, shared across commands and working directories"`
	Quantity string           `arg:"" help:"Quantity of time"`
	Unit     string           `arg:"" required:"" enum:"s,second,seconds,m,minute,minutes,h,hour,hours,d,day,days" help:"s,second,seconds,m,minute,minutes,h,hour,hours,d,day,days"` //nolint:lll
	Command  []string         `arg:"" help:"Command to run" passthrough:""`
}

func main() {
	var cli CLI
	parser := kong.Must(&cli,
		kong.Name("debounce"),
		kong.Description("run the latest command after a quiet window (Linux/Windows/macOS)"),
		kong.UsageOnError(),
		kong.Exit(func(code int) {
			if code != 0 {
				code = 1
			}
			os.Exit(code)
		}),
		kong.Vars{"version": vstring},
	)

	ctx, err := parser.Parse(os.Args[1:])
	parser.FatalIfErrorf(err)

	validateArgs(ctx, &cli)

	runContext := types.DebounceCommand{
		Quantity: cli.Quantity,
		Unit:     normalizeUnit(cli.Unit),
		Command:  cli.Command,
		Debug:    cli.Debug,
		Local:    cli.Local,
		Status:   cli.Status,
		Key:      cli.Key,
	}

	success, output, err := run.Run(&runContext)
	fmt.Print(string(output))
	if err != nil {
		fmt.Printf("💥 %s\n", err)
		os.Exit(1)
	}

	if success {
		os.Exit(0)
	}
	os.Exit(1)
}

func validateArgs(ctx *kong.Context, cli *CLI) {
	if _, err := strconv.Atoi(cli.Quantity); err != nil {
		handleError(ctx, fmt.Errorf("quantity %s is not a valid integer", cli.Quantity))
	}
	if len(cli.Command) > 0 && cli.Command[0] == "--" {
		cli.Command = cli.Command[1:]
		if len(cli.Command) == 0 {
			handleError(ctx, fmt.Errorf("command is missing"))
		}
	}
}

func normalizeUnit(unit string) string {
	switch unit {
	case "minutes", "minute", "m":
		return "m"
	case "seconds", "second", "s":
		return "s"
	case "hours", "hour", "h":
		return "h"
	case "days", "day", "d":
		return "d"
	default:
		return unit
	}
}

func handleError(ctx *kong.Context, err error) {
	fmt.Printf("💥 %s\n", err)
	_ = ctx.PrintUsage(false)
	os.Exit(1)
}
