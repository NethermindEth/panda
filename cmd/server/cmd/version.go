package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/ethpandaops/panda/internal/version"
)

var versionJSON bool

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information",
	RunE:  runVersion,
}

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "Output in JSON format")
}

func runVersion(_ *cobra.Command, _ []string) error {
	return version.Fprint(os.Stdout, "panda-server", versionJSON)
}
