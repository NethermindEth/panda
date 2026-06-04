package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var doraCmd = &cobra.Command{
	GroupID: groupDirect,
	Use:     "dora",
	Short:   "Query Dora beacon chain explorer",
	Long: `Query the Dora beacon chain explorer for network status, validators, and slots.

Examples:
  panda dora networks
  panda dora overview hoodi
  panda dora validator hoodi 12345
  panda dora slot hoodi 1000000
  panda dora epoch hoodi 100`,
}

func init() {
	rootCmd.AddCommand(doraCmd)

	doraCmd.AddCommand(
		doraNetworksCmd,
		doraOverviewCmd,
		doraValidatorCmd,
		doraSlotCmd,
		doraEpochCmd,
	)

	doraOverviewCmd.ValidArgsFunction = completeNetworkNames
	doraValidatorCmd.ValidArgsFunction = completeNetworkNames
	doraSlotCmd.ValidArgsFunction = completeNetworkNames
	doraEpochCmd.ValidArgsFunction = completeNetworkNames
}

var doraNetworksCmd = &cobra.Command{
	Use:   "networks",
	Short: "List networks with Dora explorers",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		response, err := runServerOperation(cmd, "dora.list_networks", map[string]any{})
		if err != nil {
			return err
		}

		return printListing(response, "networks", "No networks with Dora explorers found.")
	},
}

var doraOverviewCmd = &cobra.Command{
	Use:   "overview <network>",
	Short: "Get network overview",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		response, err := runServerOperation(cmd, "dora.get_network_overview", map[string]any{
			"network": args[0],
		})
		if err != nil {
			return err
		}

		if isJSON() {
			return printJSON(response)
		}

		data, _ := response.Data.(map[string]any)

		// Format participation rate as a percentage.
		participationStr := fmt.Sprintf("%v", data["participation_rate"])
		if rate, ok := data["participation_rate"].(float64); ok {
			participationStr = fmt.Sprintf("%.2f%%", rate)
		}

		pairs := [][2]string{
			{"Network", args[0]},
			{"Current epoch", fmt.Sprintf("%v", data["current_epoch"])},
			{"Epoch finalized", fmt.Sprintf("%v", data["finalized"])},
			{"Participation rate", participationStr},
		}

		for _, kv := range [][2]string{
			{"Active validators", "active_validator_count"},
			{"Total validators", "total_validator_count"},
			{"Pending validators", "pending_validator_count"},
			{"Exited validators", "exited_validator_count"},
		} {
			if value, ok := data[kv[1]]; ok {
				pairs = append(pairs, [2]string{kv[0], fmt.Sprintf("%v", value)})
			}
		}

		printKeyValue(pairs)

		return nil
	},
}

var doraValidatorCmd = &cobra.Command{
	Use:   "validator <network> <index-or-pubkey>",
	Short: "Get validator details (always JSON)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		response, err := runServerOperationRaw(cmd, "dora.get_validator", map[string]any{
			"network":         args[0],
			"index_or_pubkey": args[1],
		})
		if err != nil {
			return err
		}

		return printJSONBytes(response.Body)
	},
}

var doraSlotCmd = &cobra.Command{
	Use:   "slot <network> <slot-or-hash>",
	Short: "Get slot details (always JSON)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		response, err := runServerOperationRaw(cmd, "dora.get_slot", map[string]any{
			"network":      args[0],
			"slot_or_hash": args[1],
		})
		if err != nil {
			return err
		}

		return printJSONBytes(response.Body)
	},
}

var doraEpochCmd = &cobra.Command{
	Use:   "epoch <network> <epoch>",
	Short: "Get epoch summary (always JSON)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		response, err := runServerOperationRaw(cmd, "dora.get_epoch", map[string]any{
			"network": args[0],
			"epoch":   args[1],
		})
		if err != nil {
			return err
		}

		return printJSONBytes(response.Body)
	},
}
