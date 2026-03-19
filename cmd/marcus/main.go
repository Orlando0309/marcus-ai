package main

import (
	"fmt"
	"os"

	"github.com/marcus-ai/marcus/internal/cli"
)

var version = "1.0.0-alpha"

func main() {
	rootCmd := cli.NewRootCmd()
	rootCmd.Version = version

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
