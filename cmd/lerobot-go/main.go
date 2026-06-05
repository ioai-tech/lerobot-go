package main

import (
	"os"

	"github.com/ioai-tech/lerobot-go/internal/cli"
)

func main() {
	if err := cli.NewRoot().Execute(); err != nil {
		os.Exit(1)
	}
}
