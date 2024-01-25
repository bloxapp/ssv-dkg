package dkg

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"testing"

	"github.com/drand/kyber"
	kyber_bls "github.com/drand/kyber-bls12381"
	kyber_dkg "github.com/drand/kyber/share/dkg"
	"github.com/ethereum/go-ethereum/common"
	eth_crypto "github.com/ethereum/go-ethereum/crypto"
	herumi_bls "github.com/herumi/bls-eth-go-binary/bls"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/bloxapp/ssv-dkg/pkgs/crypto"
	"github.com/bloxapp/ssv-dkg/pkgs/utils"
	wire2 "github.com/bloxapp/ssv-dkg/pkgs/wire"
	"github.com/bloxapp/ssv/utils/rsaencryption"
)

type testVerify struct {
	ops map[uint64]*rsa.PublicKey
}

func newTestVerify() *testVerify {
	return &testVerify{
		ops: make(map[uint64]*rsa.PublicKey),
	}
}

func (tv *testVerify) Add(id uint64, pk *rsa.PublicKey) {
	tv.ops[id] = pk
}

func (tv *testVerify) Verify(id uint64, msg, sig []byte) error {
	op, ok := tv.ops[id]
	if !ok {
		return errors.New("test shouldn't do this")
	}
	return crypto.VerifyRSA(op, msg, sig)
}

type testState struct {
	T       *testing.T
	ops     map[uint64]*LocalOwner
	opsPriv map[uint64]*rsa.PrivateKey
	tv      *testVerify
	ipk     *rsa.PublicKey
}

func (ts *testState) Broadcast(id uint64, data []byte) error {
	return ts.ForAll(func(o *LocalOwner) error {
		st := &wire2.SignedTransport{}
		if err := st.UnmarshalSSZ(data); err != nil {
			return err
		}
		if err := o.Process(id, st); err != nil {
			return err
		}
		return nil
	})
}

func (ts *testState) ForAll(f func(o *LocalOwner) error) error {
	for _, op := range ts.ops {
		if err := f(op); err != nil {
			return err
		}
	}
	return nil
}
func (ts *testState) ForNew(f func(o *LocalOwner) error, newOps []*wire2.Operator) error {
	for _, op := range newOps {
		newOp := ts.ops[op.ID]
		if err := f(newOp); err != nil {
			return err
		}
	}
	return nil
}

func (ts *testState) ForOld(f func(o *LocalOwner) error, oldOps []*wire2.Operator) error {
	for _, op := range oldOps {
		newOp := ts.ops[op.ID]
		if err := f(newOp); err != nil {
			return err
		}
	}
	return nil
}

func NewTestOperator(ts *testState, id uint64) (*LocalOwner, *rsa.PrivateKey) {
	pv, pk, err := crypto.GenerateKeys()
	if err != nil {
		ts.T.Error(err)
	}
	ts.tv.Add(id, pk)
	sign := func(d []byte) ([]byte, error) {
		return crypto.SignRSA(pv, d)
	}
	encrypt := func(d []byte) ([]byte, error) {
		return rsa.EncryptPKCS1v15(rand.Reader, pk, d)
	}
	decrypt := func(d []byte) ([]byte, error) {
		return rsaencryption.DecodeKey(pv, d)
	}
	ver := ts.tv.Verify
	logger, _ := zap.NewDevelopment()
	logger = logger.With(zap.Uint64("id", id))
	return &LocalOwner{
		Logger:    logger,
		ID:        id,
		Suite:     kyber_bls.NewBLS12381Suite(),
		exchanges: make(map[uint64]*wire2.Exchange),
		deals:     make(map[uint64]*kyber_dkg.DealBundle),
		broadcastF: func(bytes []byte) error {
			return ts.Broadcast(id, bytes)
		},
		signFunc:           sign,
		verifyFunc:         ver,
		encryptFunc:        encrypt,
		decryptFunc:        decrypt,
		InitiatorPublicKey: ts.ipk,
		RSAPub:             &pv.PublicKey,
		done:               make(chan struct{}, 1),
		startedDKG:         make(chan struct{}, 1),
	}, pv
}

func AddExistingOperator(ts *testState, owner *LocalOwner) *LocalOwner {
	id := owner.ID
	ts.tv.Add(id, owner.RSAPub)
	sign := func(d []byte) ([]byte, error) {
		return owner.signFunc(d)
	}
	ver := ts.tv.Verify
	logger, _ := zap.NewDevelopment()
	logger = logger.With(zap.Uint64("id", id))
	return &LocalOwner{
		Logger:    logger,
		ID:        id,
		Suite:     kyber_bls.NewBLS12381Suite(),
		exchanges: make(map[uint64]*wire2.Exchange),
		deals:     make(map[uint64]*kyber_dkg.DealBundle),
		broadcastF: func(bytes []byte) error {
			return ts.Broadcast(id, bytes)
		},
		signFunc:           sign,
		verifyFunc:         ver,
		encryptFunc:        owner.encryptFunc,
		decryptFunc:        owner.decryptFunc,
		InitiatorPublicKey: ts.ipk,
		RSAPub:             owner.RSAPub,
		done:               make(chan struct{}, 1),
		startedDKG:         make(chan struct{}, 1),
	}

}

