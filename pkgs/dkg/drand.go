package dkg

import (
	"bytes"
	"crypto/rsa"
	"encoding/json"
	"fmt"

	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/drand/kyber"
	"github.com/drand/kyber/pairing"
	"github.com/drand/kyber/share/dkg"
	kyber_dkg "github.com/drand/kyber/share/dkg"
	"github.com/drand/kyber/util/random"
	"github.com/ethereum/go-ethereum/common"
	eth_crypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/herumi/bls-eth-go-binary/bls"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/bloxapp/ssv-dkg/pkgs/board"
	"github.com/bloxapp/ssv-dkg/pkgs/crypto"
	"github.com/bloxapp/ssv-dkg/pkgs/utils"
	"github.com/bloxapp/ssv-dkg/pkgs/wire"
)

const (
	// MaxEffectiveBalanceInGwei is the max effective balance
	MaxEffectiveBalanceInGwei phase0.Gwei = 32000000000
)

// Operator structure contains information about external operator participating in the DKG ceremony
type Operator struct {
	IP     string
	ID     uint64
	Pubkey *rsa.PublicKey
}

// DKGdata structure to store at LocalOwner information about initial message parameters and secret scalar to be used as input for DKG protocol
type DKGdata struct {
	// Request ID formed by initiator to identify DKG ceremony
	reqID [24]byte
	// initial message from initiator
	init *wire.Init
	// Randomly generated scalar to be used for DKG ceremony
	secret kyber.Scalar
	// reshare message from initiator
	reshare *wire.Reshare
}

// Result is the last message in every DKG which marks a specific node's end of process
type Result struct {
	// Operator ID
	OperatorID uint64
	// Operator RSA pubkey
	PubKeyRSA *rsa.PublicKey
	// RequestID for the DKG instance (not used for signing)
	RequestID [24]byte
	// EncryptedShare standard SSV encrypted shares
	EncryptedShare []byte
	// SharePubKey is the share's BLS pubkey
	SharePubKey []byte
	// ValidatorPubKey the resulting public key corresponding to the shared private key
	ValidatorPubKey []byte
	// Partial Operator Signature of Deposit data
	DepositPartialSignature []byte
	// SSV owner + nonce signature
	OwnerNoncePartialSignature []byte
	// Public poly commitments
	Commits []byte
}

// Encode returns a msg encoded bytes or error
func (msg *Result) Encode() ([]byte, error) {
	return json.Marshal(msg)
}

// Decode returns error if decoding failed
func (msg *Result) Decode(data []byte) error {
	return json.Unmarshal(data, msg)
}

// OwnerOpts structure to pass parameters from Switch to LocalOwner structure
type OwnerOpts struct {
	Logger               *zap.Logger
	ID                   uint64
	BroadcastF           func([]byte) error
	Suite                pairing.Suite
	VerifyFunc           func(id uint64, msg, sig []byte) error
	SignFunc             func([]byte) ([]byte, error)
	EncryptFunc          func([]byte) ([]byte, error)
	DecryptFunc          func([]byte) ([]byte, error)
	StoreSecretShareFunc func(reqID [24]byte, pubKey []byte, key *kyber_dkg.DistKeyShare) error
	RSAPub               *rsa.PublicKey
	Owner                [20]byte
	Nonce                uint64
	Version              []byte
}

type PriShare struct {
	I int    `json:"index"`
	V []byte `json:"secret_point"`
}

type DistKeyShare struct {
	Commits []byte   `json:"commits"`
	Share   PriShare `json:"secret_share"`
}

// Encode returns a msg encoded bytes or error
func (msg *DistKeyShare) Encode() ([]byte, error) {
	return json.Marshal(msg)
}

// Decode returns error if decoding failed
func (msg *DistKeyShare) Decode(data []byte) error {
	return json.Unmarshal(data, msg)
}

var ErrAlreadyExists = errors.New("duplicate message")

