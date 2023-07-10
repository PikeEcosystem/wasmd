package cli

import (
	"fmt"
	// "strings"

	"github.com/spf13/cobra"

	"github.com/PikeEcosystem/cosmos-sdk/client"
	// "github.com/PikeEcosystem/cosmos-sdk/client/flags"
	// sdk "github.com/PikeEcosystem/cosmos-sdk/types"

	"github.com/PikeEcosystem/wasmd/x/random/types"
)

// GetQueryCmd returns the cli query commands for this module
func GetQueryCmd(queryRoute string) *cobra.Command {
	// Group random queries under a subcommand
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      fmt.Sprintf("Querying commands for the %s module", types.ModuleName),
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(CmdListRandomval())
	cmd.AddCommand(CmdShowRandomval())
	cmd.AddCommand(CmdListUserval())
	cmd.AddCommand(CmdShowUserval())
	cmd.AddCommand(CmdVerifyValues())

	cmd.AddCommand(CmdListSentRandomval())
	cmd.AddCommand(CmdShowSentRandomval())
	cmd.AddCommand(CmdListTimedoutRandomval())
	cmd.AddCommand(CmdShowTimedoutRandomval())
	// this line is used by starport scaffolding # 1

	return cmd
}