func TestDKGInit(t *testing.T) {
	// Send operators we want to deal with them
	_, initatorPk, err := crypto.GenerateKeys()
	require.NoError(t, err)
	ts := &testState{
		T:       t,
		ops:     make(map[uint64]*LocalOwner),
		opsPriv: make(map[uint64]*rsa.PrivateKey),
		tv:      newTestVerify(),
		ipk:     initatorPk,
	}
	var ids []uint64
	for i := 1; i < 5; i++ {
		op, priv := NewTestOperator(ts, uint64(i))
		ts.ops[op.ID] = op
		ts.opsPriv[op.ID] = priv
		ids = append(ids, op.ID)
	}
	opsarr := make([]*wire2.Operator, 0, len(ts.ops))
	for id := range ts.ops {
		pktobytes, err := crypto.EncodePublicKey(ts.tv.ops[id])
		require.NoError(t, err)
		opsarr = append(opsarr, &wire2.Operator{
			ID:     id,
			PubKey: pktobytes,
		})
	}
	encodedInitiatorPk, err := crypto.EncodePublicKey(initatorPk)
	require.NoError(t, err)
	init := &wire2.Init{
		Operators:             opsarr,
		T:                     3,
		WithdrawalCredentials: []byte("0x0000"),
		Fork:                  [4]byte{0, 0, 0, 0},
		Nonce:                 0,
		Owner:                 common.HexToAddress("0x1234"),
		InitiatorPublicKey:    encodedInitiatorPk,
	}
	uid := crypto.NewID()
	exch := map[uint64]*wire2.Transport{}

	err = ts.ForAll(func(o *LocalOwner) error {
		ts, err := o.Init(uid, init)
		if err != nil {
			t.Error(t, err)
		}
		exch[o.ID] = ts
		return nil
	})
	require.NoError(t, err)
	err = ts.ForAll(func(o *LocalOwner) error {
		return o.Broadcast(exch[o.ID])
	})
	require.NoError(t, err)
	err = ts.ForAll(func(o *LocalOwner) error {
		<-o.startedDKG
		return nil
	})

	require.NoError(t, err)
	err = ts.ForAll(func(o *LocalOwner) error {
		<-o.done
		return nil
	})
	require.NoError(t, err)
	var encShares []byte
	var ceremonySigs []byte
	ssvContractOwnerNonceSigShares := make([]*herumi_bls.Sign, 0)
	for _, o := range ts.ops {
		encShare, ceremonySig, _, sigOwnerNonce, err := constructShares(o)
		require.NoError(t, err)
		encShares = append(encShares, encShare...)
		ceremonySigs = append(ceremonySigs, ceremonySig...)
		ssvContractOwnerNonceSigShares = append(ssvContractOwnerNonceSigShares, sigOwnerNonce)
	}
	_, err = crypto.RecoverMasterSig(ids, ssvContractOwnerNonceSigShares)
	require.NoError(t, err)
	// Verify Ceremony sigs
	encryptedKeys := utils.SplitBytes(encShares, len(encShares)/len(ts.ops))
	for _, o := range ts.ops {
		ver, err := verifyCeremonySigs(encryptedKeys, o, ts.opsPriv[o.ID], encodedInitiatorPk, ceremonySigs)
		require.NoError(t, err)
		if !ver {
			t.Fatal("cant verify ceremony signatures")
		}
	}
}

