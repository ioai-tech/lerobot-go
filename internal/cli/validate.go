package cli

import (
	"fmt"
	"os"

	"github.com/ioai-tech/lerobot-go/internal/formatcheck"
	"github.com/spf13/cobra"
)

func NewValidateCmd() *cobra.Command {
	var strict, tree bool
	cmd := &cobra.Command{
		Use:   "validate <dataset-root>",
		Short: "Validate LeRobot dataset on-disk format",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := CleanPath(args[0])
			if err != nil {
				return err
			}
			if err := EnsureDatasetRoot(root); err != nil {
				return err
			}
			rep, err := formatcheck.ValidateWithOptions(root, formatcheck.Options{Strict: strict})
			if err != nil {
				return err
			}
			if tree && !Globals.JSON {
				_ = PrintFileTree(root)
			}
			if Globals.JSON {
				return PrintJSON(rep)
			}
			fmt.Printf("version: %s\n", rep.Version)
			fmt.Println(rep.Summary)
			for _, w := range rep.Warnings {
				fmt.Fprintf(os.Stderr, "warning: %s\n", w)
			}
			if !rep.OK {
				for _, e := range rep.Errors {
					fmt.Fprintf(os.Stderr, "error: %s\n", e)
				}
				return fmt.Errorf("validation failed")
			}
			fmt.Println("OK: dataset format valid")
			return nil
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", true, "require data/ and run full layout checks")
	cmd.Flags().BoolVar(&tree, "tree", false, "print file tree on stderr")
	return cmd
}