// LocalOwner as a main structure created for a new DKG initiation or resharing ceremony
type LocalOwner struct {
	Logger           *zap.Logger
	startedDKG       chan struct{}
	ErrorChan        chan error
	ID               uint64
	data             *DKGdata
	board            *board.Board
	Suite            pairing.Suite
	broadcastF       func([]byte) error
	exchanges        map[uint64]*wire.Exchange
	deals            map[uint64]*kyber_dkg.DealBundle
	verifyFunc       func(id uint64, msg, sig []byte) error
	signFunc         func([]byte) ([]byte, error)
	encryptFunc      func([]byte) ([]byte, error)
	decryptFunc      func([]byte) ([]byte, error)
	storeSecretShare func(reqID [24]byte, pubKey []byte, key *kyber_dkg.DistKeyShare) error
	SecretShare      *kyber_dkg.DistKeyShare
	initiatorRSAPub  *rsa.PublicKey
	RSAPub           *rsa.PublicKey
	owner            common.Address
	nonce            uint64
	done             chan struct{}
	version          []byte
}

// New creates a LocalOwner structure. We create it for each new DKG ceremony.
func New(opts OwnerOpts) *LocalOwner {
	owner := &LocalOwner{
		Logger:           opts.Logger,
		startedDKG:       make(chan struct{}, 1),
		ErrorChan:        make(chan error, 1),
		ID:               opts.ID,
		broadcastF:       opts.BroadcastF,
		exchanges:        make(map[uint64]*wire.Exchange),
		deals:            make(map[uint64]*kyber_dkg.DealBundle),
		signFunc:         opts.SignFunc,
		verifyFunc:       opts.VerifyFunc,
		encryptFunc:      opts.EncryptFunc,
		decryptFunc:      opts.DecryptFunc,
		storeSecretShare: opts.StoreSecretShareFunc,
		RSAPub:           opts.RSAPub,
		done:             make(chan struct{}, 1),
		Suite:            opts.Suite,
		owner:            opts.Owner,
		nonce:            opts.Nonce,
		version:          opts.Version,
	}
	return owner
}

// StartDKG initializes and starts DKG protocol
func (o *LocalOwner) StartDKG() error {
	o.Logger.Info("Starting DKG")
	nodes := make([]kyber_dkg.Node, 0)
	// Create nodes using public points of all operators participating in the protocol
	// Each operator creates a random secret/public points at G1 when initiating new LocalOwner instance
	for id, e := range o.exchanges {
		p := o.Suite.G1().Point()
		if err := p.UnmarshalBinary(e.PK); err != nil {
			return err
		}

		nodes = append(nodes, kyber_dkg.Node{
			Index:  kyber_dkg.Index(id - 1),
			Public: p,
		})
	}
	// New protocol
	p, err := wire.NewDKGProtocol(&wire.Config{
		Identifier: o.data.reqID[:],
		Secret:     o.data.secret,
		NewNodes:   nodes,
		Suite:      o.Suite,
		T:          int(o.data.init.T),
		Board:      o.board,
		Logger:     o.Logger,
	})
	if err != nil {
		return err
	}
	// Wait when the protocol exchanges finish and process the result
	go func(p *kyber_dkg.Protocol, postF func(res *kyber_dkg.OptionResult) error) {
		res := <-p.WaitEnd()
		if err := postF(&res); err != nil {
			o.Logger.Error("Error in PostDKG function", zap.Error(err))
		}
	}(p, o.PostDKG)
	close(o.startedDKG)
	return nil
}

func (o *LocalOwner) StartReshareDKGOldNodes() error {
	o.Logger.Info("Starting Resharing DKG ceremony at old nodes")
	NewNodes, err := o.GetDKGNodes(o.data.reshare.NewOperators)
	if err != nil {
		return err
	}
	OldNodes, err := o.GetDKGNodes(o.data.reshare.OldOperators)
	if err != nil {
		return err
	}
	// New protocol
	logger := o.Logger.With(zap.Uint64("ID", o.ID))
	p, err := wire.NewReshareProtocolOldNodes(&wire.Config{
		Identifier: o.data.reqID[:],
		Secret:     o.data.secret,
		OldNodes:   OldNodes,
		NewNodes:   NewNodes,
		Suite:      o.Suite,
		T:          int(o.data.reshare.OldT),
		NewT:       int(o.data.reshare.NewT),
		Board:      o.board,
		Share:      o.SecretShare,
		Logger:     logger,
	})
	if err != nil {
		return err
	}

	go func(p *kyber_dkg.Protocol, postF func(res *kyber_dkg.OptionResult) error) {
		res := <-p.WaitEnd()
		if err := postF(&res); err != nil {
			o.Logger.Error("Error in postReshare function", zap.Error(err))
		}
	}(p, o.postReshare)
	close(o.startedDKG)
	return nil
}