func TestDKGReshare(t *testing.T) {
	// Send operators we want to deal with them
	_, initatorPk, err := crypto.GenerateKeys()
	require.NoError(t, err)
	ts := &testState{
		T:       t,
		ops:     make(map[uint64]*LocalOwner),
		opsPriv: make(map[uint64]*rsa.PrivateKey),
		tv:      newTestVerify(),
		ipk:     initatorPk,
	}
	var ids []uint64
	for i := 1; i < 5; i++ {
		op, priv := NewTestOperator(ts, uint64(i))
		ts.ops[op.ID] = op
		ts.opsPriv[op.ID] = priv
		ids = append(ids, op.ID)
	}
	opsarr := make([]*wire2.Operator, 0, len(ts.tv.ops))
	for id := range ts.ops {
		pktobytes, err := crypto.EncodePublicKey(ts.tv.ops[id])
		require.NoError(t, err)
		opsarr = append(opsarr, &wire2.Operator{
			ID:     id,
			PubKey: pktobytes,
		})
	}
	encodedInitiatorPk, err := crypto.EncodePublicKey(initatorPk)
	require.NoError(t, err)
	init := &wire2.Init{
		Operators:             opsarr,
		T:                     3,
		WithdrawalCredentials: []byte("0x0000"),
		Fork:                  [4]byte{0, 0, 0, 0},
		Nonce:                 0,
		Owner:                 common.HexToAddress("0x1234"),
		InitiatorPublicKey:    encodedInitiatorPk,
	}
	uid := crypto.NewID()
	exch := map[uint64]*wire2.Transport{}

	err = ts.ForAll(func(o *LocalOwner) error {
		ts, err := o.Init(uid, init)
		if err != nil {
			t.Error(t, err)
		}
		exch[o.ID] = ts
		return nil
	})
	require.NoError(t, err)
	err = ts.ForAll(func(o *LocalOwner) error {
		return o.Broadcast(exch[o.ID])
	})
	require.NoError(t, err)
	err = ts.ForAll(func(o *LocalOwner) error {
		<-o.startedDKG
		return nil
	})

	require.NoError(t, err)

	pubs := make(map[uint64]kyber.Point)
	err = ts.ForAll(func(o *LocalOwner) error {
		<-o.done
		pubs[o.ID] = o.SecretShare.Public()
		return nil
	})
	require.NoError(t, err)
	var encShares []byte
	var ceremonySigs []byte
	var pubkeys []byte
	ssvContractOwnerNonceSigShares := make([]*herumi_bls.Sign, 0)
	for _, o := range ts.ops {
		encShare, ceremonySig, pubkey, sigOwnerNonce, err := constructShares(o)
		require.NoError(t, err)
		encShares = append(encShares, encShare...)
		ceremonySigs = append(ceremonySigs, ceremonySig...)
		pubkeys = append(pubkeys, pubkey...)
		ssvContractOwnerNonceSigShares = append(ssvContractOwnerNonceSigShares, sigOwnerNonce)
	}
	encryptedKeys := utils.SplitBytes(encShares, len(encShares)/len(ts.ops))
	copyOps := ts.ops
	for _, o := range copyOps {
		ver, err := verifyCeremonySigs(encryptedKeys, o, ts.opsPriv[o.ID], encodedInitiatorPk, ceremonySigs)
		require.NoError(t, err)
		if !ver {
			t.Fatal("cant verify ceremony signatures")
		}
	}
	var sharesData []byte
	sharesData = append(sharesData, pubkeys...)
	sharesData = append(sharesData, encShares...)
	reconstructedOwnerNonceMasterSig, err := crypto.RecoverMasterSig(ids, ssvContractOwnerNonceSigShares)
	require.NoError(t, err)
	var sharesDataSigned []byte
	sharesDataSigned = append(sharesDataSigned, reconstructedOwnerNonceMasterSig.Serialize()...)
	sharesDataSigned = append(sharesDataSigned, sharesData...)
	require.NoError(t, err)

	// ###################
	// Start RESHARE
	// ###################
	t.Log("Starting resharing")
	ts2 := &testState{
		T:       t,
		ops:     make(map[uint64]*LocalOwner),
		opsPriv: make(map[uint64]*rsa.PrivateKey),
		tv:      newTestVerify(),
		ipk:     initatorPk,
	}

	for _, opx := range ts.ops {
		op := AddExistingOperator(ts2, opx)
		ts2.ops[op.ID] = op
		ts2.opsPriv[op.ID] = ts.opsPriv[op.ID]
	}

	var newops []uint64
	newopsArr := make([]*wire2.Operator, 0, len(ts2.tv.ops))
	for i := 5; i < 9; i++ {
		op, priv := NewTestOperator(ts2, uint64(i))
		ts2.ops[op.ID] = op
		ts2.opsPriv[op.ID] = priv
		newops = append(newops, op.ID)
	}

	for _, opid := range newops {
		pktobytes, err := crypto.EncodePublicKey(ts2.tv.ops[opid])
		require.NoError(t, err)
		newopsArr = append(newopsArr, &wire2.Operator{
			ID:     opid,
			PubKey: pktobytes,
		})
	}

	oldopstr := "Old operators \n"
	for _, i := range opsarr {
		oldopstr += fmt.Sprintln(i.ID)
	}
	t.Logf(oldopstr)

	newopstr := "new operators \n"
	for _, i := range newopsArr {
		newopstr += fmt.Sprintln(i.ID)
	}
	t.Logf(newopstr)

	reshare := &wire2.Reshare{
		OldOperators:       opsarr,
		NewOperators:       newopsArr,
		OldT:               3,
		NewT:               3,
		Nonce:              1,
		Owner:              common.HexToAddress("0x1234"),
		Keyshares:          sharesDataSigned,
		CeremonySigs:       ceremonySigs,
		InitiatorPublicKey: encodedInitiatorPk,
	}
	newuid := crypto.NewID()
	exch2 := map[uint64]*wire2.Transport{}
	for _, o := range ts2.ops {
		commits, err := crypto.GetPubCommitsFromSharesData(reshare)
		require.NoError(t, err)
		for _, op := range reshare.OldOperators {
			if op.ID == o.ID {
				secretShare, err := crypto.GetSecretShareFromSharesData(reshare, ts2.opsPriv[op.ID], op.ID)
				require.NoError(t, err)
				if secretShare == nil {
					t.Fatal(fmt.Errorf("cant decrypt incoming private share"))
				}
				o.SecretShare = &kyber_dkg.DistKeyShare{
					Commits: commits,
					Share:   secretShare,
				}
			}
		}
		tmsg, err := o.InitReshare(newuid, reshare, commits)
		require.NoError(t, err)
		exch2[o.ID] = tmsg
	}
	require.NoError(t, err)
	err = ts2.ForAll(func(o *LocalOwner) error {
		return o.Broadcast(exch2[o.ID])
	})
	require.NoError(t, err)
	err = ts2.ForNew(func(o *LocalOwner) error {
		<-o.done
		return nil
	}, newopsArr)
	require.NoError(t, err)
	copyNewOps := make(map[uint64]*LocalOwner, 4)
	for i := 1; i < 5; i++ {
		copyNewOps[uint64(i)] = ts.ops[uint64(i)]
	}
	var encSharesReshare []byte
	var ceremonySigsReshare []byte
	for _, o := range copyNewOps {
		encShare, ceremonySig, _, _, err := constructShares(o)
		require.NoError(t, err)
		encSharesReshare = append(encSharesReshare, encShare...)
		ceremonySigsReshare = append(ceremonySigsReshare, ceremonySig...)
	}
	encryptedKeysReshare := utils.SplitBytes(encSharesReshare, len(encSharesReshare)/4)
	for _, o := range copyNewOps {
		ver, err := verifyCeremonySigs(encryptedKeysReshare, o, ts2.opsPriv[o.ID], encodedInitiatorPk, ceremonySigsReshare)
		require.NoError(t, err)
		if !ver {
			t.Fatal("cant verify ceremony signatures")
		}
	}
}

