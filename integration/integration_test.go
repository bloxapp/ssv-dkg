package integration

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/pkg/errors"

	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/herumi/bls-eth-go-binary/bls"

	"github.com/bloxapp/ssv/utils/rsaencryption"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	eth_crypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/bloxapp/ssv-dkg-tool/pkgs/client"
	"github.com/bloxapp/ssv-dkg-tool/pkgs/crypto"
	ourcrypto "github.com/bloxapp/ssv-dkg-tool/pkgs/crypto"
	"github.com/bloxapp/ssv-dkg-tool/pkgs/dkg"
	"github.com/bloxapp/ssv-dkg-tool/pkgs/server"
)

const encryptedKeyLength = 256

type testServer struct {
	id      uint64
	privKey *rsa.PrivateKey
	srv     *httptest.Server
}

func CreateServer(t *testing.T, id uint64) *testServer {
	_, pv, err := rsaencryption.GenerateKeys()
	require.NoError(t, err)
	priv, err := rsaencryption.ConvertPemToPrivateKey(string(pv))
	require.NoError(t, err)
	r := chi.NewRouter()
	swtch := server.NewSwitch(priv)
	lg := logrus.New()
	lg.SetLevel(logrus.DebugLevel)
	s := &server.Server{
		Logger: logrus.NewEntry(lg).WithField("comp", "server"),
		Router: r,
		State:  swtch,
	}
	server.RegisterRoutes(s)
	sTest := httptest.NewServer(s.Router)
	return &testServer{
		id:      id,
		privKey: priv,
		srv:     sTest,
	}
}