func (o *LocalOwner) StartReshareDKGNewNodes() error {
	o.Logger.Info("Starting Resharing DKG ceremony at new nodes")
	NewNodes, err := o.GetDKGNodes(o.data.reshare.NewOperators)
	if err != nil {
		return err
	}
	OldNodes := make([]kyber_dkg.Node, 0)
	var commits []byte
	for _, op := range o.data.reshare.OldOperators {
		if o.exchanges[op.ID] == nil {
			return fmt.Errorf("no operator at exchanges")
		}
		e := o.exchanges[op.ID]
		if e.Commits == nil {
			return fmt.Errorf("no commits at exchanges")
		}
		commits = e.Commits
		p := o.Suite.G1().Point()
		if err := p.UnmarshalBinary(e.PK); err != nil {
			return err
		}

		OldNodes = append(OldNodes, kyber_dkg.Node{
			Index:  kyber_dkg.Index(op.ID - 1),
			Public: p,
		})
	}
	var coefs []kyber.Point
	coefsBytes := utils.SplitBytes(commits, 48)
	for _, c := range coefsBytes {
		p := o.Suite.G1().Point()
		err := p.UnmarshalBinary(c)
		if err != nil {
			return err
		}
		coefs = append(coefs, p)
	}

	// New protocol
	logger := o.Logger.With(zap.Uint64("ID", o.ID))
	p, err := wire.NewReshareProtocolNewNodes(&wire.Config{
		Identifier:   o.data.reqID[:],
		Secret:       o.data.secret,
		OldNodes:     OldNodes,
		NewNodes:     NewNodes,
		Suite:        o.Suite,
		T:            int(o.data.reshare.OldT),
		NewT:         int(o.data.reshare.NewT),
		Board:        o.board,
		PublicCoeffs: coefs,
		Logger:       logger,
	})
	if err != nil {
		return err
	}
	for _, b := range o.deals {
		o.board.DealC <- *b
	}
	go func(p *kyber_dkg.Protocol, postF func(res *kyber_dkg.OptionResult) error) {
		res := <-p.WaitEnd()
		if err := postF(&res); err != nil {
			o.Logger.Error("Error in postReshare function", zap.Error(err))
		}
	}(p, o.postReshare)
	return nil
}

func (o *LocalOwner) PushDealsOldNodes() error {
	for _, b := range o.deals {
		o.board.DealC <- *b
	}
	return nil
}

// Function to send signed messages back to initiator
func (o *LocalOwner) Broadcast(ts *wire.Transport) error {
	bts, err := ts.MarshalSSZ()
	if err != nil {
		return err
	}
	// Sign message with RSA private key
	sign, err := o.signFunc(bts)
	if err != nil {
		return err
	}

	signed := &wire.SignedTransport{
		Message:   ts,
		Signer:    o.ID,
		Signature: sign,
	}

	final, err := signed.MarshalSSZ()
	if err != nil {
		return err
	}

	return o.broadcastF(final)
}

