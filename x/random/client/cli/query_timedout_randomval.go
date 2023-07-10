package cli

import (
	"context"
	"strconv"

	"github.com/PikeEcosystem/cosmos-sdk/client"
	"github.com/PikeEcosystem/cosmos-sdk/client/flags"
	"github.com/PikeEcosystem/wasmd/x/random/types"
	"github.com/spf13/cobra"
)

func CmdListTimedoutRandomval() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-timedout-randomval",
		Short: "list all timedoutRandomval",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)

			pageReq, err := client.ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			queryClient := types.NewQueryClient(clientCtx)

			params := &types.QueryAllTimedoutRandomvalRequest{
				Pagination: pageReq,
			}

			res, err := queryClient.TimedoutRandomvalAll(context.Background(), params)
			if err != nil {
				return err
			}

			return clientCtx.PrintProto(res)
		},
	}

	flags.AddPaginationFlagsToCmd(cmd, cmd.Use)
	flags.AddQueryFlagsToCmd(cmd)

	return cmd
}

func CmdShowTimedoutRandomval() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show-timedout-randomval [id]",
		Short: "shows a timedoutRandomval",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)

			queryClient := types.NewQueryClient(clientCtx)

			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return err
			}

			params := &types.QueryGetTimedoutRandomvalRequest{
				Id: id,
			}

			res, err := queryClient.TimedoutRandomval(context.Background(), params)
			if err != nil {
				return err
			}

			return clientCtx.PrintProto(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)

	return cmd
}