package ibctesting

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	protoversion "github.com/tendermint/tendermint/proto/tendermint/version"

	"github.com/PikeEcosystem/cosmos-sdk/client"
	"github.com/PikeEcosystem/cosmos-sdk/codec"
	"github.com/PikeEcosystem/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/PikeEcosystem/cosmos-sdk/crypto/types"
	sdk "github.com/PikeEcosystem/cosmos-sdk/types"
	sdkerrors "github.com/PikeEcosystem/cosmos-sdk/types/errors"
	authtypes "github.com/PikeEcosystem/cosmos-sdk/x/auth/types"
	banktypes "github.com/PikeEcosystem/cosmos-sdk/x/bank/types"
	capabilitykeeper "github.com/PikeEcosystem/cosmos-sdk/x/capability/keeper"
	capabilitytypes "github.com/PikeEcosystem/cosmos-sdk/x/capability/types"
	"github.com/PikeEcosystem/cosmos-sdk/x/staking/teststaking"
	stakingtypes "github.com/PikeEcosystem/cosmos-sdk/x/staking/types"
	clienttypes "github.com/PikeEcosystem/ibc-go/v3/modules/core/02-client/types"
	channeltypes "github.com/PikeEcosystem/ibc-go/v3/modules/core/04-channel/types"
	commitmenttypes "github.com/PikeEcosystem/ibc-go/v3/modules/core/23-commitment/types"
	host "github.com/PikeEcosystem/ibc-go/v3/modules/core/24-host"
	"github.com/PikeEcosystem/ibc-go/v3/modules/core/exported"
	"github.com/PikeEcosystem/ibc-go/v3/modules/core/types"
	ibctmtypes "github.com/PikeEcosystem/ibc-go/v3/modules/light-clients/07-tendermint/types"
	"github.com/PikeEcosystem/ibc-go/v3/testing/mock"
	tmabci "github.com/PikeEcosystem/tendermint/abci/types"
	"github.com/PikeEcosystem/tendermint/crypto/tmhash"
	octypes "github.com/PikeEcosystem/tendermint/types"
	ocversion "github.com/PikeEcosystem/tendermint/version"

	"github.com/PikeEcosystem/wasmd/app"
	"github.com/PikeEcosystem/wasmd/app/params"
	"github.com/PikeEcosystem/wasmd/x/wasm"
)

// TestChain is a testing struct that wraps a simapp with the last TM Header, the current ABCI
// header and the validators of the TestChain. It also contains a field called ChainID. This
// is the clientID that *other* chains use to refer to this TestChain. The SenderAccount
// is used for delivering transactions through the application state.
// NOTE: the actual application uses an empty chain-id for ease of testing.
type TestChain struct {
	t *testing.T

	Coordinator   *Coordinator
	App           *app.WasmApp
	ChainID       string
	LastHeader    *ibctmtypes.Header // header for last block height committed
	CurrentHeader tmproto.Header     // header for current block height
	QueryServer   types.QueryServer
	TxConfig      client.TxConfig
	Codec         codec.BinaryCodec

	Vals    *octypes.ValidatorSet
	Signers []octypes.PrivValidator

	SenderPrivKey cryptotypes.PrivKey
	SenderAccount authtypes.AccountI

	PendingSendPackets []channeltypes.Packet
	PendingAckPackets  []PacketAck
}

type PacketAck struct {
	Packet channeltypes.Packet
	Ack    []byte
}

