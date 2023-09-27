package operator

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bloxapp/ssv-dkg/pkgs/crypto"
	"github.com/bloxapp/ssv-dkg/pkgs/dkg"
	"github.com/bloxapp/ssv-dkg/pkgs/wire"
	"github.com/drand/kyber"
	bls3 "github.com/drand/kyber-bls12381"
	kyber_dkg "github.com/drand/kyber/share/dkg"
	"go.uber.org/zap"
)

const MaxInstances = 1024
const MaxInstanceTime = 5 * time.Minute

var ErrMissingInstance = errors.New("got message to instance that I don't have, send Init first")
var ErrAlreadyExists = errors.New("got init msg for existing instance")
var ErrMaxInstances = errors.New("max number of instances ongoing, please wait")

type Instance interface {
	Process(uint64, *wire.SignedTransport) error // maybe return resp, threadsafe
	ReadResponse(ctx context.Context) []byte
	ReadError() error
	VerifyInitiatorMessage(msg, sig []byte) error
	GetLocalOwner() *dkg.LocalOwner
}

type instWrapper struct {
	*dkg.LocalOwner
	initiator *rsa.PublicKey
	respChan  chan []byte
	errChan   chan error
}

func (iw *instWrapper) ReadResponse(ctx context.Context) []byte {
	s := time.Now()
	defer fmt.Println("Took %v to get msg", time.Since(s).String())
	for {
		select {
		case rd := <-iw.respChan:
			return rd
		case <-ctx.Done():
			return nil
		default:
			continue
		}
	}
}
func (iw *instWrapper) ReadError() error {
	return <-iw.errChan
}

func (iw *instWrapper) VerifyInitiatorMessage(msg, sig []byte) error {
	return crypto.VerifyRSA(iw.initiator, msg, sig)
}

type InstanceID [24]byte

func (s *Switch) CreateInstance(reqID [24]byte, init *wire.Init, initiatorPublicKey *rsa.PublicKey, secret kyber.Scalar, secretShare *kyber_dkg.DistKeyShare) (Instance, []byte, error) {

	verify, err := s.CreateVerifyFunc(append(init.Operators, init.NewOperators...))
	if err != nil {
		return nil, nil, err
	}

	operatorID := uint64(0)
	operatorPubKey := s.PrivateKey.Public().(*rsa.PublicKey)
	pkBytes, err := crypto.EncodePublicKey(operatorPubKey)
	if err != nil {
		return nil, nil, err
	}
	for _, op := range append(init.Operators, init.NewOperators...) {
		if bytes.Equal(op.PubKey, pkBytes) {
			operatorID = op.ID
			break
		}
	}

	if operatorID == 0 {
		return nil, nil, errors.New("my operator is missing inside the op list")
	}

	bchan := make(chan []byte, 5)

	broadcast := func(msg []byte) error {
		bchan <- msg
		return nil
	}

	opts := dkg.OwnerOpts{
		Logger:             s.Logger.With(zap.String("instance", hex.EncodeToString(reqID[:])), zap.Uint64("operator_id", operatorID)),
		BroadcastF:         broadcast,
		SignFunc:           s.Sign,
		VerifyFunc:         verify,
		Suite:              bls3.NewBLS12381Suite(),
		ID:                 operatorID,
		OpPrivKey:          s.PrivateKey,
		Owner:              init.Owner,
		Nonce:              init.Nonce,
		InitiatorPublicKey: initiatorPublicKey,
	}
	owner := dkg.New(opts)
	// wait for exchange msg
	if secretShare != nil {
		owner.SecretShare = secretShare
	}
	resp, err := owner.Init(reqID, init, secret)
	if err != nil {
		return nil, nil, err
	}
	if err := owner.Broadcast(resp); err != nil {
		return nil, nil, err
	}
	s.Logger.Info("Waiting for owner response to init")
	res := <-bchan
	return &instWrapper{LocalOwner: owner, initiator: initiatorPublicKey, respChan: bchan, errChan: owner.ErrorChan}, res, nil
}

