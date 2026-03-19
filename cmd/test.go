package cmd

import (
	"miro/internal/miro"

	"github.com/spf13/cobra"
)

func newTestCommand() *cobra.Command {
	return &cobra.Command{
		Use:  "test",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return miro.RunTests()
		},
	}
}
