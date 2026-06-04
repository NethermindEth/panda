package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/ethpandaops/panda/internal/version"
)

var versionCmd = &cobra.Command{
	GroupID: groupSetup,
	Use:     "version",
	Short:   "Print version information",
	RunE: func(_ *cobra.Command, _ []string) error {
		return version.Fprint(os.Stdout, "panda", isJSON())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
