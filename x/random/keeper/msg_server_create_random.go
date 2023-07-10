package keeper

import (
	"context"
	"strconv"

	sdk "github.com/PikeEcosystem/cosmos-sdk/types"
	"github.com/PikeEcosystem/wasmd/x/random/types"
)

func (k msgServer) CreateRandom(goCtx context.Context, msg *types.MsgCreateRandom) (*types.MsgCreateRandomResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	randomval, _, err := k.CreateRandomNumber(ctx, msg.Creator, strconv.FormatUint(msg.Multiplier, 10))

	// TODO: Handling the message
	_ = ctx

	return &types.MsgCreateRandomResponse{Id: randomval.Index}, err
}