func constructShares(o *LocalOwner) (encShare, ceremonySig, pubkey []byte, sigOwnerNonce *herumi_bls.Sign, err error) {
	key, err := crypto.KyberShareToBLSKey(o.SecretShare.PriShare())
	if err != nil {
		return nil, nil, nil, nil, err
	}
	// Encrypt BLS share for SSV contract
	ciphertext, err := o.encryptFunc([]byte(key.SerializeToHexStr()))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	ceremonySig, err = o.GetCeremonySig(key)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	pubkey = key.GetPublicKey().Serialize()
	data := []byte(fmt.Sprintf("%s:%d", o.owner.String(), o.nonce))
	hash := eth_crypto.Keccak256([]byte(data))
	sigOwnerNonce = key.SignByte(hash)
	return ciphertext, ceremonySig, pubkey, sigOwnerNonce, nil
}

func verifyCeremonySigs(encryptedKeys [][]byte, o *LocalOwner, sk *rsa.PrivateKey, encodedInitiatorPk, ceremonySigs []byte) (bool, error) {
	var initVerify bool
	for _, enck := range encryptedKeys {
		prShare, err := rsaencryption.DecodeKey(sk, enck)
		if err != nil {
			continue
		}
		secret := &herumi_bls.SecretKey{}
		if err := secret.SetHexString(string(prShare)); err != nil {
			return false, err
		}
		// Check operator signature

		initiatorPubKey, err := crypto.ParseRSAPubkey(encodedInitiatorPk)
		if err != nil {
			return false, err
		}
		encInitPub, err := crypto.EncodePublicKey(initiatorPubKey)
		if err != nil {
			return false, err
		}
		sigs := utils.SplitBytes(ceremonySigs, crypto.SignatureLength)
		serialized := secret.Serialize()
		for _, sig := range sigs {
			dataToVerify := make([]byte, len(serialized)+len(encInitPub))
			copy(dataToVerify[:len(serialized)], serialized)
			copy(dataToVerify[len(serialized):], encInitPub)
			err := crypto.VerifyRSA(o.RSAPub, dataToVerify, sig)
			if err != nil {
				continue
			}
			initVerify = true
			break
		}
	}
	return initVerify, nil
}
