package keeper

import (
	"github.com/PikeEcosystem/wasmd/x/random/types"
)

var _ types.QueryServer = Keeper{}
