package operator_test

import (
	"crypto/rsa"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/bloxapp/ssv-dkg/pkgs/consts"
	"github.com/bloxapp/ssv-dkg/pkgs/crypto"
	"github.com/bloxapp/ssv-dkg/pkgs/initiator"
	"github.com/bloxapp/ssv-dkg/pkgs/operator"
	"github.com/bloxapp/ssv-dkg/pkgs/utils/test_utils"
	"github.com/bloxapp/ssv-dkg/pkgs/wire"
	"github.com/bloxapp/ssv/logging"
	"github.com/bloxapp/ssv/utils/rsaencryption"
)

const examplePath = "../../examples/"

func TestRateLimit(t *testing.T) {
	version := "v1.0.2"
	srv := test_utils.CreateTestOperatorFromFile(t, 1, examplePath, version)
	// Initiator priv key
	_, pv, err := rsaencryption.GenerateKeys()
	require.NoError(t, err)
	priv, err := rsaencryption.ConvertPemToPrivateKey(string(pv))
	require.NoError(t, err)
	pubKey := priv.Public().(*rsa.PublicKey)
	initPubBytes, err := crypto.EncodePublicKey(pubKey)
	require.NoError(t, err)
	t.Run("test /init rate limit", func(t *testing.T) {
		ops := make(map[uint64]initiator.Operator)
		ops[1] = initiator.Operator{Addr: srv.HttpSrv.URL, ID: 1, PubKey: &srv.PrivKey.PublicKey}

		parts := make([]*wire.Operator, 0)
		for _, id := range []uint64{1} {
			op, ok := ops[id]
			if !ok {
				t.Fatalf("no op")
			}
			pkBytes, err := crypto.EncodePublicKey(op.PubKey)
			require.NoError(t, err)
			parts = append(parts, &wire.Operator{
				ID:     op.ID,
				PubKey: pkBytes,
			})
		}

		init := &wire.Init{
			Operators:             parts,
			T:                     3,
			WithdrawalCredentials: common.HexToAddress("0x0000000000000000000000000000000000000009").Bytes(),
			Fork:                  [4]byte{0, 0, 0, 0},
			Owner:                 common.HexToAddress("0x0000000000000000000000000000000000000007"),
			Nonce:                 0,
			InitiatorPublicKey:    initPubBytes,
		}
		sszinit, err := init.MarshalSSZ()
		require.NoError(t, err)

		ts := &wire.Transport{
			Type:       wire.InitMessageType,
			Identifier: [24]byte{},
			Data:       sszinit,
			Version:    []byte(version),
		}

		tsssz, err := ts.MarshalSSZ()
		require.NoError(t, err)

		sig, err := crypto.SignRSA(priv, tsssz)
		require.NoError(t, err)

		signedTransportMsg := &wire.SignedTransport{
			Message:   ts,
			Signer:    0,
			Signature: sig,
		}

		msg, err := signedTransportMsg.MarshalSSZ()
		require.NoError(t, err)

		client := req.C()
		r := client.R()

		r.SetBodyBytes(msg)

		// Send requests
		errChan := make(chan []byte)
		time.Sleep(time.Second)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer close(errChan)
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				res, err := r.Post(fmt.Sprintf("%v/%v", srv.HttpSrv.URL, "init"))
				require.NoError(t, err)
				if res.Status == "429 Too Many Requests" {
					b, err := io.ReadAll(res.Body)
					require.NoError(t, err)
					errChan <- b
				}
			}
		}()
		for errResp := range errChan {
			require.NotEmpty(t, errResp)
			require.Equal(t, operator.ErrTooManyInitRequests, string(errResp))
		}
		wg.Wait()
	})
	t.Run("test /dkg rate limit", func(t *testing.T) {
		client := req.C()
		r := client.R()

		r.SetBodyBytes([]byte{})

		// Send requests
		errChan := make(chan []byte)
		time.Sleep(time.Second)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer close(errChan)
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				res, err := r.Post(fmt.Sprintf("%v/%v", srv.HttpSrv.URL, "dkg"))
				require.NoError(t, err)
				if res.Status == "429 Too Many Requests" {
					b, err := io.ReadAll(res.Body)
					require.NoError(t, err)
					errChan <- b
				}
			}
		}()
		for errResp := range errChan {
			require.NotEmpty(t, errResp)
			require.Equal(t, operator.ErrTooManyDKGRequests, string(errResp))
		}
		wg.Wait()
	})
	srv.HttpSrv.Close()
}

