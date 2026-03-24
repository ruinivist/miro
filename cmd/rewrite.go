package cmd

import (
	"mire/internal/mire"

	"github.com/spf13/cobra"
)

func newRewriteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "rewrite [path]",
		Short: "Refresh recorded CLI output fixtures",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) == 1 {
				path = args[0]
			}

			if err := mire.Rewrite(path); err != nil {
				cmd.SilenceUsage = true
				return err
			}

			return nil
		},
	}
}