func TestHappyFlow(t *testing.T) {
	t.Run("test 4 operators happy flow", func(t *testing.T) {
		ops := make(map[uint64]client.Operator)
		srv1 := CreateServer(t, 1)
		ops[1] = client.Operator{srv1.srv.URL, 1, &srv1.privKey.PublicKey}
		srv2 := CreateServer(t, 2)
		ops[2] = client.Operator{srv2.srv.URL, 2, &srv2.privKey.PublicKey}
		srv3 := CreateServer(t, 3)
		ops[3] = client.Operator{srv3.srv.URL, 3, &srv3.privKey.PublicKey}
		srv4 := CreateServer(t, 7)
		ops[101] = client.Operator{srv4.srv.URL, 101, &srv4.privKey.PublicKey}
		clnt := client.New(ops)
		withdraw := newEthAddress(t)
		owner := newEthAddress(t)
		depositData, ks, err := clnt.StartDKG(withdraw.Bytes(), []uint64{1, 2, 3, 101}, [4]byte{0, 0, 0, 0}, "mainnnet", owner, 0)
		require.NoError(t, err)
		sharesDataSigned, err := hex.DecodeString(ks.Payload.Readable.Shares[2:])
		require.NoError(t, err)
		pubkeyraw, err := hex.DecodeString(ks.Payload.Readable.PublicKey[2:])
		require.NoError(t, err)
		testSharesData(t, ops, []*rsa.PrivateKey{srv1.privKey, srv2.privKey, srv3.privKey, srv4.privKey}, sharesDataSigned, pubkeyraw, owner, 0)
		testDepositData(t, depositData, withdraw.Bytes(), owner, 0)
		srv1.srv.Close()
		srv2.srv.Close()
		srv3.srv.Close()
		srv4.srv.Close()
	})
	t.Run("test 7 operators happy flow", func(t *testing.T) {
		ops := make(map[uint64]client.Operator)
		srv1 := CreateServer(t, 1)
		ops[1] = client.Operator{srv1.srv.URL, 1, &srv1.privKey.PublicKey}
		srv2 := CreateServer(t, 2)
		ops[2] = client.Operator{srv2.srv.URL, 2, &srv2.privKey.PublicKey}
		srv3 := CreateServer(t, 3)
		ops[3] = client.Operator{srv3.srv.URL, 3, &srv3.privKey.PublicKey}
		srv4 := CreateServer(t, 4)
		ops[4] = client.Operator{srv4.srv.URL, 4, &srv4.privKey.PublicKey}
		srv5 := CreateServer(t, 5)
		ops[5] = client.Operator{srv5.srv.URL, 5, &srv5.privKey.PublicKey}
		srv6 := CreateServer(t, 6)
		ops[6] = client.Operator{srv6.srv.URL, 6, &srv6.privKey.PublicKey}
		srv7 := CreateServer(t, 7)
		ops[7] = client.Operator{srv7.srv.URL, 7, &srv7.privKey.PublicKey}
		clnt := client.New(ops)
		withdraw := newEthAddress(t)
		owner := newEthAddress(t)
		depositData, ks, err := clnt.StartDKG(withdraw.Bytes(), []uint64{1, 2, 3, 4, 5, 6, 7}, [4]byte{0, 0, 0, 0}, "mainnnet", owner, 0)
		require.NoError(t, err)
		sharesDataSigned, err := hex.DecodeString(ks.Payload.Readable.Shares[2:])
		require.NoError(t, err)
		pubkeyraw, err := hex.DecodeString(ks.Payload.Readable.PublicKey[2:])
		require.NoError(t, err)
		testSharesData(t, ops, []*rsa.PrivateKey{srv1.privKey, srv2.privKey, srv3.privKey, srv4.privKey, srv5.privKey, srv6.privKey, srv7.privKey}, sharesDataSigned, pubkeyraw, owner, 0)
		testDepositData(t, depositData, withdraw.Bytes(), owner, 0)
		srv1.srv.Close()
		srv2.srv.Close()
		srv3.srv.Close()
		srv4.srv.Close()
		srv5.srv.Close()
		srv6.srv.Close()
		srv7.srv.Close()
	})
	t.Run("test 12 operators happy flow", func(t *testing.T) {
		ops := make(map[uint64]client.Operator)
		srv1 := CreateServer(t, 1)
		ops[1] = client.Operator{srv1.srv.URL, 1, &srv1.privKey.PublicKey}
		srv2 := CreateServer(t, 2)
		ops[2] = client.Operator{srv2.srv.URL, 2, &srv2.privKey.PublicKey}
		srv3 := CreateServer(t, 3)
		ops[3] = client.Operator{srv3.srv.URL, 3, &srv3.privKey.PublicKey}
		srv4 := CreateServer(t, 4)
		ops[4] = client.Operator{srv4.srv.URL, 4, &srv4.privKey.PublicKey}
		srv5 := CreateServer(t, 5)
		ops[5] = client.Operator{srv5.srv.URL, 5, &srv5.privKey.PublicKey}
		srv6 := CreateServer(t, 6)
		ops[6] = client.Operator{srv6.srv.URL, 6, &srv6.privKey.PublicKey}
		srv7 := CreateServer(t, 7)
		ops[7] = client.Operator{srv7.srv.URL, 7, &srv7.privKey.PublicKey}
		srv8 := CreateServer(t, 8)
		ops[8] = client.Operator{srv8.srv.URL, 8, &srv8.privKey.PublicKey}
		srv9 := CreateServer(t, 9)
		ops[9] = client.Operator{srv9.srv.URL, 9, &srv9.privKey.PublicKey}
		srv10 := CreateServer(t, 10)
		ops[10] = client.Operator{srv10.srv.URL, 10, &srv10.privKey.PublicKey}
		srv11 := CreateServer(t, 11)
		ops[11] = client.Operator{srv11.srv.URL, 11, &srv11.privKey.PublicKey}
		srv12 := CreateServer(t, 12)
		ops[12] = client.Operator{srv12.srv.URL, 12, &srv12.privKey.PublicKey}
		clnt := client.New(ops)
		withdraw := newEthAddress(t)
		owner := newEthAddress(t)
		depositData, ks, err := clnt.StartDKG(withdraw.Bytes(), []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, [4]byte{0, 0, 0, 0}, "mainnnet", owner, 0)
		require.NoError(t, err)
		sharesDataSigned, err := hex.DecodeString(ks.Payload.Readable.Shares[2:])
		require.NoError(t, err)
		pubkeyraw, err := hex.DecodeString(ks.Payload.Readable.PublicKey[2:])
		require.NoError(t, err)
		testSharesData(t, ops, []*rsa.PrivateKey{srv1.privKey, srv2.privKey, srv3.privKey, srv4.privKey, srv5.privKey, srv6.privKey, srv7.privKey, srv8.privKey, srv9.privKey, srv10.privKey, srv11.privKey, srv12.privKey}, sharesDataSigned, pubkeyraw, owner, 0)
		testDepositData(t, depositData, withdraw.Bytes(), owner, 0)
		srv1.srv.Close()
		srv2.srv.Close()
		srv3.srv.Close()
		srv4.srv.Close()
		srv5.srv.Close()
		srv6.srv.Close()
		srv7.srv.Close()
		srv8.srv.Close()
		srv9.srv.Close()
		srv10.srv.Close()
		srv11.srv.Close()
		srv12.srv.Close()
	})
}

