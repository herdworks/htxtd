package cli

import (
	"fmt"
	"strconv"

	"github.com/herdworks/clic"
	"github.com/herdworks/htxtd"
)

// ServeCommand serves a static HyperTexting website directory.
func ServeCommand() clic.Command {
	return clic.Command{
		Meta: clic.CommandMeta{
			Name:        "htxtd",
			Description: "Serve a static HyperTexting website.",
			Usage: []string{
				"htxtd [options] <directory>",
			},
		},
		Spec: clic.CommandSpec{
			Key:  "htxtd",
			Help: clic.DefaultHelp,
			Options: clic.Options{
				"bind": {
					Meta: clic.OptionMeta{
						Name:        "bind",
						ShortName:   "B",
						EnvName:     "HYPER_BIND",
						Description: "IP address or hostname to bind.",
						Default:     "localhost",
					},
					Spec: clic.OptionSpec{
						Key:  "bind",
						Kind: clic.StringOption,
					},
				},
				"port": {
					Meta: clic.OptionMeta{
						Name:        "port",
						ShortName:   "P",
						EnvName:     "HYPER_PORT",
						Description: "TCP port to listen on.",
						Default:     "8080",
					},
					Spec: clic.OptionSpec{
						Key:  "port",
						Kind: clic.StringOption,
					},
				},
			},
			Validator: serveValidator,
			Handler:   serveHandler,
		},
	}
}

func serveValidator(execution *clic.Execution) error {
	if execution.Command.PositionalArguments() != 1 {
		return fmt.Errorf(
			"%s requires one website directory",
			execution.Command.Meta.Name,
		)
	}
	port, err := strconv.Atoi(execution.Options["port"].StringValue())
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid port: %s", execution.Options["port"].StringValue())
	}
	if execution.Options["bind"].StringValue() == "" {
		return fmt.Errorf("bind address is required")
	}
	return nil
}

func serveHandler(execution *clic.Execution) error {
	server, err := serveServer(execution)
	if err != nil {
		return err
	}
	listener, err := server.Listen()
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(execution.Stdout, "Listening on %s...\n", server.Address()); err != nil {
		_ = listener.Close()
		return err
	}
	return server.Serve(execution.Context, listener)
}

func serveServer(execution *clic.Execution) (*htxtd.Server, error) {
	directory, err := htxtd.AbsoluteDirectory(
		execution.WorkingDirectory,
		execution.Command.State.Arguments[0],
	)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(execution.Options["port"].StringValue())
	if err != nil {
		return nil, fmt.Errorf("invalid port: %s", execution.Options["port"].StringValue())
	}
	server := &htxtd.Server{
		Directory: directory,
		Bind:      execution.Options["bind"].StringValue(),
		Port:      port,
		LogWriter: execution.Stdout,
	}
	if err = server.Validate(); err != nil {
		return nil, err
	}
	return server, nil
}