// NewTestChain initializes a new TestChain instance with a single validator set using a
// generated private key. It also creates a sender account to be used for delivering transactions.
//
// The first block height is committed to state in order to allow for client creations on
// counterparty chains. The TestChain will return with a block height starting at 2.
//
// Time management is handled by the Coordinator in order to ensure synchrony between chains.
// Each update of any chain increments the block header time for all chains by 5 seconds.
func NewTestChain(t *testing.T, coord *Coordinator, chainID string, opts ...wasm.Option) *TestChain {
	// generate validator private/public key
	privVal := mock.NewPV()
	pubKey, err := privVal.GetPubKey()
	require.NoError(t, err)

	// create validator set with single validator
	validator := octypes.NewValidator(pubKey, 1)
	valSet := octypes.NewValidatorSet([]*octypes.Validator{validator})
	signers := []octypes.PrivValidator{privVal}

	// generate genesis account
	senderPrivKey := secp256k1.GenPrivKey()
	acc := authtypes.NewBaseAccount(senderPrivKey.PubKey().Address().Bytes(), senderPrivKey.PubKey(), 0, 0)
	amount, ok := sdk.NewIntFromString("10000000000000000000")
	require.True(t, ok)

	balance := banktypes.Balance{
		Address: acc.GetAddress().String(),
		Coins:   sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, amount)),
	}

	wasmApp := app.SetupWithGenesisValSet(t, valSet, []authtypes.GenesisAccount{acc}, opts, balance)

	// create current header and call begin block
	header := tmproto.Header{
		ChainID: chainID,
		Height:  1,
		Time:    coord.CurrentTime.UTC(),
	}

	txConfig := params.MakeEncodingConfig().TxConfig

	// create an account to send transactions from
	chain := &TestChain{
		t:             t,
		Coordinator:   coord,
		ChainID:       chainID,
		App:           wasmApp,
		CurrentHeader: header,
		QueryServer:   wasmApp.IBCKeeper,
		TxConfig:      txConfig,
		Codec:         wasmApp.AppCodec(),
		Vals:          valSet,
		Signers:       signers,
		SenderPrivKey: senderPrivKey,
		SenderAccount: acc,
	}

	coord.CommitBlock(chain)

	return chain
}

// GetContext returns the current context for the application.
func (chain *TestChain) GetContext() sdk.Context {
	return chain.App.BaseApp.NewContext(false, chain.CurrentHeader)
}

// QueryProof performs an abci query with the given key and returns the proto encoded merkle proof
// for the query and the height at which the proof will succeed on a tendermint verifier.
func (chain *TestChain) QueryProof(key []byte) ([]byte, clienttypes.Height) {
	return chain.QueryProofAtHeight(key, chain.App.LastBlockHeight())
}

// QueryProof performs an abci query with the given key and returns the proto encoded merkle proof
// for the query and the height at which the proof will succeed on a tendermint verifier.
func (chain *TestChain) QueryProofAtHeight(key []byte, height int64) ([]byte, clienttypes.Height) {
	res := chain.App.Query(abci.RequestQuery{
		Path:   fmt.Sprintf("store/%s/key", host.StoreKey),
		Height: height - 1,
		Data:   key,
		Prove:  true,
	})

	merkleProof, err := commitmenttypes.ConvertProofs(res.ProofOps)
	require.NoError(chain.t, err)

	proof, err := chain.App.AppCodec().Marshal(&merkleProof)
	require.NoError(chain.t, err)

	revision := clienttypes.ParseChainID(chain.ChainID)

	// proof height + 1 is returned as the proof created corresponds to the height the proof
	// was created in the IAVL tree. Tendermint and subsequently the clients that rely on it
	// have heights 1 above the IAVL tree. Thus we return proof height + 1
	return proof, clienttypes.NewHeight(revision, uint64(res.Height)+1)
}

// QueryUpgradeProof performs an abci query with the given key and returns the proto encoded merkle proof
// for the query and the height at which the proof will succeed on a tendermint verifier.
func (chain *TestChain) QueryUpgradeProof(key []byte, height uint64) ([]byte, clienttypes.Height) {
	res := chain.App.Query(abci.RequestQuery{
		Path:   "store/upgrade/key",
		Height: int64(height - 1),
		Data:   key,
		Prove:  true,
	})

	merkleProof, err := commitmenttypes.ConvertProofs(res.ProofOps)
	require.NoError(chain.t, err)

	proof, err := chain.App.AppCodec().Marshal(&merkleProof)
	require.NoError(chain.t, err)

	revision := clienttypes.ParseChainID(chain.ChainID)

	// proof height + 1 is returned as the proof created corresponds to the height the proof
	// was created in the IAVL tree. Tendermint and subsequently the clients that rely on it
	// have heights 1 above the IAVL tree. Thus we return proof height + 1
	return proof, clienttypes.NewHeight(revision, uint64(res.Height+1))
}

