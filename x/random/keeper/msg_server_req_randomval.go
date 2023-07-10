package keeper

import (
	"context"

	sdk "github.com/PikeEcosystem/cosmos-sdk/types"
	clienttypes "github.com/PikeEcosystem/ibc-go/v3/modules/core/02-client/types"
	"github.com/PikeEcosystem/wasmd/x/random/types"
)

func (k msgServer) SendReqRandomval(goCtx context.Context, msg *types.MsgSendReqRandomval) (*types.MsgSendReqRandomvalResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	// TODO: logic before transmitting the packet

	// Construct the packet
	var packet types.ReqRandomvalPacketData

	packet.Multiplier = msg.Multiplier
	packet.Creator = msg.Creator

	// Transmit the packet
	err := k.TransmitReqRandomvalPacket(
		ctx,
		packet,
		msg.Port,
		msg.ChannelID,
		clienttypes.ZeroHeight(),
		msg.TimeoutTimestamp,
	)
	if err != nil {
		return nil, err
	}

	return &types.MsgSendReqRandomvalResponse{}, nil
}