// PostDKG stores the resulting key share, convert it to BLS points acceptable by ETH2
// and creates the Result structure to send back to initiator
func (o *LocalOwner) PostDKG(res *kyber_dkg.OptionResult) error {
	if res.Error != nil {
		o.broadcastError(res.Error)
		return fmt.Errorf("dkg protocol failed: %w", res.Error)
	}
	o.Logger.Info("DKG ceremony finished successfully")
	// Store result share a instance
	o.SecretShare = res.Result.Key
	if err := o.storeSecretShare(o.data.reqID, o.data.init.InitiatorPublicKey, res.Result.Key); err != nil {
		o.broadcastError(err)
		return fmt.Errorf("failed to store secret share: %w", err)
	}
	// Get validator BLS public key from result
	validatorPubKey, err := crypto.ResultToValidatorPK(res.Result, o.Suite.G1().(kyber_dkg.Suite))
	if err != nil {
		o.broadcastError(err)
		return fmt.Errorf("failed to get validator BLS public key: %w", err)
	}
	// Get BLS partial secret key share from DKG
	secretKeyBLS, err := crypto.ResultToShareSecretKey(res.Result)
	if err != nil {
		o.broadcastError(err)
		return fmt.Errorf("failed to get BLS partial secret key share: %w", err)
	}
	// Encrypt BLS share for SSV contract
	ciphertext, err := o.encryptSecretShare(secretKeyBLS)
	if err != nil {
		o.broadcastError(err)
		return fmt.Errorf("failed to encrypt BLS share: %w", err)
	}
	// Sign root
	depositRootSig, signRoot, err := crypto.SignDepositData(secretKeyBLS, o.data.init.WithdrawalCredentials, validatorPubKey, utils.GetNetworkByFork(o.data.init.Fork), MaxEffectiveBalanceInGwei)
	if err != nil {
		o.broadcastError(err)
		return fmt.Errorf("failed to sign deposit data: %w", err)
	}
	// Validate partial signature
	val := depositRootSig.VerifyByte(secretKeyBLS.GetPublicKey(), signRoot)
	if !val {
		o.broadcastError(err)
		return fmt.Errorf("partial deposit root signature is not valid %x", depositRootSig.Serialize())
	}
	// Sign SSV owner + nonce
	data := []byte(fmt.Sprintf("%s:%d", o.owner.String(), o.nonce))
	hash := eth_crypto.Keccak256([]byte(data))
	sigOwnerNonce := secretKeyBLS.SignByte(hash)
	// Verify partial SSV owner + nonce signature
	val = sigOwnerNonce.VerifyByte(secretKeyBLS.GetPublicKey(), hash)
	if !val {
		o.broadcastError(err)
		return fmt.Errorf("partial owner + nonce signature isnt valid %x", sigOwnerNonce.Serialize())
	}
	out := Result{
		RequestID:                  o.data.reqID,
		EncryptedShare:             ciphertext,
		SharePubKey:                secretKeyBLS.GetPublicKey().Serialize(),
		ValidatorPubKey:            validatorPubKey.Serialize(),
		DepositPartialSignature:    depositRootSig.Serialize(),
		PubKeyRSA:                  o.RSAPub,
		OperatorID:                 o.ID,
		OwnerNoncePartialSignature: sigOwnerNonce.Serialize(),
		Commits:                    utils.CommitsToBytes(res.Result.Key.Commits),
	}

	encodedOutput, err := out.Encode()
	if err != nil {
		o.broadcastError(err)
		return fmt.Errorf("failed to encode output: %w", err)
	}

	tsMsg := &wire.Transport{
		Type:       wire.OutputMessageType,
		Identifier: o.data.reqID,
		Data:       encodedOutput,
		Version:    o.version,
	}

	o.Broadcast(tsMsg)
	close(o.done)
	return nil
}

func (o *LocalOwner) postReshare(res *kyber_dkg.OptionResult) error {
	if res.Error != nil {
		o.broadcastError(res.Error)
		return res.Error
	}
	o.Logger.Info("DKG resharing ceremony finished successfully")
	// Store result share a instance
	o.SecretShare = res.Result.Key
	if err := o.storeSecretShare(o.data.reqID, o.data.reshare.InitiatorPublicKey, res.Result.Key); err != nil {
		o.broadcastError(err)
		return err
	}
	// Get validator BLS public key from result
	validatorPubKey, err := crypto.ResultToValidatorPK(res.Result, o.Suite.G1().(dkg.Suite))
	if err != nil {
		o.broadcastError(err)
		return err
	}
	// Get BLS partial secret key share from DKG
	secretKeyBLS, err := crypto.ResultToShareSecretKey(res.Result)
	if err != nil {
		o.broadcastError(err)
		return err
	}
	// Encrypt BLS share for SSV contract
	ciphertext, err := o.encryptSecretShare(secretKeyBLS)
	if err != nil {
		o.broadcastError(err)
		return err
	}
	// Sign SSV owner + nonce
	data := []byte(fmt.Sprintf("%s:%d", o.owner.String(), o.nonce))
	hash := eth_crypto.Keccak256([]byte(data))
	sigOwnerNonce := secretKeyBLS.SignByte(hash)
	if err != nil {
		o.broadcastError(err)
		return err
	}
	// Verify partial SSV owner + nonce signature
	val := sigOwnerNonce.VerifyByte(secretKeyBLS.GetPublicKey(), hash)
	if !val {
		o.broadcastError(err)
		return fmt.Errorf("partial owner + nonce signature isnt valid %x", sigOwnerNonce.Serialize())
	}
	out := Result{
		RequestID:                  o.data.reqID,
		EncryptedShare:             ciphertext,
		SharePubKey:                secretKeyBLS.GetPublicKey().Serialize(),
		ValidatorPubKey:            validatorPubKey.Serialize(),
		PubKeyRSA:                  o.RSAPub,
		OperatorID:                 o.ID,
		OwnerNoncePartialSignature: sigOwnerNonce.Serialize(),
		Commits:                    utils.CommitsToBytes(res.Result.Key.Commits),
	}
	encodedOutput, err := out.Encode()
	if err != nil {
		o.broadcastError(err)
		return err
	}
	tsMsg := &wire.Transport{
		Type:       wire.OutputMessageType,
		Identifier: o.data.reqID,
		Data:       encodedOutput,
		Version:    o.version,
	}
	o.Broadcast(tsMsg)
	close(o.done)
	return nil
}