// QueryConsensusStateProof performs an abci query for a consensus state
// stored on the given clientID. The proof and consensusHeight are returned.
func (chain *TestChain) QueryConsensusStateProof(clientID string) ([]byte, clienttypes.Height) {
	clientState := chain.GetClientState(clientID)

	consensusHeight := clientState.GetLatestHeight().(clienttypes.Height)
	consensusKey := host.FullConsensusStateKey(clientID, consensusHeight)
	proofConsensus, _ := chain.QueryProof(consensusKey)

	return proofConsensus, consensusHeight
}

// NextBlock sets the last header to the current header and increments the current header to be
// at the next block height. It does not update the time as that is handled by the Coordinator.
//
// CONTRACT: this function must only be called after app.Commit() occurs
func (chain *TestChain) NextBlock() {
	// set the last header to the current header
	// use nil trusted fields
	chain.LastHeader = chain.CurrentTMClientHeader()

	// increment the current header
	chain.CurrentHeader = tmproto.Header{
		ChainID: chain.ChainID,
		Height:  chain.App.LastBlockHeight() + 1,
		AppHash: chain.App.LastCommitID().Hash,
		// NOTE: the time is increased by the coordinator to maintain time synchrony amongst
		// chains.
		Time:               chain.CurrentHeader.Time,
		ValidatorsHash:     chain.Vals.Hash(),
		NextValidatorsHash: chain.Vals.Hash(),
	}

	chain.App.BeginBlock(tmabci.RequestBeginBlock{Header: chain.CurrentHeader})
}

func (chain *TestChain) CommitBlock() {
	chain.App.EndBlock(abci.RequestEndBlock{Height: chain.CurrentHeader.Height})
	chain.App.Commit()

	chain.App.BeginRecheckTx(tmabci.RequestBeginRecheckTx{Header: chain.CurrentHeader})
	chain.App.EndRecheckTx(tmabci.RequestEndRecheckTx{Height: chain.CurrentHeader.Height})
}

// sendMsgs delivers a transaction through the application without returning the result.
func (chain *TestChain) sendMsgs(msgs ...sdk.Msg) error {
	_, err := chain.SendMsgs(msgs...)
	return err
}

// SendMsgs delivers a transaction through the application. It updates the senders sequence
// number and updates the TestChain's headers. It returns the result and error if one
// occurred.
func (chain *TestChain) SendMsgs(msgs ...sdk.Msg) (*sdk.Result, error) {
	// ensure the chain has the latest time
	chain.Coordinator.UpdateTimeForChain(chain)

	_, r, err := app.SignAndDeliver(
		chain.t,
		chain.TxConfig,
		chain.App.BaseApp,
		chain.GetContext().BlockHeader(),
		msgs,
		chain.ChainID,
		[]uint64{chain.SenderAccount.GetAccountNumber()},
		[]uint64{chain.SenderAccount.GetSequence()},
		true, true, chain.SenderPrivKey,
	)
	if err != nil {
		return nil, err
	}

	// SignAndDeliver calls app.Commit()
	chain.NextBlock()

	// increment sequence for successful transaction execution
	err = chain.SenderAccount.SetSequence(chain.SenderAccount.GetSequence() + 1)
	if err != nil {
		return nil, err
	}

	chain.Coordinator.IncrementTime()

	chain.captureIBCEvents(r)

	return r, nil
}

func (chain *TestChain) captureIBCEvents(r *sdk.Result) {
	toSend := getSendPackets(r.Events)
	if len(toSend) > 0 {
		// Keep a queue on the chain that we can relay in tests
		chain.PendingSendPackets = append(chain.PendingSendPackets, toSend...)
	}
	toAck := getAckPackets(r.Events)
	if len(toAck) > 0 {
		// Keep a queue on the chain that we can relay in tests
		chain.PendingAckPackets = append(chain.PendingAckPackets, toAck...)
	}
}

