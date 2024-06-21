package integration_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	eth_crypto "github.com/ethereum/go-ethereum/crypto"
	spec "github.com/ssvlabs/dkg-spec"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/bloxapp/ssv-dkg/pkgs/initiator"
	"github.com/bloxapp/ssv-dkg/pkgs/validator"
	"github.com/bloxapp/ssv-dkg/pkgs/wire"
	"github.com/bloxapp/ssv/logging"
	spec_crypto "github.com/ssvlabs/dkg-spec/crypto"
	"github.com/ssvlabs/dkg-spec/eip1271"
	"github.com/ssvlabs/dkg-spec/testing/stubs"
)

func TestReshareValidEOASig(t *testing.T) {
	err := logging.SetGlobalLogger("info", "capital", "console", nil)
	require.NoError(t, err)
	logger := zap.L().Named("integration-tests")
	version := "test.version"
	withdraw := common.HexToAddress("0x81592c3de184a3e2c0dcb5a261bc107bfa91f494")
	// Open ethereum keystore
	jsonBytes, err := os.ReadFile("../examples/initiator/UTC--2024-06-14T14-05-12.366668334Z--dcc846fa10c7cfce9e6eb37e06ed93b666cfc5e9")
	require.NoError(t, err)
	keyStorePassword, err := os.ReadFile(filepath.Clean("../examples/initiator/password"))
	require.NoError(t, err)
	sk, err := keystore.DecryptKey(jsonBytes, string(keyStorePassword))
	require.NoError(t, err)
	owner := eth_crypto.PubkeyToAddress(sk.PrivateKey.PublicKey)
	signedProofs, err := wire.LoadProofs("./stubs/4/000001-0xb92b076fdd7dcfb209bec593abb1291ee9ddfe8ecab279dc851b06bcd3fb056872888f947e4b5f9d6df6703e547679e7/proofs.json")
	require.NoError(t, err)
	proofsData := wire.ConvertSignedProofsToSpec(signedProofs)
	stubClient := &stubs.Client{
		CallContractF: func(call ethereum.CallMsg) ([]byte, error) {
			return nil, nil
		},
	}
	servers, ops := createOperatorsFromExamplesFolder(t, version, stubClient)
	clnt, err := initiator.New(ops, logger, version, rootCert)
	require.NoError(t, err)
	t.Run("test reshare 4 new operators", func(t *testing.T) {
		ids := []uint64{11, 22, 33, 44}
		newIds := []uint64{55, 66, 77, 88}
		newId := spec.NewID()
		// construct reshare message and sign eip1271
		reshare, err := clnt.ConstructReshareMessage(
			ids,
			newIds,
			proofsData[0].Proof.ValidatorPubKey,
			"mainnet",
			withdraw.Bytes(),
			owner,
			0)
		require.NoError(t, err)
		hash, err := reshare.HashTreeRoot()
		require.NoError(t, err)
		ownerSig, err := eth_crypto.Sign(hash[:], sk.PrivateKey)
		require.NoError(t, err)
		err = spec_crypto.VerifySignedMessageByOwner(stubClient,
			owner,
			reshare,
			ownerSig,
		)
		require.NoError(t, err)
		reshareMsg := &wire.ReshareMessage{
			SignedReshare: &spec.SignedReshare{
				Reshare:   *reshare,
				Signature: ownerSig,
			},
			Proofs: proofsData,
		}
		depositData, ks, proofs, err := clnt.StartResharing(newId, reshareMsg)
		require.NoError(t, err)
		err = validator.ValidateResults([]*wire.DepositDataCLI{depositData}, ks, [][]*wire.SignedProof{proofs}, 1, owner, 0, withdraw)
		require.NoError(t, err)
	})
	for _, srv := range servers {
		srv.HttpSrv.Close()
	}
}