func (s *Switch) Sign(msg []byte) ([]byte, error) {
	return crypto.SignRSA(s.PrivateKey, msg)
}

func (s *Switch) CreateVerifyFunc(ops []*wire.Operator) (func(id uint64, msg []byte, sig []byte) error, error) {

	inst_ops := make(map[uint64]*rsa.PublicKey)
	for _, op := range ops {
		pk, err := crypto.ParseRSAPubkey(op.PubKey)
		if err != nil {
			return nil, err
		}
		inst_ops[op.ID] = pk
	}
	return func(id uint64, msg []byte, sig []byte) error {
		pk, ok := inst_ops[id]
		if !ok {
			return errors.New("ops not exist for this instance")
		}
		return crypto.VerifyRSA(pk, msg, sig)
	}, nil
}

type Switch struct {
	Logger           *zap.Logger
	Mtx              sync.RWMutex
	InstanceInitTime map[InstanceID]time.Time
	Instances        map[InstanceID]Instance

	PrivateKey *rsa.PrivateKey

	//broadcastF func([]byte) error
}

func NewSwitch(pv *rsa.PrivateKey, logger *zap.Logger) *Switch {
	return &Switch{
		Logger:           logger,
		Mtx:              sync.RWMutex{},
		InstanceInitTime: make(map[InstanceID]time.Time, MaxInstances),
		Instances:        make(map[InstanceID]Instance, MaxInstances),
		PrivateKey:       pv,
	}
}

func (s *Switch) InitInstance(reqID [24]byte, initMsg *wire.Transport, initiatorSignature []byte) ([]byte, error) {
	logger := s.Logger.With(zap.String("reqid", hex.EncodeToString(reqID[:])))
	logger.Info("initializing DKG instance")
	init := &wire.Init{}
	if err := init.UnmarshalSSZ(initMsg.Data); err != nil {
		return nil, err
	}
	s.Logger.Debug("decoded init message")
	// Check that incoming init message signature is valid
	initiatorPubKey, err := crypto.ParseRSAPubkey(init.InitiatorPublicKey)
	if err != nil {
		return nil, err
	}
	marshalledWireMsg, err := initMsg.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	err = crypto.VerifyRSA(initiatorPubKey, marshalledWireMsg, initiatorSignature)
	if err != nil {
		return nil, fmt.Errorf("init message signature isn't valid: %s", err.Error())
	}
	s.Logger.Info(fmt.Sprintf("init message signature is successfully verified, from: %x", sha256.Sum256(initiatorPubKey.N.Bytes())))
	// Check if we run reshare
	var reshare bool
	if len(init.NewOperators) != 0 {
		reshare = true
	}
	// Get existing instance params
	if reshare {
		s.Logger.Info(fmt.Sprint("Starting reshare protocol"))
		_, ok := s.Instances[init.OldID]
		if ok {
			s.Mtx.Lock()
			l := len(s.Instances)
			if l >= MaxInstances {
				cleaned := s.CleanInstances() // not thread safe
				if l-cleaned >= MaxInstances {
					s.Mtx.Unlock()
					return nil, ErrMaxInstances
				}
			}
			_, ok := s.Instances[reqID]
			if ok {
				tm := s.InstanceInitTime[reqID]
				if !time.Now().After(tm.Add(MaxInstanceTime)) {
					s.Mtx.Unlock()
					return nil, ErrAlreadyExists
				}
				delete(s.Instances, reqID)
				delete(s.InstanceInitTime, reqID)
			}
			oldLocalOwner := s.Instances[init.OldID].GetLocalOwner()
			s.Mtx.Unlock()
			inst, resp, err := s.CreateInstance(reqID, init, initiatorPubKey, oldLocalOwner.Data.Secret, oldLocalOwner.SecretShare)
			if err != nil {
				return nil, err
			}
			s.Mtx.Lock()
			_, ok = s.Instances[reqID]
			if ok {
				s.Mtx.Unlock()
				return nil, ErrAlreadyExists
			}
			s.Instances[reqID] = inst
			s.InstanceInitTime[reqID] = time.Now()
			s.Mtx.Unlock()
			return resp, nil
		}
	}

	s.Mtx.Lock()
	l := len(s.Instances)
	if l >= MaxInstances {
		cleaned := s.CleanInstances() // not thread safe
		if l-cleaned >= MaxInstances {
			s.Mtx.Unlock()
			return nil, ErrMaxInstances
		}
	}
	_, ok := s.Instances[reqID]
	if ok {
		tm := s.InstanceInitTime[reqID]
		if !time.Now().After(tm.Add(MaxInstanceTime)) {
			s.Mtx.Unlock()
			return nil, ErrAlreadyExists
		}
		delete(s.Instances, reqID)
		delete(s.InstanceInitTime, reqID)
	}
	s.Mtx.Unlock()
	s.Logger.Info(fmt.Sprint("Starting initial DKG protocol"))
	inst, resp, err := s.CreateInstance(reqID, init, initiatorPubKey, nil, nil)

	if err != nil {
		return nil, err
	}
	s.Mtx.Lock()
	_, ok = s.Instances[reqID]
	if ok {
		s.Mtx.Unlock()
		return nil, ErrAlreadyExists
	}
	s.Instances[reqID] = inst
	s.InstanceInitTime[reqID] = time.Now()
	s.Mtx.Unlock()

	return resp, nil

}