// GetClientState retrieves the client state for the provided clientID. The client is
// expected to exist otherwise testing will fail.
func (chain *TestChain) GetClientState(clientID string) exported.ClientState {
	clientState, found := chain.App.IBCKeeper.ClientKeeper.GetClientState(chain.GetContext(), clientID)
	require.True(chain.t, found)

	return clientState
}

// GetConsensusState retrieves the consensus state for the provided clientID and height.
// It will return a success boolean depending on if consensus state exists or not.
func (chain *TestChain) GetConsensusState(clientID string, height exported.Height) (exported.ConsensusState, bool) {
	return chain.App.IBCKeeper.ClientKeeper.GetClientConsensusState(chain.GetContext(), clientID, height)
}

// GetValsAtHeight will return the validator set of the chain at a given height. It will return
// a success boolean depending on if the validator set exists or not at that height.
func (chain *TestChain) GetValsAtHeight(height int64) (*octypes.ValidatorSet, bool) {
	histInfo, ok := chain.App.StakingKeeper.GetHistoricalInfo(chain.GetContext(), height)
	if !ok {
		return nil, false
	}

	valSet := stakingtypes.Validators(histInfo.Valset)

	tmValidators, err := teststaking.ToOcValidators(valSet, sdk.DefaultPowerReduction)
	if err != nil {
		panic(err)
	}
	return octypes.NewValidatorSet(tmValidators), true
}

// GetAcknowledgement retrieves an acknowledgement for the provided packet. If the
// acknowledgement does not exist then testing will fail.
func (chain *TestChain) GetAcknowledgement(packet exported.PacketI) []byte {
	ack, found := chain.App.IBCKeeper.ChannelKeeper.GetPacketAcknowledgement(chain.GetContext(), packet.GetDestPort(), packet.GetDestChannel(), packet.GetSequence())
	require.True(chain.t, found)

	return ack
}

// GetPrefix returns the prefix for used by a chain in connection creation
func (chain *TestChain) GetPrefix() commitmenttypes.MerklePrefix {
	return commitmenttypes.NewMerklePrefix(chain.App.IBCKeeper.ConnectionKeeper.GetCommitmentPrefix().Bytes())
}

// ConstructUpdateTMClientHeader will construct a valid 07-tendermint Header to update the
// light client on the source chain.
func (chain *TestChain) ConstructUpdateTMClientHeader(counterparty *TestChain, clientID string) (*ibctmtypes.Header, error) {
	return chain.ConstructUpdateTMClientHeaderWithTrustedHeight(counterparty, clientID, clienttypes.ZeroHeight())
}

// ConstructUpdateTMClientHeaderWithTrustedHeight will construct a valid 07-tendermint Header to update the
// light client on the source chain.
func (chain *TestChain) ConstructUpdateTMClientHeaderWithTrustedHeight(counterparty *TestChain, clientID string, trustedHeight clienttypes.Height) (*ibctmtypes.Header, error) {
	header := counterparty.LastHeader
	// Relayer must query for LatestHeight on client to get TrustedHeight if the trusted height is not set
	if trustedHeight.IsZero() {
		trustedHeight = chain.GetClientState(clientID).GetLatestHeight().(clienttypes.Height)
	}
	var (
		tmTrustedVals *octypes.ValidatorSet
		ok            bool
	)
	// Once we get TrustedHeight from client, we must query the validators from the counterparty chain
	// If the LatestHeight == LastHeader.Height, then TrustedValidators are current validators
	// If LatestHeight < LastHeader.Height, we can query the historical validator set from HistoricalInfo
	if trustedHeight == counterparty.LastHeader.GetHeight() {
		tmTrustedVals = counterparty.Vals
	} else {
		// NOTE: We need to get validators from counterparty at height: trustedHeight+1
		// since the last trusted validators for a header at height h
		// is the NextValidators at h+1 committed to in header h by
		// NextValidatorsHash
		tmTrustedVals, ok = counterparty.GetValsAtHeight(int64(trustedHeight.RevisionHeight + 1))
		if !ok {
			return nil, sdkerrors.Wrapf(ibctmtypes.ErrInvalidHeaderHeight, "could not retrieve trusted validators at trustedHeight: %d", trustedHeight)
		}
	}
	// inject trusted fields into last header
	// for now assume revision number is 0
	header.TrustedHeight = trustedHeight

	trustedVals, err := tmTrustedVals.ToProto()
	if err != nil {
		return nil, err
	}
	header.TrustedValidators = trustedVals

	return header, nil
}

