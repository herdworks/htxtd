// Command htxtd serves static HyperTexting websites.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/herdworks/clic"
	"github.com/herdworks/htxtd/cli"
)

func main() {
	workingDirectory, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	context, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result := cli.CLI.Run(clic.Execution{
		Context:          context,
		Stdin:            os.Stdin,
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
		LookupEnv:        os.LookupEnv,
		WorkingDirectory: workingDirectory,
	}, os.Args[1:])
	if result.Error != nil {
		fmt.Fprintln(os.Stderr, result.Error)
	}
	os.Exit(result.ExitCode)
}