func TestReshareInvalidEOASig(t *testing.T) {
	err := logging.SetGlobalLogger("info", "capital", "console", nil)
	require.NoError(t, err)
	logger := zap.L().Named("integration-tests")
	version := "test.version"

	withdraw := common.HexToAddress("0x81592c3de184a3e2c0dcb5a261bc107bfa91f494")
	// Open ethereum keystore
	jsonBytes, err := os.ReadFile("../examples/initiator/UTC--2024-06-14T14-05-12.366668334Z--dcc846fa10c7cfce9e6eb37e06ed93b666cfc5e9")
	require.NoError(t, err)
	keyStorePassword, err := os.ReadFile(filepath.Clean("../examples/initiator/password"))
	require.NoError(t, err)
	sk, err := keystore.DecryptKey(jsonBytes, string(keyStorePassword))
	require.NoError(t, err)
	signedProofs, err := wire.LoadProofs("./stubs/4/000001-0xb92b076fdd7dcfb209bec593abb1291ee9ddfe8ecab279dc851b06bcd3fb056872888f947e4b5f9d6df6703e547679e7/proofs.json")
	require.NoError(t, err)
	proofsData := wire.ConvertSignedProofsToSpec(signedProofs)
	stubClient := &stubs.Client{
		CallContractF: func(call ethereum.CallMsg) ([]byte, error) {
			return nil, nil
		},
	}
	servers, ops := createOperatorsFromExamplesFolder(t, version, stubClient)
	clnt, err := initiator.New(ops, logger, version, rootCert)
	require.NoError(t, err)
	t.Run("test reshare 4 new operators", func(t *testing.T) {
		ids := []uint64{11, 22, 33, 44}
		newIds := []uint64{55, 66, 77, 88}
		newId := spec.NewID()
		// construct reshare message and sign eip1271
		reshare, err := clnt.ConstructReshareMessage(
			ids,
			newIds,
			proofsData[0].Proof.ValidatorPubKey,
			"mainnet",
			withdraw.Bytes(),
			[20]byte{},
			0)
		require.NoError(t, err)
		hash, err := reshare.HashTreeRoot()
		require.NoError(t, err)
		ownerSig, err := eth_crypto.Sign(hash[:], sk.PrivateKey)
		require.NoError(t, err)
		reshareMsg := &wire.ReshareMessage{
			SignedReshare: &spec.SignedReshare{
				Reshare:   *reshare,
				Signature: ownerSig,
			},
			Proofs: proofsData,
		}
		_, _, _, err = clnt.StartResharing(newId, reshareMsg)
		require.Error(t, err, "invalid signed reshare signature")
	})
	for _, srv := range servers {
		srv.HttpSrv.Close()
	}
}

func TestReshareValidContractSig(t *testing.T) {
	err := logging.SetGlobalLogger("info", "capital", "console", nil)
	require.NoError(t, err)
	logger := zap.L().Named("integration-tests")
	version := "test.version"
	withdraw := common.HexToAddress("0x81592c3de184a3e2c0dcb5a261bc107bfa91f494")
	// Open ethereum keystore
	jsonBytes, err := os.ReadFile("../examples/initiator/UTC--2024-06-14T14-05-12.366668334Z--dcc846fa10c7cfce9e6eb37e06ed93b666cfc5e9")
	require.NoError(t, err)
	keyStorePassword, err := os.ReadFile(filepath.Clean("../examples/initiator/password"))
	require.NoError(t, err)
	sk, err := keystore.DecryptKey(jsonBytes, string(keyStorePassword))
	require.NoError(t, err)
	owner := eth_crypto.PubkeyToAddress(sk.PrivateKey.PublicKey)
	signedProofs, err := wire.LoadProofs("./stubs/4/000001-0xb92b076fdd7dcfb209bec593abb1291ee9ddfe8ecab279dc851b06bcd3fb056872888f947e4b5f9d6df6703e547679e7/proofs.json")
	require.NoError(t, err)
	proofsData := wire.ConvertSignedProofsToSpec(signedProofs)
	stubClient := &stubs.Client{
		CallContractF: func(call ethereum.CallMsg) ([]byte, error) {
			ret := make([]byte, 32) // needs to be 32 byte for packing
			copy(ret[:4], eip1271.MagicValue[:])

			return ret, nil
		},
		CodeAtMap: map[common.Address]bool{
			owner: true,
		},
	}
	servers, ops := createOperatorsFromExamplesFolder(t, version, stubClient)
	clnt, err := initiator.New(ops, logger, version, rootCert)
	require.NoError(t, err)
	t.Run("test reshare 4 new operators", func(t *testing.T) {
		ids := []uint64{11, 22, 33, 44}
		newIds := []uint64{55, 66, 77, 88}
		newId := spec.NewID()
		// construct reshare message and sign eip1271
		reshare, err := clnt.ConstructReshareMessage(
			ids,
			newIds,
			proofsData[0].Proof.ValidatorPubKey,
			"mainnet",
			withdraw.Bytes(),
			owner,
			0)
		require.NoError(t, err)
		hash, err := reshare.HashTreeRoot()
		require.NoError(t, err)
		ownerSig, err := eth_crypto.Sign(hash[:], sk.PrivateKey)
		require.NoError(t, err)
		reshareMsg := &wire.ReshareMessage{
			SignedReshare: &spec.SignedReshare{
				Reshare:   *reshare,
				Signature: ownerSig,
			},
			Proofs: proofsData,
		}
		depositData, ks, proofs, err := clnt.StartResharing(newId, reshareMsg)
		require.NoError(t, err)
		err = validator.ValidateResults([]*wire.DepositDataCLI{depositData}, ks, [][]*wire.SignedProof{proofs}, 1, owner, 0, withdraw)
		require.NoError(t, err)
	})
	for _, srv := range servers {
		srv.HttpSrv.Close()
	}
}