func testSharesData(t *testing.T, ops map[uint64]client.Operator, keys []*rsa.PrivateKey, sharesData []byte, validatorPublicKey []byte, owner common.Address, nonce uint16) {
	operatorCount := len(keys)
	signatureOffset := phase0.SignatureLength
	pubKeysOffset := phase0.PublicKeyLength*operatorCount + signatureOffset
	sharesExpectedLength := encryptedKeyLength*operatorCount + pubKeysOffset
	require.Len(t, sharesData, sharesExpectedLength)
	signature := sharesData[:signatureOffset]
	msg := []byte("Hello")
	require.NoError(t, ourcrypto.VerifyOwnerNoceSignature(signature, owner, validatorPublicKey, nonce))
	_ = splitBytes(sharesData[signatureOffset:pubKeysOffset], phase0.PublicKeyLength)
	encryptedKeys := splitBytes(sharesData[pubKeysOffset:], len(sharesData[pubKeysOffset:])/operatorCount)
	sigs2 := make(map[uint64][]byte)
	for i, enck := range encryptedKeys {
		priv := keys[i]
		share, err := rsaencryption.DecodeKey(priv, enck)
		require.NoError(t, err)
		secret := &bls.SecretKey{}
		require.NoError(t, secret.SetHexString(string(share)))
		// Find operator ID by PubKey
		var operatorID uint64
		for id, op := range ops {
			if bytes.Equal(priv.PublicKey.N.Bytes(), op.PubKey.N.Bytes()) {
				operatorID = id
			}
		}
		sig := secret.SignByte(msg)
		sigs2[operatorID] = sig.Serialize()
	}
	recon, err := ReconstructSignatures(sigs2)
	require.NoError(t, err)
	require.NoError(t, VerifyReconstructedSignature(recon, validatorPublicKey, msg))
}

// ReconstructSignatures receives a map of user indexes and serialized bls.Sign.
// It then reconstructs the original threshold signature using lagrange interpolation
func ReconstructSignatures(signatures map[uint64][]byte) (*bls.Sign, error) {
	reconstructedSig := bls.Sign{}
	idVec := make([]bls.ID, 0)
	sigVec := make([]bls.Sign, 0)
	for index, signature := range signatures {
		blsID := bls.ID{}
		err := blsID.SetDecString(fmt.Sprintf("%d", index))
		if err != nil {
			return nil, err
		}
		idVec = append(idVec, blsID)
		blsSig := bls.Sign{}

		err = blsSig.Deserialize(signature)
		if err != nil {
			return nil, err
		}
		sigVec = append(sigVec, blsSig)
	}
	err := reconstructedSig.Recover(sigVec, idVec)
	return &reconstructedSig, err
}

func VerifyReconstructedSignature(sig *bls.Sign, validatorPubKey []byte, msg []byte) error {
	pk := &bls.PublicKey{}
	if err := pk.Deserialize(validatorPubKey); err != nil {
		return errors.Wrap(err, "could not deserialize validator pk")
	}
	// verify reconstructed sig
	if res := sig.VerifyByte(pk, msg); !res {
		return errors.New("could not reconstruct a valid signature")
	}
	return nil
}

func newEthAddress(t *testing.T) common.Address {
	privateKey, err := eth_crypto.GenerateKey()
	require.NoError(t, err)
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	require.True(t, ok)
	address := eth_crypto.PubkeyToAddress(*publicKeyECDSA)
	return address
}

func splitBytes(buf []byte, lim int) [][]byte {
	var chunk []byte
	chunks := make([][]byte, 0, len(buf)/lim+1)
	for len(buf) >= lim {
		chunk, buf = buf[:lim], buf[lim:]
		chunks = append(chunks, chunk)
	}
	if len(buf) > 0 {
		chunks = append(chunks, buf[:])
	}
	return chunks
}

func testDepositData(t *testing.T, depsitDataJson *client.DepositDataJson, withdrawCred []byte, owner common.Address, nonce uint16) {
	require.True(t, bytes.Equal(crypto.WithdrawalCredentialsHash(withdrawCred), hexutil.MustDecode("0x"+depsitDataJson.WithdrawalCredentials)))
	masterSig := &bls.Sign{}
	require.NoError(t, masterSig.DeserializeHexStr(depsitDataJson.Signature))
	valdatorPubKey := &bls.PublicKey{}
	require.NoError(t, valdatorPubKey.DeserializeHexStr(depsitDataJson.PubKey))

	// Check root
	var fork [4]byte
	copy(fork[:], hexutil.MustDecode("0x"+depsitDataJson.ForkVersion))
	depositDataRoot, err := ourcrypto.DepositDataRoot(withdrawCred, valdatorPubKey, dkg.GetNetworkByFork(fork), client.MaxEffectiveBalanceInGwei)
	require.NoError(t, err)
	res := masterSig.VerifyByte(valdatorPubKey, depositDataRoot[:])
	require.True(t, res)
	depositData, _, err := ourcrypto.DepositData(masterSig.Serialize(), withdrawCred, valdatorPubKey.Serialize(), dkg.GetNetworkByFork(fork), client.MaxEffectiveBalanceInGwei)
	require.NoError(t, err)
	res, err = crypto.VerifyDepositData(depositData, dkg.GetNetworkByFork(fork))
	require.NoError(t, err)
	require.True(t, res)
	depositMsg := &phase0.DepositMessage{
		WithdrawalCredentials: depositData.WithdrawalCredentials,
		Amount:                client.MaxEffectiveBalanceInGwei,
	}
	copy(depositMsg.PublicKey[:], depositData.PublicKey[:])
	depositMsgRoot, _ := depositMsg.HashTreeRoot()
	require.True(t, bytes.Equal(depositMsgRoot[:], hexutil.MustDecode("0x"+depsitDataJson.DepositMessageRoot)))
}
