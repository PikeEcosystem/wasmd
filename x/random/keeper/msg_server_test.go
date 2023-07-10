package keeper_test

import (
	"context"
	"testing"

	sdk "github.com/PikeEcosystem/cosmos-sdk/types"
	keepertest "github.com/PikeEcosystem/wasmd/testutil/keeper"
	"github.com/PikeEcosystem/wasmd/x/random/keeper"
	"github.com/PikeEcosystem/wasmd/x/random/types"
)

func setupMsgServer(t testing.TB) (types.MsgServer, context.Context) {
	k, ctx := keepertest.RandomKeeper(t)
	return keeper.NewMsgServerImpl(*k), sdk.WrapSDKContext(ctx)
}
