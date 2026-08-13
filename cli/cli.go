// Package cli composes htxtd's static website server command.
package cli

import (
	"github.com/herdworks/clic"
)

var CLI = clic.CLI{
	Command: ServeCommand(),
}
