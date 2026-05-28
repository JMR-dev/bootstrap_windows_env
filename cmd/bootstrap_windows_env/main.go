package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/JMR-dev/bootstrap_windows_env/internal/bootstrap"
)

type bootstrapApp interface {
	Run(context.Context, bootstrap.Options) error
}

var newBootstrapper = func() (bootstrapApp, error) {
	return bootstrap.NewBootstrapper()
}

var stderr io.Writer = os.Stderr
var exitProcess = os.Exit

func main() {
	exitProcess(run(os.Args[1:], stderr, newBootstrapper))
}

func run(args []string, stderr io.Writer, newApp func() (bootstrapApp, error)) int {
	opts, err := bootstrap.ParseOptions(args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	app, err := newApp()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := app.Run(context.Background(), opts); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
