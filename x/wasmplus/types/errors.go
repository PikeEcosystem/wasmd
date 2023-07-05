package types

import (
	sdkErrors "github.com/PikeEcosystem/cosmos-sdk/types/errors"

	wasmtypes "github.com/PikeEcosystem/wasmd/x/wasm/types"
)

// Codes for wasm contract errors
var (
	// ErrInactiveContract error if the contract set inactive
	ErrInactiveContract = sdkErrors.Register(wasmtypes.DefaultCodespace, 101, "inactive contract")
)
