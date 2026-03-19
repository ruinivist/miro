package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// Run executes the miro CLI and returns a process exit code.
func Run(args []string) int {
	if err := ensureDependencies(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	rootCmd := newRootCommand()
	rootCmd.SetArgs(args)

	if err := rootCmd.Execute(); err != nil {
		return 1
	}

	return 0
}

func newRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "miro",
		Short: "A lean CLI E2E testing framework.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	rootCmd.AddCommand(newInitCommand(), newRecordCommand(), newTestCommand())

	return rootCmd
}

func ensureDependencies() error {
	for _, name := range []string{"script", "bwrap", "bash"} {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("required command %q not found in PATH", name)
		}
	}

	return nil
}
