package cli

import "github.com/spf13/cobra"

func newReviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "review",
		Short: "Review a plan or phase",
	}
}
