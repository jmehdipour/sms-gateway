package worker

import "github.com/spf13/cobra"

// NewWorkerCmd returns the parent "worker" command.
func NewWorkerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Run background workers",
	}
	// attach subcommands
	cmd.AddCommand(senderCmd)

	return cmd
}