// ExpireClient fast forwards the chain's block time by the provided amount of time which will
// expire any clients with a trusting period less than or equal to this amount of time.
func (chain *TestChain) ExpireClient(amount time.Duration) {
	chain.Coordinator.IncrementTimeBy(amount)
}

// CurrentTMClientHeader creates a TM header using the current header parameters
// on the chain. The trusted fields in the header are set to nil.
func (chain *TestChain) CurrentTMClientHeader() *ibctmtypes.Header {
	return chain.CreateTMClientHeader(chain.ChainID, chain.CurrentHeader.Height, clienttypes.Height{}, chain.CurrentHeader.Time, chain.Vals, nil, chain.Signers)
}

// CreateTMClientHeader creates a TM header to update the TM client. Args are passed in to allow
// caller flexibility to use params that differ from the chain.
func (chain *TestChain) CreateTMClientHeader(chainID string, blockHeight int64, trustedHeight clienttypes.Height, timestamp time.Time, tmValSet, tmTrustedVals *octypes.ValidatorSet, signers []octypes.PrivValidator) *ibctmtypes.Header {
	var (
		valSet      *tmproto.ValidatorSet
		trustedVals *tmproto.ValidatorSet
	)
	require.NotNil(chain.t, tmValSet)

	vsetHash := tmValSet.Hash()

	tmHeader := octypes.Header{
		Version:            protoversion.Consensus{Block: ocversion.BlockProtocol, App: 2},
		ChainID:            chainID,
		Height:             blockHeight,
		Time:               timestamp,
		LastBlockID:        MakeBlockID(make([]byte, tmhash.Size), 10_000, make([]byte, tmhash.Size)),
		LastCommitHash:     chain.App.LastCommitID().Hash,
		DataHash:           tmhash.Sum([]byte("data_hash")),
		ValidatorsHash:     vsetHash,
		NextValidatorsHash: vsetHash,
		ConsensusHash:      tmhash.Sum([]byte("consensus_hash")),
		AppHash:            chain.CurrentHeader.AppHash,
		LastResultsHash:    tmhash.Sum([]byte("last_results_hash")),
		EvidenceHash:       tmhash.Sum([]byte("evidence_hash")),
		ProposerAddress:    tmValSet.SelectProposer([]byte{}, blockHeight, 0).Address, //nolint:staticcheck
	}
	hhash := tmHeader.Hash()
	blockID := MakeBlockID(hhash, 3, tmhash.Sum([]byte("part_set")))
	voteSet := octypes.NewVoteSet(chainID, blockHeight, 1, tmproto.PrecommitType, tmValSet)

	commit, err := octypes.MakeCommit(blockID, blockHeight, 1, voteSet, signers, timestamp)
	require.NoError(chain.t, err)

	signedHeader := &tmproto.SignedHeader{
		Header: tmHeader.ToProto(),
		Commit: commit.ToProto(),
	}

	if tmValSet != nil {
		valSet, err = tmValSet.ToProto()
		require.NoError(chain.t, err)
	}

	if tmTrustedVals != nil {
		trustedVals, err = tmTrustedVals.ToProto()
		require.NoError(chain.t, err)
	}

	// The trusted fields may be nil. They may be filled before relaying messages to a client.
	// The relayer is responsible for querying client and injecting appropriate trusted fields.
	return &ibctmtypes.Header{
		SignedHeader:      signedHeader,
		ValidatorSet:      valSet,
		TrustedHeight:     trustedHeight,
		TrustedValidators: trustedVals,
	}
}

// MakeBlockID copied unimported test functions from octypes to use them here
func MakeBlockID(hash []byte, partSetSize uint32, partSetHash []byte) octypes.BlockID {
	return octypes.BlockID{
		Hash: hash,
		PartSetHeader: octypes.PartSetHeader{
			Total: partSetSize,
			Hash:  partSetHash,
		},
	}
}