// Init function creates an interface for DKG (board) which process protocol messages
// Here we randomly create a point at G1 as a DKG public key for the node
func (o *LocalOwner) Init(reqID [24]byte, init *wire.Init) (*wire.Transport, error) {
	if o.data == nil {
		o.data = &DKGdata{}
	}
	o.data.init = init
	o.data.reqID = reqID
	kyberLogger := o.Logger.With(zap.String("reqid", fmt.Sprintf("%x", o.data.reqID[:])))
	o.board = board.NewBoard(
		kyberLogger,
		func(msg *wire.KyberMessage) error {
			kyberLogger.Debug("server: broadcasting kyber message")
			byts, err := msg.MarshalSSZ()
			if err != nil {
				return err
			}

			trsp := &wire.Transport{
				Type:       wire.KyberMessageType,
				Identifier: o.data.reqID,
				Data:       byts,
				Version:    o.version,
			}

			// todo not loop with channels
			go func(trsp *wire.Transport) {
				if err := o.Broadcast(trsp); err != nil {
					o.Logger.Error("broadcasting failed", zap.Error(err))
				}
			}(trsp)

			return nil
		},
	)
	// Generate random k scalar (secret) and corresponding public key k*G where G is a G1 generator
	eciesSK, pk := initsecret(o.Suite)
	o.data.secret = eciesSK
	bts, _, err := CreateExchange(pk, nil)
	if err != nil {
		return nil, err
	}
	return o.exchangeWireMessage(bts, reqID), nil
}

// InitReshare initiates a resharing owner of dkg protocol
func (o *LocalOwner) InitReshare(reqID [24]byte, reshare *wire.Reshare, commits []byte) (*wire.Transport, error) {
	if o.data == nil {
		o.data = &DKGdata{}
	}
	o.data.reshare = reshare
	o.data.reqID = reqID
	kyberLogger := o.Logger.With(zap.String("reqid", fmt.Sprintf("%x", o.data.reqID[:])))
	o.board = board.NewBoard(
		kyberLogger,
		func(msg *wire.KyberMessage) error {
			kyberLogger.Debug("server: broadcasting kyber message")
			byts, err := msg.MarshalSSZ()
			if err != nil {
				return err
			}

			trsp := &wire.Transport{
				Type:       wire.ReshareKyberMessageType,
				Identifier: o.data.reqID,
				Data:       byts,
				Version:    o.version,
			}

			// todo not loop with channels
			go func(trsp *wire.Transport) {
				if err := o.Broadcast(trsp); err != nil {
					o.Logger.Error("broadcasting failed", zap.Error(err))
				}
			}(trsp)

			return nil
		},
	)

	eciesSK, pk := initsecret(o.Suite)
	o.data.secret = eciesSK
	bts, _, err := CreateExchange(pk, commits)
	if err != nil {
		return nil, err
	}
	return o.reshareExchangeWireMessage(bts, reqID), nil
}

