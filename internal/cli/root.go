package cli

import (
	"github.com/spf13/cobra"
)

// AppVersion is set at link time by GoReleaser (-X flag); dev builds use the default.
var AppVersion = "dev"

var Globals GlobalOpts

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "lerobot-go",
		Short: "LeRobot dataset tools: validate, convert, create, merge",
		Long: `lerobot-go manages LeRobot v2.1 and v3.0 datasets on disk.

Examples:
  lerobot-go validate ./dataset
  lerobot-go convert -i ./v21 -o ./v30 --to v3.0
  lerobot-go create -o ./dataset --version v3.0 --fps 30 --features ./features.json
  lerobot-go merge -o ./merged --to v3.0 -i ./a -i ./b`,
		SilenceUsage: true,
	}
	root.PersistentFlags().BoolVarP(&Globals.Verbose, "verbose", "v", false, "verbose logs on stderr")
	root.PersistentFlags().StringVar(&Globals.FFmpegPath, "ffmpeg-path", "", "path to ffmpeg binary")
	root.PersistentFlags().BoolVar(&Globals.JSON, "json", false, "machine-readable JSON on stdout")

	root.AddGroup(&cobra.Group{ID: "dataset", Title: "Dataset Commands:"})
	root.AddGroup(&cobra.Group{ID: "other", Title: "Other Commands:"})

	validate := NewValidateCmd()
	validate.GroupID = "dataset"
	convert := NewConvertCmd()
	convert.GroupID = "dataset"
	create := NewCreateCmd()
	create.GroupID = "dataset"
	merge := NewMergeCmd()
	merge.GroupID = "dataset"
	version := NewVersionCmd()
	version.GroupID = "other"
	completion := NewCompletionCmd()
	completion.GroupID = "other"

	root.AddCommand(validate, convert, create, merge, version, completion)
	return root
}