func (s *Switch) CleanInstances() int {
	count := 0
	for id, instime := range s.InstanceInitTime {
		if time.Now().After(instime.Add(MaxInstanceTime)) {
			delete(s.Instances, id)
			delete(s.InstanceInitTime, id)
			count++
		}
	}
	return count
}

func (s *Switch) ProcessMessage(dkgMsg []byte) ([]byte, error) {
	// get instanceID
	st := &wire.MultipleSignedTransports{}
	err := st.UnmarshalSSZ(dkgMsg)
	if err != nil {
		return nil, err
	}

	id := InstanceID(st.Identifier)

	s.Mtx.RLock()
	inst, ok := s.Instances[id]
	s.Mtx.RUnlock()

	if !ok {
		return nil, ErrMissingInstance
	}
	var mltplMsgsBytes []byte
	for _, ts := range st.Messages {
		tsBytes, err := ts.MarshalSSZ()
		if err != nil {
			return nil, err
		}
		mltplMsgsBytes = append(mltplMsgsBytes, tsBytes...)
	}
	// Verify initiator signature
	err = inst.VerifyInitiatorMessage(mltplMsgsBytes, st.Signature)
	if err != nil {
		return nil, err
	}
	for _, ts := range st.Messages {
		err = inst.Process(ts.Signer, ts)
		if err != nil {
			return nil, err
		}
	}
	tm := time.Millisecond * 50
	if st.Messages[0].Message.Type == wire.KyberMessageType {
		tm = time.Second * 11
	}
	ctx, c := context.WithTimeout(context.Background(), tm)
	defer c()
	resp := inst.ReadResponse(ctx)
	if resp == nil {
		t := &wire.Transport{
			Type:       wire.EmptyMessageType,
			Identifier: id,
			Data:       nil,
		}

		mrshl, err := t.MarshalSSZ()
		if err != nil {
			return nil, err
		}

		// Sign message with RSA private key
		sign, err := s.Sign(mrshl)
		if err != nil {
			return nil, err
		}

		ts := &wire.SignedTransport{
			Message:   t,
			Signer:    inst.GetLocalOwner().ID,
			Signature: sign,
		}

		resp, err = ts.MarshalSSZ()
		if err != nil {
			return nil, err
		}
	}
	return resp, nil
}