// processDKG after receiving a kyber message type at /dkg route
// KyberDealBundleMessageType - message that contains all the deals and the public polynomial from participating party
// KyberResponseBundleMessageType - status for the deals received at deal bundle
// KyberJustificationBundleMessageType - all justifications for each complaint for received deals bundles
func (o *LocalOwner) processDKG(from uint64, msg *wire.Transport) error {
	kyberMsg := &wire.KyberMessage{}
	if err := kyberMsg.UnmarshalSSZ(msg.Data); err != nil {
		return err
	}

	o.Logger.Debug("operator: received kyber msg", zap.String("type", kyberMsg.Type.String()), zap.Uint64("from", from))

	switch kyberMsg.Type {
	case wire.KyberDealBundleMessageType:
		b, err := wire.DecodeDealBundle(kyberMsg.Data, o.Suite.G1().(dkg.Suite))
		if err != nil {
			return err
		}
		o.Logger.Debug("operator: received deal bundle from", zap.Uint64("ID", from))
		o.board.DealC <- *b
	case wire.KyberResponseBundleMessageType:

		b, err := wire.DecodeResponseBundle(kyberMsg.Data)
		if err != nil {
			return err
		}
		o.Logger.Debug("operator: received response bundle from", zap.Uint64("ID", from))
		o.board.ResponseC <- *b
	case wire.KyberJustificationBundleMessageType:
		b, err := wire.DecodeJustificationBundle(kyberMsg.Data, o.Suite.G1().(dkg.Suite))
		if err != nil {
			return err
		}
		o.Logger.Debug("operator: received justification bundle from", zap.Uint64("ID", from))
		o.board.JustificationC <- *b
	default:
		return fmt.Errorf("unknown kyber message type")
	}
	return nil
}

// Process processes incoming messages from initiator at /dkg route
func (o *LocalOwner) Process(from uint64, st *wire.SignedTransport) error {
	msgbts, err := st.Message.MarshalSSZ()
	if err != nil {
		return err
	}
	// Verify operator signatures
	if err := o.verifyFunc(st.Signer, msgbts, st.Signature); err != nil {
		return err
	}
	t := st.Message
	o.Logger.Info("✅ Successfully verified incoming DKG", zap.String("message type", t.Type.String()), zap.Uint64("from", st.Signer))
	switch t.Type {
	case wire.ExchangeMessageType:
		exchMsg := &wire.Exchange{}
		if err := exchMsg.UnmarshalSSZ(t.Data); err != nil {
			return err
		}
		if _, ok := o.exchanges[from]; ok {
			return ErrAlreadyExists
		}

		o.exchanges[from] = exchMsg

		// check if have all participating operators pub keys, then start dkg protocol
		if o.checkOperators() {
			if err := o.StartDKG(); err != nil {
				return err
			}
		}
	case wire.ReshareExchangeMessageType:
		exchMsg := &wire.Exchange{}
		if err := exchMsg.UnmarshalSSZ(t.Data); err != nil {
			return err
		}
		if _, ok := o.exchanges[from]; ok {
			return ErrAlreadyExists
		}
		o.exchanges[from] = exchMsg
		allOps := utils.JoinSets(o.data.reshare.OldOperators, o.data.reshare.NewOperators)
		if len(o.exchanges) == len(allOps) {
			for _, op := range o.data.reshare.OldOperators {
				if o.ID == op.ID {
					if err := o.StartReshareDKGOldNodes(); err != nil {
						return err
					}
				}
			}
			for _, op := range utils.GetDisjointNewOperators(o.data.reshare.OldOperators, o.data.reshare.NewOperators) {
				if o.ID != op.ID {
					continue
				}
				bundle := &dkg.DealBundle{}
				b, err := wire.EncodeDealBundle(bundle)
				if err != nil {
					return err
				}
				msg := &wire.ReshareKyberMessage{
					Type: wire.KyberDealBundleMessageType,
					Data: b,
				}

				byts, err := msg.MarshalSSZ()
				if err != nil {
					return err
				}
				trsp := &wire.Transport{
					Type:       wire.ReshareKyberMessageType,
					Identifier: o.data.reqID,
					Data:       byts,
					Version:    o.version,
				}
				o.Broadcast(trsp)
			}
		}
	case wire.ReshareKyberMessageType:
		kyberMsg := &wire.ReshareKyberMessage{}
		if err := kyberMsg.UnmarshalSSZ(t.Data); err != nil {
			return err
		}
		b, err := wire.DecodeDealBundle(kyberMsg.Data, o.Suite.G1().(dkg.Suite))
		if err != nil {
			return err
		}
		if _, ok := o.deals[from]; ok {
			return ErrAlreadyExists
		}
		if len(b.Deals) != 0 {
			o.deals[from] = b
		}
		oldNodes := utils.GetDisjointOldOperators(o.data.reshare.OldOperators, o.data.reshare.NewOperators)
		newNodes := utils.GetDisjointNewOperators(o.data.reshare.OldOperators, o.data.reshare.NewOperators)
		if len(o.deals) == len(o.data.reshare.OldOperators) {
			for _, op := range oldNodes {
				if o.ID == op.ID {
					if err := o.PushDealsOldNodes(); err != nil {
						return err
					}
				}
			}
			for _, op := range newNodes {
				if o.ID == op.ID {
					if err := o.StartReshareDKGNewNodes(); err != nil {
						return err
					}
				}
			}
		}

	case wire.KyberMessageType:
		<-o.startedDKG
		return o.processDKG(from, t)
	default:
		return fmt.Errorf("unknown message type")
	}
	return nil
}

