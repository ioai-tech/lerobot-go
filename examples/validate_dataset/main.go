// Validate an on-disk LeRobot dataset.
//
// Run:
//
//	go run ./examples/validate_dataset ./path/to/dataset
//
// Or use bundled testdata after `make e2e`:
//
//	go run ./examples/validate_dataset ./testdata/output/v30
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/ioai-tech/lerobot-go/lerobot"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <dataset-root>\n", os.Args[0])
		os.Exit(2)
	}
	root := os.Args[1]
	ctx := context.Background()

	insp := lerobot.NewInspector()
	report, err := insp.Validate(ctx, root)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("version: %s\n", report.Version)
	fmt.Println(report.Summary)
	if len(report.Warnings) > 0 {
		fmt.Println("warnings:")
		for _, w := range report.Warnings {
			fmt.Printf("  - %s\n", w)
		}
	}
	if !report.OK {
		fmt.Println("errors:")
		for _, e := range report.Errors {
			fmt.Printf("  - %s\n", e)
		}
		os.Exit(1)
	}
	fmt.Println("OK")
}