func TestReshareInvalidContractSig(t *testing.T) {
	err := logging.SetGlobalLogger("info", "capital", "console", nil)
	require.NoError(t, err)
	logger := zap.L().Named("integration-tests")
	version := "test.version"
	withdraw := common.HexToAddress("0x81592c3de184a3e2c0dcb5a261bc107bfa91f494")
	// Open ethereum keystore
	jsonBytes, err := os.ReadFile("../examples/initiator/UTC--2024-06-14T14-05-12.366668334Z--dcc846fa10c7cfce9e6eb37e06ed93b666cfc5e9")
	require.NoError(t, err)
	keyStorePassword, err := os.ReadFile(filepath.Clean("../examples/initiator/password"))
	require.NoError(t, err)
	sk, err := keystore.DecryptKey(jsonBytes, string(keyStorePassword))
	require.NoError(t, err)
	owner := eth_crypto.PubkeyToAddress(sk.PrivateKey.PublicKey)
	signedProofs, err := wire.LoadProofs("./stubs/4/000001-0xb92b076fdd7dcfb209bec593abb1291ee9ddfe8ecab279dc851b06bcd3fb056872888f947e4b5f9d6df6703e547679e7/proofs.json")
	require.NoError(t, err)
	proofsData := wire.ConvertSignedProofsToSpec(signedProofs)
	stubClient := &stubs.Client{
		CallContractF: func(call ethereum.CallMsg) ([]byte, error) {
			ret := make([]byte, 32) // needs to be 32 byte for packing
			copy(ret[:4], eip1271.InvalidSigValue[:])

			return ret, nil
		},
		CodeAtMap: map[common.Address]bool{
			owner: true,
		},
	}
	servers, ops := createOperatorsFromExamplesFolder(t, version, stubClient)
	clnt, err := initiator.New(ops, logger, version, rootCert)
	require.NoError(t, err)
	t.Run("test reshare 4 new operators", func(t *testing.T) {
		ids := []uint64{11, 22, 33, 44}
		newIds := []uint64{55, 66, 77, 88}
		newId := spec.NewID()
		// construct reshare message and sign eip1271
		reshare, err := clnt.ConstructReshareMessage(
			ids,
			newIds,
			proofsData[0].Proof.ValidatorPubKey,
			"mainnet",
			withdraw.Bytes(),
			owner,
			0)
		require.NoError(t, err)
		hash, err := reshare.HashTreeRoot()
		require.NoError(t, err)
		ownerSig, err := eth_crypto.Sign(hash[:], sk.PrivateKey)
		require.NoError(t, err)
		reshareMsg := &wire.ReshareMessage{
			SignedReshare: &spec.SignedReshare{
				Reshare:   *reshare,
				Signature: ownerSig,
			},
			Proofs: proofsData,
		}
		_, _, _, err = clnt.StartResharing(newId, reshareMsg)
		require.Error(t, err, "signature invalid")
	})
	for _, srv := range servers {
		srv.HttpSrv.Close()
	}
}
