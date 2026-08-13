package cli

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/herdworks/clic"
)

func TestServeCommandValidatesArguments(t *testing.T) {
	for _, arguments := range []clic.Arguments{
		{},
		{"one", "two"},
		{"--port", "0", "site"},
		{"--bind", "", "site"},
	} {
		result := CLI.Run(testExecution(t), arguments)
		if result.Error == nil {
			t.Fatalf("Run(%v) unexpectedly succeeded", arguments)
		}
	}
}

func TestServeCommandResolvesAndValidatesDirectory(t *testing.T) {
	directory := t.TempDir()
	command := ServeCommand()
	execution := testExecution(t)
	execution.Command = &command
	execution.Command.State.Arguments = clic.Arguments{filepath.Base(directory)}
	execution.WorkingDirectory = filepath.Dir(directory)
	execution.Options = clic.Options{
		"bind": {
			State: clic.OptionState{
				Value: "localhost",
			},
		},
		"port": {
			State: clic.OptionState{
				Value: "8080",
			},
		},
	}
	server, err := serveServer(&execution)
	if err != nil {
		t.Fatal(err)
	}
	if server.Directory != directory {
		t.Fatalf("Directory = %q, want %q", server.Directory, directory)
	}
}

func TestServeCommandRejectsMissingDirectory(t *testing.T) {
	result := CLI.Run(testExecution(t), clic.Arguments{"missing"})
	if result.Error == nil || !os.IsNotExist(result.Error) {
		t.Fatalf("Run() error = %v, want missing-directory error", result.Error)
	}
}

func TestServeCommandWaitsForListenerBeforeReportingReadiness(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	execution := testExecution(t)
	execution.Stdout = &output
	result := CLI.Run(execution, clic.Arguments{
		"--bind", "127.0.0.1",
		"--port", strconv.Itoa(mustPort(t, port)),
		t.TempDir(),
	})
	if result.Error == nil {
		t.Fatal("Run() unexpectedly succeeded")
	}
	if got := output.String(); got != "" {
		t.Fatalf("output = %q, want no readiness output", got)
	}
}

func testExecution(t *testing.T) clic.Execution {
	t.Helper()
	return clic.Execution{
		Context:          context.Background(),
		Stdout:           io.Discard,
		Stderr:           io.Discard,
		LookupEnv:        func(string) (string, bool) { return "", false },
		WorkingDirectory: t.TempDir(),
	}
}

func mustPort(t *testing.T, value string) int {
	t.Helper()
	port, err := strconv.Atoi(value)
	if err != nil {
		t.Fatal(err)
	}
	return port
}
