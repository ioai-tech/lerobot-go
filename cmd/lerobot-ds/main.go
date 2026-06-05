package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "lerobot-ds is deprecated; use lerobot-go instead:")
	fmt.Fprintln(os.Stderr, "  go build -o lerobot-go ./cmd/lerobot-go")
	os.Exit(1)
}