// CreateSortedSignerArray takes two PrivValidators, and the corresponding Validator structs
// (including voting power). It returns a signer array of PrivValidators that matches the
// sorting of ValidatorSet.
// The sorting is first by .VotingPower (descending), with secondary index of .Address (ascending).
func CreateSortedSignerArray(altPrivVal, suitePrivVal octypes.PrivValidator,
	altVal, suiteVal *octypes.Validator,
) []octypes.PrivValidator {
	switch {
	case altVal.VotingPower > suiteVal.VotingPower:
		return []octypes.PrivValidator{altPrivVal, suitePrivVal}
	case altVal.VotingPower < suiteVal.VotingPower:
		return []octypes.PrivValidator{suitePrivVal, altPrivVal}
	default:
		if bytes.Compare(altVal.Address, suiteVal.Address) == -1 {
			return []octypes.PrivValidator{altPrivVal, suitePrivVal}
		}
		return []octypes.PrivValidator{suitePrivVal, altPrivVal}
	}
}

// CreatePortCapability binds and claims a capability for the given portID if it does not
// already exist. This function will fail testing on any resulting error.
// NOTE: only creation of a capbility for a transfer or mock port is supported
// Other applications must bind to the port in InitGenesis or modify this code.
func (chain *TestChain) CreatePortCapability(scopedKeeper capabilitykeeper.ScopedKeeper, portID string) {
	// check if the portId is already binded, if not bind it
	_, ok := chain.App.ScopedIBCKeeper.GetCapability(chain.GetContext(), host.PortPath(portID))
	if !ok {
		// create capability using the IBC capability keeper
		cap, err := chain.App.ScopedIBCKeeper.NewCapability(chain.GetContext(), host.PortPath(portID))
		require.NoError(chain.t, err)

		// claim capability using the scopedKeeper
		err = scopedKeeper.ClaimCapability(chain.GetContext(), cap, host.PortPath(portID))
		require.NoError(chain.t, err)
	}

	chain.App.Commit()

	chain.NextBlock()
}

// GetPortCapability returns the port capability for the given portID. The capability must
// exist, otherwise testing will fail.
func (chain *TestChain) GetPortCapability(portID string) *capabilitytypes.Capability {
	cap, ok := chain.App.ScopedIBCKeeper.GetCapability(chain.GetContext(), host.PortPath(portID))
	require.True(chain.t, ok)

	return cap
}

// CreateChannelCapability binds and claims a capability for the given portID and channelID
// if it does not already exist. This function will fail testing on any resulting error. The
// scoped keeper passed in will claim the new capability.
func (chain *TestChain) CreateChannelCapability(scopedKeeper capabilitykeeper.ScopedKeeper, portID, channelID string) {
	capName := host.ChannelCapabilityPath(portID, channelID)
	// check if the portId is already binded, if not bind it
	_, ok := chain.App.ScopedIBCKeeper.GetCapability(chain.GetContext(), capName)
	if !ok {
		cap, err := chain.App.ScopedIBCKeeper.NewCapability(chain.GetContext(), capName)
		require.NoError(chain.t, err)
		err = scopedKeeper.ClaimCapability(chain.GetContext(), cap, capName)
		require.NoError(chain.t, err)
	}

	chain.App.Commit()

	chain.NextBlock()
}

// GetChannelCapability returns the channel capability for the given portID and channelID.
// The capability must exist, otherwise testing will fail.
func (chain *TestChain) GetChannelCapability(portID, channelID string) *capabilitytypes.Capability {
	cap, ok := chain.App.ScopedIBCKeeper.GetCapability(chain.GetContext(), host.ChannelCapabilityPath(portID, channelID))
	require.True(chain.t, ok)

	return cap
}

func (chain *TestChain) Balance(acc sdk.AccAddress, denom string) sdk.Coin {
	return chain.App.BankKeeper.GetBalance(chain.GetContext(), acc, denom)
}

func (chain *TestChain) AllBalances(acc sdk.AccAddress) sdk.Coins {
	return chain.App.BankKeeper.GetAllBalances(chain.GetContext(), acc)
}