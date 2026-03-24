package cmd

import (
	"mire/internal/mire"
	"mire/internal/output"

	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialise mire in the current project",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := mire.Init(); err != nil {
				return err
			}

			output.Println("Done initialising...")
			return nil
		},
	}
}