// initsecret generates a random scalar and computes public point k*G where G is a generator of the field
func initsecret(suite pairing.Suite) (kyber.Scalar, kyber.Point) {
	eciesSK := suite.G1().Scalar().Pick(random.New())
	pk := suite.G1().Point().Mul(eciesSK, nil)
	return eciesSK, pk
}

func CreateExchange(pk kyber.Point, commits []byte) ([]byte, *wire.Exchange, error) {
	pkByts, err := pk.MarshalBinary()
	if err != nil {
		return nil, nil, err
	}
	exch := wire.Exchange{
		PK:      pkByts,
		Commits: commits,
	}
	exchByts, err := exch.MarshalSSZ()
	if err != nil {
		return nil, nil, err
	}

	return exchByts, &exch, nil
}

// ExchangeWireMessage creates a transport message with operator DKG public key
func (o *LocalOwner) exchangeWireMessage(exchdata []byte, reqID [24]byte) *wire.Transport {
	return &wire.Transport{
		Type:       wire.ExchangeMessageType,
		Identifier: reqID,
		Data:       exchdata,
		Version:    o.version,
	}
}

func (o *LocalOwner) reshareExchangeWireMessage(exchdata []byte, reqID [24]byte) *wire.Transport {
	return &wire.Transport{
		Type:       wire.ReshareExchangeMessageType,
		Identifier: reqID,
		Data:       exchdata,
		Version:    o.version,
	}
}

// broadcastError propagates the error at operator back to initiator
func (o *LocalOwner) broadcastError(err error) {
	errMsgEnc, _ := json.Marshal(err.Error())
	errMsg := &wire.Transport{
		Type:       wire.ErrorMessageType,
		Identifier: o.data.reqID,
		Data:       errMsgEnc,
		Version:    o.version,
	}
	o.Broadcast(errMsg)
	close(o.done)
}

// checkOperators checks that operator received all participating parties DKG public keys
func (o *LocalOwner) checkOperators() bool {
	for _, op := range o.data.init.Operators {
		if o.exchanges[op.ID] == nil {
			return false
		}
	}
	return true
}

func (o *LocalOwner) GetLocalOwner() *LocalOwner {
	return o
}

// encryptsecretShare encrypts with RSA private key resulting DKG private key share
func (o *LocalOwner) encryptSecretShare(secretKeyBLS *bls.SecretKey) ([]byte, error) {
	rawshare := secretKeyBLS.SerializeToHexStr()
	ciphertext, err := o.encryptFunc([]byte(rawshare))
	if err != nil {
		return nil, fmt.Errorf("cant encrypt private share")
	}
	// check that we encrypt correctly
	sharesecretDecrypted := &bls.SecretKey{}
	decryptedSharePrivateKey, err := o.decryptFunc(ciphertext)
	if err != nil {
		return nil, err
	}
	if err := sharesecretDecrypted.SetHexString(string(decryptedSharePrivateKey)); err != nil {
		return nil, err
	}

	if !bytes.Equal(sharesecretDecrypted.Serialize(), secretKeyBLS.Serialize()) {
		return nil, err
	}
	return ciphertext, nil
}

// GetDKGNodes returns a slice of DKG node instances used for the protocol
func (o *LocalOwner) GetDKGNodes(ops []*wire.Operator) ([]dkg.Node, error) {
	nodes := make([]dkg.Node, 0)
	for _, op := range ops {
		if o.exchanges[op.ID] == nil {
			return nil, fmt.Errorf("no operator at exchanges")
		}
		e := o.exchanges[op.ID]
		p := o.Suite.G1().Point()
		if err := p.UnmarshalBinary(e.PK); err != nil {
			return nil, err
		}

		nodes = append(nodes, dkg.Node{
			Index:  dkg.Index(op.ID - 1),
			Public: p,
		})
	}
	return nodes, nil
}