func TestWrongInitiatorSignature(t *testing.T) {
	err := logging.SetGlobalLogger("info", "capital", "console", nil)
	require.NoError(t, err)
	logger := zap.L().Named("operator-tests")
	ops := make(map[uint64]initiator.Operator)
	version := "v1.0.2"
	srv1 := test_utils.CreateTestOperatorFromFile(t, 1, examplePath, version)
	srv2 := test_utils.CreateTestOperatorFromFile(t, 2, examplePath, version)
	srv3 := test_utils.CreateTestOperatorFromFile(t, 3, examplePath, version)
	srv4 := test_utils.CreateTestOperatorFromFile(t, 4, examplePath, version)
	ops[1] = initiator.Operator{Addr: srv1.HttpSrv.URL, ID: 1, PubKey: &srv1.PrivKey.PublicKey}
	ops[2] = initiator.Operator{Addr: srv2.HttpSrv.URL, ID: 2, PubKey: &srv2.PrivKey.PublicKey}
	ops[3] = initiator.Operator{Addr: srv3.HttpSrv.URL, ID: 3, PubKey: &srv3.PrivKey.PublicKey}
	ops[4] = initiator.Operator{Addr: srv4.HttpSrv.URL, ID: 4, PubKey: &srv4.PrivKey.PublicKey}
	t.Run("test wrong pub key in init message", func(t *testing.T) {
		_, pv, err := rsaencryption.GenerateKeys()
		require.NoError(t, err)
		priv, err := rsaencryption.ConvertPemToPrivateKey(string(pv))
		require.NoError(t, err)
		withdraw := common.HexToAddress("0x0000000000000000000000000000000000000009")
		owner := common.HexToAddress("0x0000000000000000000000000000000000000007")
		ids := []uint64{1, 2, 3, 4}

		c := initiator.New(priv, ops, logger, version)
		// compute threshold (3f+1)
		threshold := len(ids) - ((len(ids) - 1) / 3)
		parts := make([]*wire.Operator, 0)
		for _, id := range ids {
			op, ok := c.Operators[id]
			require.True(t, ok)
			pkBytes, err := crypto.EncodePublicKey(op.PubKey)
			require.NoError(t, err)
			parts = append(parts, &wire.Operator{
				ID:     op.ID,
				PubKey: pkBytes,
			})
		}
		// Change pub key
		_, newPv, err := rsaencryption.GenerateKeys()
		require.NoError(t, err)
		newPriv, err := rsaencryption.ConvertPemToPrivateKey(string(newPv))
		require.NoError(t, err)
		wrongPub, err := crypto.EncodePublicKey(&newPriv.PublicKey)
		require.NoError(t, err)
		encPub, err := crypto.EncodePublicKey(&c.PrivateKey.PublicKey)
		require.NoError(t, err)
		c.Logger.Info("Initiator", zap.String("Pubkey:", fmt.Sprintf("%x", encPub)))
		// make init message
		init := &wire.Init{
			Operators:             parts,
			T:                     uint64(threshold),
			WithdrawalCredentials: withdraw.Bytes(),
			Fork:                  [4]byte{0, 0, 0, 0},
			Owner:                 owner,
			Nonce:                 0,
			InitiatorPublicKey:    wrongPub,
		}
		id := crypto.NewID()
		results, err := c.SendInitMsg(init, id, parts)
		require.NoError(t, err)
		var errs []error
		for i := 0; i < len(results); i++ {
			msg := results[i]
			tsp := &wire.SignedTransport{}
			if err := tsp.UnmarshalSSZ(msg); err != nil {
				// try parsing an error
				errmsg, parseErr := test_utils.ParseAsError(msg)
				require.NoError(t, parseErr)
				errs = append(errs, errmsg)
			}
		}
		require.Equal(t, 4, len(errs))
		for _, err := range errs {
			require.ErrorContains(t, err, "init: initiator signature isn't valid: crypto/rsa: verification error")
		}
	})
	t.Run("test wrong signature of init message", func(t *testing.T) {
		_, pv, err := rsaencryption.GenerateKeys()
		require.NoError(t, err)
		priv, err := rsaencryption.ConvertPemToPrivateKey(string(pv))
		require.NoError(t, err)
		withdraw := common.HexToAddress("0x0000000000000000000000000000000000000009")
		owner := common.HexToAddress("0x0000000000000000000000000000000000000007")
		ids := []uint64{1, 2, 3, 4}

		c := initiator.New(priv, ops, logger, version)
		// compute threshold (3f+1)
		threshold := len(ids) - ((len(ids) - 1) / 3)
		parts := make([]*wire.Operator, 0)
		for _, id := range ids {
			op, ok := c.Operators[id]
			require.True(t, ok)
			pkBytes, err := crypto.EncodePublicKey(op.PubKey)
			require.NoError(t, err)
			parts = append(parts, &wire.Operator{
				ID:     op.ID,
				PubKey: pkBytes,
			})
		}
		wrongPub, err := crypto.EncodePublicKey(&c.PrivateKey.PublicKey)
		require.NoError(t, err)
		encPub, err := crypto.EncodePublicKey(&c.PrivateKey.PublicKey)
		require.NoError(t, err)
		c.Logger.Info("Initiator", zap.String("Pubkey:", fmt.Sprintf("%x", encPub)))
		// make init message
		init := &wire.Init{
			Operators:             parts,
			T:                     uint64(threshold),
			WithdrawalCredentials: withdraw.Bytes(),
			Fork:                  [4]byte{0, 0, 0, 0},
			Owner:                 owner,
			Nonce:                 0,
			InitiatorPublicKey:    wrongPub,
		}
		id := crypto.NewID()
		sszinit, err := init.MarshalSSZ()
		require.NoError(t, err)
		initMessage := &wire.Transport{
			Type:       wire.InitMessageType,
			Identifier: id,
			Data:       sszinit,
			Version:    c.Version,
		}
		sig, err := hex.DecodeString("a32d0f695aad4a546b5507bb6b7cf43be7c54385589bbc6616bb97e58e839b596e8e827f8309488e6adc86562f7662738f46ae57f166e226913d66d6134149e8c6d6c60676da480c3ace2ea18f031ca4cfb51fa11a0595e63fe5808440b46c45d90e020f77bf35e64d7886ecf2e6f825168c955110753f73b37a5492191bd60a1bc7779f550b60aa37150ca2d16c15d33f014bca3dcfbb7a937312a51eb8d059a95203492e669238e5effdd38893b851d04f70cd58ad7ba0da7b21cb826b7397dbdffcbf6d66a8bcbf4e081a568c6e647e8d942c838533907ab7190c8a63eac73bec612cc1c44686164e734abec87ae223959b0f09f0c21cd99945e5319cb5a9")
		require.NoError(t, err)
		// Create signed init message
		signedInitMsg := &wire.SignedTransport{
			Message:   initMessage,
			Signer:    0,
			Signature: sig}
		signedInitMsgBts, err := signedInitMsg.MarshalSSZ()
		require.NoError(t, err)
		results, err := c.SendToAll(consts.API_INIT_URL, signedInitMsgBts, parts)
		require.NoError(t, err)
		var errs []error
		for i := 0; i < len(results); i++ {
			msg := results[i]
			tsp := &wire.SignedTransport{}
			if err := tsp.UnmarshalSSZ(msg); err != nil {
				// try parsing an error
				errmsg, parseErr := test_utils.ParseAsError(msg)
				require.NoError(t, parseErr)
				errs = append(errs, errmsg)
			}
		}
		require.Equal(t, 4, len(errs))
		for _, err := range errs {
			require.ErrorContains(t, err, "init: initiator signature isn't valid: crypto/rsa: verification error")
		}
	})
	srv1.HttpSrv.Close()
	srv2.HttpSrv.Close()
	srv3.HttpSrv.Close()
	srv4.HttpSrv.Close()
}
