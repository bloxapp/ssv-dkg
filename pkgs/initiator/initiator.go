package initiator

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/ethereum/go-ethereum/common"
	eth_crypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/hashicorp/go-version"
	"github.com/herumi/bls-eth-go-binary/bls"
	"github.com/imroc/req/v3"
	"go.uber.org/zap"

	eth2_key_manager_core "github.com/bloxapp/eth2-key-manager/core"
	"github.com/bloxapp/ssv-dkg/pkgs/consts"
	"github.com/bloxapp/ssv-dkg/pkgs/crypto"
	"github.com/bloxapp/ssv-dkg/pkgs/dkg"
	"github.com/bloxapp/ssv-dkg/pkgs/utils"
	"github.com/bloxapp/ssv-dkg/pkgs/wire"
	ssvspec_types "github.com/bloxapp/ssv-spec/types"
)

// Operator structure represents operators info which is public
type Operator struct {
	Addr   string         // ip:port
	ID     uint64         // operators ID
	PubKey *rsa.PublicKey // operators RSA public key
}

// OperatorDataJson is used to store operators info ar JSON
type OperatorDataJson struct {
	Addr   string `json:"ip"`
	ID     uint64 `json:"id"`
	PubKey string `json:"public_key"`
}

// Operators mapping storage for operator structs [ID]operator
type Operators map[uint64]Operator

func (o Operators) Clone() Operators {
	clone := make(Operators)
	for k, v := range o {
		clone[k] = v
	}
	return clone
}

// Initiator main structure for initiator
type Initiator struct {
	Logger           *zap.Logger                            // logger
	Client           *req.Client                            // http client
	Operators        Operators                              // operators info mapping
	VerifyFunc       func(id uint64, msg, sig []byte) error // function to verify signatures of incoming messages
	PrivateKey       *rsa.PrivateKey                        // a unique initiator's RSA private key used for signing messages and identity
	Version          []byte
	KeysharesVersion []byte
}

// DepositDataJson structure to create a resulting deposit data JSON file according to ETH2 protocol
type DepositDataJson struct {
	PubKey                string      `json:"pubkey"`
	WithdrawalCredentials string      `json:"withdrawal_credentials"`
	Amount                phase0.Gwei `json:"amount"`
	Signature             string      `json:"signature"`
	DepositMessageRoot    string      `json:"deposit_message_root"`
	DepositDataRoot       string      `json:"deposit_data_root"`
	ForkVersion           string      `json:"fork_version"`
	NetworkName           string      `json:"network_name"`
	DepositCliVersion     string      `json:"deposit_cli_version"`
}

// DepositCliVersion is last version accepted by launchpad
const DepositCliVersion = "2.7.0"

// KeyShares structure to create an json file for ssv smart contract
type KeyShares struct {
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	Shares    []Data    `json:"shares"`
}

// Data structure as a part of KeyShares representing BLS validator public key and information about validators
type Data struct {
	ShareData `json:"data"`
	Payload   Payload `json:"payload"`
}

type ShareData struct {
	OwnerNonce   uint64         `json:"ownerNonce"`
	OwnerAddress string         `json:"ownerAddress "`
	PublicKey    string         `json:"publicKey"`
	Operators    []OperatorData `json:"operators"`
}

// OperatorData structure to represent information about operators participating in signing validator's duty
type OperatorData struct {
	ID          uint64 `json:"id"`
	OperatorKey string `json:"operatorKey"` // encoded RSA public key
}

type Payload struct {
	PublicKey   string   `json:"publicKey"`   // validator's public key
	OperatorIDs []uint64 `json:"operatorIds"` // operators IDs
	SharesData  string   `json:"sharesData"`  // encrypted private BLS shares of each operator participating in DKG
}

type pongResult struct {
	ip     string
	err    error
	result []byte
}

type CeremonySigs struct {
	ValidatorPubKey    string   `json:"validator"`
	OperatorIDs        []uint64 `json:"operatorIds"`
	Sigs               string   `json:"ceremonySigs"`
	InitiatorPublicKey string   `json:"initiatorPublicKey"`
}

// GeneratePayload generates at initiator ssv smart contract payload using DKG result  received from operators participating in DKG ceremony
func (c *Initiator) generateSSVKeysharesPayload(dkgResults []dkg.Result, owner common.Address, nonce uint64, ops []*wire.Operator) (*KeyShares, error) {
	ids := make([]uint64, 0)
	for i := 0; i < len(dkgResults); i++ {
		ids = append(ids, dkgResults[i].OperatorID)
	}
	ssvContractOwnerNoncePartialSigs, err := c.prepareOwnerNonceSigs(dkgResults, owner, nonce)
	if err != nil {
		return nil, err
	}
	// Recover and verify Master Signature for SSV contract owner+nonce
	reconstructedOwnerNonceMasterSig, err := crypto.RecoverMasterSig(ids, ssvContractOwnerNoncePartialSigs)
	if err != nil {
		return nil, err
	}
	c.Logger.Info("✅ successfully reconstructed master signature from partial signatures (threshold holds)")
	sigOwnerNonce := reconstructedOwnerNonceMasterSig.Serialize()
	err = crypto.VerifyOwnerNonceSignature(sigOwnerNonce, owner, dkgResults[0].ValidatorPubKey, uint16(nonce))
	if err != nil {
		return nil, err
	}
	c.Logger.Info("✅ verified owner and nonce master signature")
	operatorData := make([]OperatorData, 0)
	operatorIds := make([]uint64, 0)
	var pubkeys []byte
	var encryptedShares []byte
	for i := 0; i < len(dkgResults); i++ {
		// Data for forming share string
		pubkeys = append(pubkeys, dkgResults[i].SharePubKey...)
		encryptedShares = append(encryptedShares, dkgResults[i].EncryptedShare...)

		encPubKey, err := crypto.EncodePublicKey(dkgResults[i].PubKeyRSA)
		if err != nil {
			return nil, err
		}
		// compare RSA public key at result that they match operators we requested to participate at the ceremony
		if !bytes.Equal(encPubKey, ops[i].PubKey) {
			return nil, fmt.Errorf("incoming ceremony result has  ")
		}
		operatorData = append(operatorData, OperatorData{
			ID:          dkgResults[i].OperatorID,
			OperatorKey: string(encPubKey),
		})
		operatorIds = append(operatorIds, dkgResults[i].OperatorID)
	}

	// Create share string for ssv contract
	pubkeys = append(pubkeys, encryptedShares...)
	sigOwnerNonce = append(sigOwnerNonce, pubkeys...)

	operatorCount := len(dkgResults)
	signatureOffset := phase0.SignatureLength
	pubKeysOffset := phase0.PublicKeyLength*operatorCount + signatureOffset
	sharesExpectedLength := crypto.EncryptedKeyLength*operatorCount + pubKeysOffset

	if sharesExpectedLength != len(sigOwnerNonce) {
		return nil, fmt.Errorf("malformed ssv share data")
	}

	data := []Data{{ShareData{
		OwnerNonce:   nonce,
		OwnerAddress: owner.Hex(),
		PublicKey:    "0x" + hex.EncodeToString(dkgResults[0].ValidatorPubKey),
		Operators:    operatorData,
	}, Payload{
		PublicKey:   "0x" + hex.EncodeToString(dkgResults[0].ValidatorPubKey),
		OperatorIDs: operatorIds,
		SharesData:  "0x" + hex.EncodeToString(sigOwnerNonce),
	}}}

	ks := &KeyShares{}
	ks.Version = string(c.KeysharesVersion)
	ks.Shares = data
	ks.CreatedAt = time.Now().UTC()
	return ks, nil
}

func GenerateAggregatedKeyshares(keySharesArr []*KeyShares) (*KeyShares, error) {
	// order the keyshares by nonce
	sort.SliceStable(keySharesArr, func(i, j int) bool {
		return keySharesArr[i].Shares[0].OwnerNonce < keySharesArr[j].Shares[0].OwnerNonce
	})
	sorted := sort.SliceIsSorted(keySharesArr, func(p, q int) bool {
		return keySharesArr[p].Shares[0].OwnerNonce < keySharesArr[q].Shares[0].OwnerNonce
	})
	if !sorted {
		return nil, fmt.Errorf("slice is not sorted")
	}
	var data []Data
	for _, keyShares := range keySharesArr {
		data = append(data, keyShares.Shares...)
	}
	ks := &KeyShares{}
	ks.Version = keySharesArr[0].Version
	ks.Shares = data
	ks.CreatedAt = time.Now().UTC()
	return ks, nil
}

// New creates a main initiator structure
func New(privKey *rsa.PrivateKey, operatorMap Operators, logger *zap.Logger, ver, ksVer string) *Initiator {
	client := req.C()
	// Set timeout for operator responses
	client.SetTimeout(30 * time.Second)
	c := &Initiator{
		Logger:           logger,
		Client:           client,
		Operators:        operatorMap,
		PrivateKey:       privKey,
		VerifyFunc:       CreateVerifyFunc(operatorMap),
		Version:          []byte(ver),
		KeysharesVersion: []byte(ksVer),
	}
	return c
}

// opReqResult structure to represent http communication messages incoming to initiator from operators
type opReqResult struct {
	operatorID uint64
	err        error
	result     []byte
}

// SendAndCollect ssends http message to operator and read the response
func (c *Initiator) SendAndCollect(op Operator, method string, data []byte) ([]byte, error) {
	r := c.Client.R()
	r.SetBodyBytes(data)
	res, err := r.Post(fmt.Sprintf("%v/%v", op.Addr, method))
	if err != nil {
		return nil, err
	}
	resdata, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	c.Logger.Debug("operator responded", zap.Uint64("operator", op.ID), zap.String("method", method))
	return resdata, nil
}

// GetAndCollect request Get at operator route
func (c *Initiator) GetAndCollect(op Operator, method string) ([]byte, error) {
	r := c.Client.R()
	res, err := r.Get(fmt.Sprintf("%v/%v", op.Addr, method))
	if err != nil {
		return nil, err
	}
	resdata, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	c.Logger.Debug("operator responded", zap.String("IP", op.Addr), zap.String("method", method))
	return resdata, nil
}

// SendToAll sends http messages to all operators. Makes sure that all responses are received
func (c *Initiator) SendToAll(method string, msg []byte, operatorsIDs []*wire.Operator) ([][]byte, error) {
	resc := make(chan opReqResult, len(operatorsIDs))
	for _, op := range operatorsIDs {
		go func(operator Operator) {
			res, err := c.SendAndCollect(operator, method, msg)
			resc <- opReqResult{
				operatorID: operator.ID,
				err:        err,
				result:     res,
			}
		}(c.Operators[op.ID])
	}
	final := make([][]byte, 0, len(operatorsIDs))

	errarr := make([]error, 0)

	responses := make([]opReqResult, 0)
	for i := 0; i < len(operatorsIDs); i++ {
		res := <-resc
		if res.err != nil {
			errarr = append(errarr, fmt.Errorf("operator ID: %d, %w", res.operatorID, res.err))
			continue
		}
		responses = append(responses, res)
		// final = append(final, res.result)
	}
	// sort responses
	sort.SliceStable(responses, func(i, j int) bool {
		return responses[i].operatorID < responses[j].operatorID
	})
	// iterate and create final
	for _, res := range responses {
		final = append(final, res.result)
	}

	finalerr := error(nil)

	if len(errarr) > 0 {
		finalerr = errors.Join(errarr...)
	}

	return final, finalerr
}

// parseAsError parses the error from an operator
func ParseAsError(msg []byte) (parsedErr, err error) {
	sszerr := &wire.ErrSSZ{}
	err = sszerr.UnmarshalSSZ(msg)
	if err != nil {
		return nil, err
	}
	return errors.New(string(sszerr.Error)), nil
}

// VerifyAll verifies incoming to initiator messages from operators.
// Incoming message from operator should have valid signature
func (c *Initiator) VerifyAll(id [24]byte, allmsgs [][]byte) error {
	var errs error
	for i := 0; i < len(allmsgs); i++ {
		msg := allmsgs[i]
		tsp := &wire.SignedTransport{}
		if err := tsp.UnmarshalSSZ(msg); err != nil {
			errmsg, parseErr := ParseAsError(msg)
			if parseErr == nil {
				errs = errors.Join(errs, fmt.Errorf("%v", errmsg))
				continue
			}
			return err
		}
		signedBytes, err := tsp.Message.MarshalSSZ()
		if err != nil {
			return err
		}
		// Verify that incoming messages have valid DKG ceremony ID
		if !bytes.Equal(id[:], tsp.Message.Identifier[:]) {
			return fmt.Errorf("incoming message has wrong ID, aborting... operator %d, msg ID %x", tsp.Signer, tsp.Message.Identifier[:])
		}
		// Verification operator signatures
		if err := c.VerifyFunc(tsp.Signer, signedBytes, tsp.Signature); err != nil {
			return err
		}
	}
	return errs
}

// MakeMultiple creates a one combined message from operators with initiator signature
func (c *Initiator) MakeMultiple(id [24]byte, allmsgs [][]byte) (*wire.MultipleSignedTransports, error) {
	// We are collecting responses at SendToAll which gives us int(msg)==int(oprators)
	final := &wire.MultipleSignedTransports{
		Identifier: id,
		Messages:   make([]*wire.SignedTransport, len(allmsgs)),
	}
	var allMsgsBytes []byte
	for i := 0; i < len(allmsgs); i++ {
		msg := allmsgs[i]
		tsp := &wire.SignedTransport{}
		if err := tsp.UnmarshalSSZ(msg); err != nil {
			errmsg, parseErr := ParseAsError(msg)
			if parseErr == nil {
				return nil, fmt.Errorf("msg %d returned: %v", i, errmsg)
			}
			return nil, err
		}
		// Verify that incoming messages have valid DKG ceremony ID
		if !bytes.Equal(id[:], tsp.Message.Identifier[:]) {
			return nil, fmt.Errorf("incoming message has wrong ID, aborting... operator %d, msg ID %x", tsp.Signer, tsp.Message.Identifier[:])
		}
		final.Messages[i] = tsp
		allMsgsBytes = append(allMsgsBytes, msg...)
	}
	// sign message by initiator
	c.Logger.Debug("Signing combined messages from operators", zap.String("initiator_id", hex.EncodeToString(c.PrivateKey.N.Bytes())))
	sig, err := crypto.SignRSA(c.PrivateKey, allMsgsBytes)
	if err != nil {
		return nil, err
	}
	final.Signature = sig
	return final, nil
}

// ValidatedOperatorData validates operators information data before starting a DKG ceremony
func ValidatedOperatorData(ids []uint64, operators Operators) ([]*wire.Operator, error) {
	if len(ids) < 4 {
		return nil, fmt.Errorf("wrong operators len: < 4")
	}
	if len(ids) > 13 {
		return nil, fmt.Errorf("wrong operators len: > 13")
	}
	if len(ids)%3 != 1 {
		return nil, fmt.Errorf("amount of operators should be 4,7,10,13")
	}

	ops := make([]*wire.Operator, 0)
	opMap := make(map[uint64]struct{})
	for _, id := range ids {
		op, ok := operators[id]
		if !ok {
			return nil, errors.New("operator is not in given operator data list")
		}

		_, exist := opMap[id]
		if exist {
			return nil, errors.New("operators ids should be unique in the list")
		}
		opMap[id] = struct{}{}

		pkBytes, err := crypto.EncodePublicKey(op.PubKey)
		if err != nil {
			return nil, fmt.Errorf("can't encode public key err: %v", err)
		}
		ops = append(ops, &wire.Operator{
			ID:     op.ID,
			PubKey: pkBytes,
		})
	}
	return ops, nil
}

// messageFlowHandling main steps of DKG at initiator
func (c *Initiator) messageFlowHandling(init *wire.Init, id [24]byte, operators []*wire.Operator) ([][]byte, error) {
	c.Logger.Info("phase 1: sending init message to operators")
	results, err := c.SendInitMsg(init, id, operators)
	if err != nil {
		return nil, err
	}
	err = c.VerifyAll(id, results)
	if err != nil {
		return nil, err
	}
	c.Logger.Info("phase 1: ✅ verified operator init responses signatures")

	c.Logger.Info("phase 2: ➡️ sending operator data (exchange messages) required for dkg")
	results, err = c.SendExchangeMsgs(results, id, operators)
	if err != nil {
		return nil, err
	}
	err = c.VerifyAll(id, results)
	if err != nil {
		return nil, err
	}
	c.Logger.Info("phase 2: ✅ verified operator responses (deal messages) signatures")
	c.Logger.Info("phase 3: ➡️ sending deal dkg data to all operators")
	dkgResult, err := c.SendKyberMsgs(results, id, operators)
	if err != nil {
		return nil, err
	}
	err = c.VerifyAll(id, results)
	if err != nil {
		return nil, err
	}
	c.Logger.Info("phase 2: ✅ verified operator dkg results signatures")
	return dkgResult, nil
}

func (c *Initiator) messageFlowHandlingReshare(reshare *wire.Reshare, newID [24]byte, oldOperators, newOperators []*wire.Operator) ([][]byte, error) {
	c.Logger.Info("phase 1: sending reshare message to old operators")
	allOps := utils.JoinSets(oldOperators, newOperators)
	results, err := c.SendReshareMsg(reshare, newID, allOps)
	if err != nil {
		return nil, err
	}
	err = c.VerifyAll(newID, results)
	if err != nil {
		return nil, err
	}
	c.Logger.Info("phase 1: ✅ verified operator resharing responses signatures")
	c.Logger.Info("phase 2: ➡️ sending operator data (exchange messages) required for dkg")
	results, err = c.SendExchangeMsgs(results, newID, allOps)
	if err != nil {
		return nil, err
	}
	err = c.VerifyAll(newID, results)
	if err != nil {
		return nil, err
	}
	c.Logger.Info("phase 2: ✅ verified old operator responses (deal messages) signatures")
	c.Logger.Info("phase 3: ➡️ sending deal dkg data to new operators")

	dkgResult, err := c.SendKyberMsgs(results, newID, newOperators)
	if err != nil {
		return nil, err
	}
	err = c.VerifyAll(newID, results)
	if err != nil {
		return nil, err
	}
	c.Logger.Info("phase 2: ✅ verified operator dkg results signatures")
	return dkgResult, nil
}

// reconstructAndVerifyDepositData verifies incoming from operators DKG result data and creates a resulting DepositDataJson structure to store as JSON file
func (c *Initiator) reconstructAndVerifyDepositData(dkgResults []dkg.Result, init *wire.Init) (*DepositDataJson, error) {
	ids := make([]uint64, 0)
	for i := 0; i < len(dkgResults); i++ {
		ids = append(ids, dkgResults[i].OperatorID)
	}
	sharePks, sigDepositShares, err := c.prepareDepositSigsAndPubs(dkgResults)
	if err != nil {
		return nil, err
	}
	var validatorPubKey bls.PublicKey
	if err := validatorPubKey.Deserialize(dkgResults[0].ValidatorPubKey); err != nil {
		return nil, err
	}
	network := utils.GetNetworkByFork(init.Fork)
	shareRoot, err := crypto.DepositDataRoot(init.WithdrawalCredentials, &validatorPubKey, network, dkg.MaxEffectiveBalanceInGwei)
	if err != nil {
		return nil, fmt.Errorf("failed to compute deposit data root: %v", err)
	}
	// Verify partial signatures and recovered threshold signature
	err = crypto.VerifyPartialSigs(sigDepositShares, sharePks, shareRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to verify partial signatures: %v", err)
	}

	// Recover and verify Master Signature
	// 1. Recover validator pub key
	validatorRecoveredPK, err := crypto.RecoverValidatorPublicKey(ids, sharePks)
	if err != nil {
		return nil, fmt.Errorf("failed to recover validator public key from shares: %v", err)
	}

	if !bytes.Equal(validatorPubKey.Serialize(), validatorRecoveredPK.Serialize()) {
		return nil, fmt.Errorf("incoming validator pub key is not equal recovered from shares: want %x, got %x", validatorRecoveredPK.Serialize(), validatorPubKey.Serialize())
	}
	// 2. Recover master signature from shares
	reconstructedDepositMasterSig, err := crypto.RecoverMasterSig(ids, sigDepositShares)
	if err != nil {
		return nil, fmt.Errorf("failed to recover master signature from shares: %v", err)
	}
	if !reconstructedDepositMasterSig.VerifyByte(&validatorPubKey, shareRoot) {
		return nil, fmt.Errorf("deposit root signature recovered from shares is invalid")
	}
	depositData, root, err := crypto.DepositData(reconstructedDepositMasterSig.Serialize(), init.WithdrawalCredentials, validatorPubKey.Serialize(), network, dkg.MaxEffectiveBalanceInGwei)
	if err != nil {
		return nil, fmt.Errorf("failed to compute deposit data: %v", err)
	}
	depositMsg := &phase0.DepositMessage{
		WithdrawalCredentials: depositData.WithdrawalCredentials,
		Amount:                dkg.MaxEffectiveBalanceInGwei,
	}
	copy(depositMsg.PublicKey[:], depositData.PublicKey[:])
	depositMsgRoot, err := depositMsg.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to compute deposit message root: %v", err)
	}
	// Final checks of prepared deposit data
	if !bytes.Equal(depositData.PublicKey[:], validatorRecoveredPK.Serialize()) {
		return nil, fmt.Errorf("deposit data is invalid. Wrong validator public key %x", depositData.PublicKey[:])
	}
	if !bytes.Equal(depositData.WithdrawalCredentials, crypto.ETH1WithdrawalCredentialsHash(init.WithdrawalCredentials)) {
		return nil, fmt.Errorf("deposit data is invalid. Wrong withdrawal address %x", depositData.WithdrawalCredentials)
	}
	if !(dkg.MaxEffectiveBalanceInGwei == depositData.Amount) {
		return nil, fmt.Errorf("deposit data is invalid. Wrong amount %d", depositData.Amount)
	}
	forkbytes := network.GenesisForkVersion()
	depositDataJson := &DepositDataJson{
		PubKey:                hex.EncodeToString(validatorPubKey.Serialize()),
		WithdrawalCredentials: hex.EncodeToString(depositData.WithdrawalCredentials),
		Amount:                dkg.MaxEffectiveBalanceInGwei,
		Signature:             hex.EncodeToString(reconstructedDepositMasterSig.Serialize()),
		DepositMessageRoot:    hex.EncodeToString(depositMsgRoot[:]),
		DepositDataRoot:       hex.EncodeToString(root[:]),
		ForkVersion:           hex.EncodeToString(forkbytes[:]),
		NetworkName:           string(network),
		DepositCliVersion:     DepositCliVersion,
	}

	return depositDataJson, nil
}

// StartDKG starts DKG ceremony at initiator with requested parameters
func (c *Initiator) StartDKG(id [24]byte, withdraw []byte, ids []uint64, network eth2_key_manager_core.Network, owner common.Address, nonce uint64) (*DepositDataJson, *KeyShares, *CeremonySigs, error) {

	ops, err := ValidatedOperatorData(ids, c.Operators)
	if err != nil {
		return nil, nil, nil, err
	}

	pkBytes, err := crypto.EncodePublicKey(&c.PrivateKey.PublicKey)
	if err != nil {
		return nil, nil, nil, err
	}

	instanceIDField := zap.String("instance_id", hex.EncodeToString(id[:]))
	c.Logger.Info("🚀 Starting dkg ceremony", zap.String("initiator_id", string(pkBytes)), zap.Uint64s("operator_ids", ids), instanceIDField)

	// compute threshold (3f+1)
	threshold := len(ids) - ((len(ids) - 1) / 3)
	// make init message
	init := &wire.Init{
		Operators:             ops,
		T:                     uint64(threshold),
		WithdrawalCredentials: withdraw,
		Fork:                  network.GenesisForkVersion(),
		Owner:                 owner,
		Nonce:                 nonce,
		InitiatorPublicKey:    pkBytes,
	}
	c.Logger = c.Logger.With(instanceIDField)

	dkgResultsBytes, err := c.messageFlowHandling(init, id, ops)
	if err != nil {
		return nil, nil, nil, err
	}
	dkgResults, err := parseDKGResultsFromBytes(dkgResultsBytes, id)
	if err != nil {
		return nil, nil, nil, err
	}
	c.Logger.Info("🏁 DKG completed, verifying deposit data and ssv payload")
	depositDataJson, keyshares, err := c.processDKGResultResponseInitial(dkgResults, init)
	if err != nil {
		return nil, nil, nil, err
	}
	c.Logger.Info("✅ verified master signature for ssv contract data")
	if err := c.ValidateDepositJSON(depositDataJson); err != nil {
		return nil, nil, nil, err
	}
	// sending back to operators results
	depositData, err := json.Marshal(depositDataJson)
	if err != nil {
		return nil, nil, nil, err
	}
	keysharesData, err := json.Marshal(keyshares)
	if err != nil {
		return nil, nil, nil, err
	}
	ceremonySigs, err := c.getCeremonySigs(dkgResults)
	if err != nil {
		return nil, nil, nil, err
	}
	ceremonySigsBytes, err := json.Marshal(ceremonySigs)
	if err != nil {
		return nil, nil, nil, err
	}
	cSigBytes, err := hex.DecodeString(ceremonySigs.Sigs)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := c.ValidateKeysharesJSON(keyshares, cSigBytes, id, init, depositDataJson.PubKey); err != nil {
		return nil, nil, nil, err
	}
	resultMsg := &wire.ResultData{
		Operators:     ops,
		Identifier:    id,
		DepositData:   depositData,
		KeysharesData: keysharesData,
		CeremonySigs:  ceremonySigsBytes,
	}
	err = c.sendResult(resultMsg, ops, consts.API_RESULTS_URL, id)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("🤖 Error storing results at operators %w", err)
	}
	return depositDataJson, keyshares, ceremonySigs, nil
}

func (c *Initiator) StartReshare(id [24]byte, newOpIDs []uint64, keysharesFile, ceremonySigs []byte, nonce uint64) (*KeyShares, *CeremonySigs, error) {
	var ks *KeyShares
	if err := json.Unmarshal(keysharesFile, &ks); err != nil {
		return nil, nil, err
	}
	var cSigs *CeremonySigs
	if err := json.Unmarshal(ceremonySigs, &cSigs); err != nil {
		return nil, nil, err
	}
	cSigBytes, err := hex.DecodeString(cSigs.Sigs)
	if err != nil {
		return nil, nil, err
	}
	oldOpIDs := ks.Shares[0].Payload.OperatorIDs
	owner := common.HexToAddress(ks.Shares[0].OwnerAddress)
	oldOps, err := ValidatedOperatorData(oldOpIDs, c.Operators)
	if err != nil {
		return nil, nil, err
	}
	newOps, err := ValidatedOperatorData(newOpIDs, c.Operators)
	if err != nil {
		return nil, nil, err
	}
	pkBytes, err := crypto.EncodePublicKey(&c.PrivateKey.PublicKey)
	if err != nil {
		return nil, nil, err
	}
	instanceIDField := zap.String("instance_id", hex.EncodeToString(id[:]))
	c.Logger.Info("🚀 Starting ReSHARING ceremony", zap.String("initiator_id", string(pkBytes)), zap.Uint64s("old_operator_ids", oldOpIDs), zap.Uint64s("new_operator_ids", newOpIDs), instanceIDField)
	// compute threshold (3f+1)
	oldThreshold := len(oldOpIDs) - ((len(oldOpIDs) - 1) / 3)
	newThreshold := len(newOpIDs) - ((len(newOpIDs) - 1) / 3)
	sharesData, err := hex.DecodeString(ks.Shares[0].Payload.SharesData[2:])
	if err != nil {
		return nil, nil, err
	}
	reshare := &wire.Reshare{
		OldOperators:       oldOps,
		NewOperators:       newOps,
		OldT:               uint64(oldThreshold),
		NewT:               uint64(newThreshold),
		Owner:              owner,
		Nonce:              nonce,
		Keyshares:          sharesData,
		CeremonySigs:       cSigBytes,
		InitiatorPublicKey: pkBytes,
	}
	dkgResultsBytes, err := c.messageFlowHandlingReshare(reshare, id, oldOps, newOps)
	if err != nil {
		return nil, nil, err
	}
	dkgResults, err := parseDKGResultsFromBytes(dkgResultsBytes, id)
	if err != nil {
		return nil, nil, err
	}
	c.Logger.Info("🏁 DKG completed, verifying deposit data and ssv payload")
	keyshares, err := c.processDKGResultResponseResharing(dkgResults, reshare)
	if err != nil {
		return nil, nil, err
	}
	c.Logger.Info("✅ verified master signature for ssv contract data")
	// sending back to operators results
	keysharesData, err := json.Marshal(keyshares)
	if err != nil {
		return nil, nil, err
	}
	ceremonySigsNew, err := c.getCeremonySigs(dkgResults)
	if err != nil {
		return nil, nil, err
	}
	ceremonySigsNewBytes, err := json.Marshal(ceremonySigsNew)
	if err != nil {
		return nil, nil, err
	}
	resultMsg := &wire.ResultData{
		Operators:     newOps,
		Identifier:    id,
		DepositData:   nil,
		KeysharesData: keysharesData,
		CeremonySigs:  ceremonySigsNewBytes,
	}
	err = c.sendResult(resultMsg, newOps, consts.API_RESULTS_URL, id)
	if err != nil {
		c.Logger.Error("🤖 Error storing results at operators", zap.Error(err))
	}
	return keyshares, ceremonySigsNew, nil
}

type KeySign struct {
	ValidatorPK ssvspec_types.ValidatorPK
	SigningRoot []byte
}

// Encode returns a msg encoded bytes or error
func (msg *KeySign) Encode() ([]byte, error) {
	return json.Marshal(msg)
}

// Decode returns error if decoding failed
func (msg *KeySign) Decode(data []byte) error {
	return json.Unmarshal(data, msg)
}

// CreateVerifyFunc creates function to verify each participating operator RSA signature for incoming to initiator messages
func CreateVerifyFunc(ops Operators) func(id uint64, msg []byte, sig []byte) error {
	inst_ops := make(map[uint64]*rsa.PublicKey)
	for _, op := range ops {
		inst_ops[op.ID] = op.PubKey
	}
	return func(id uint64, msg []byte, sig []byte) error {
		pk, ok := inst_ops[id]
		if !ok {
			return fmt.Errorf("cant find operator, was it provided at operators information file %d", id)
		}
		return crypto.VerifyRSA(pk, msg, sig)
	}
}

// processDKGResultResponseInitial deserializes incoming DKG result messages from operators after successful initiation ceremony
func (c *Initiator) processDKGResultResponseInitial(dkgResults []dkg.Result, init *wire.Init) (*DepositDataJson, *KeyShares, error) {
	// check results sorted by operatorID
	sorted := sort.SliceIsSorted(dkgResults, func(p, q int) bool {
		return dkgResults[p].OperatorID < dkgResults[q].OperatorID
	})
	if !sorted {
		return nil, nil, fmt.Errorf("slice is not sorted")
	}
	depositDataJson, err := c.reconstructAndVerifyDepositData(dkgResults, init)
	if err != nil {
		return nil, nil, err
	}
	c.Logger.Info("✅ deposit data was successfully reconstructed")
	keyshares, err := c.generateSSVKeysharesPayload(dkgResults, init.Owner, init.Nonce, init.Operators)
	if err != nil {
		return nil, nil, err
	}
	return depositDataJson, keyshares, nil
}

// processDKGResultResponseResharing deserializes incoming DKG result messages from operators after successful resharing ceremony
func (c *Initiator) processDKGResultResponseResharing(dkgResults []dkg.Result, reshare *wire.Reshare) (*KeyShares, error) {
	// check results sorted by operatorID
	sorted := sort.SliceIsSorted(dkgResults, func(p, q int) bool {
		return dkgResults[p].OperatorID < dkgResults[q].OperatorID
	})
	if !sorted {
		return nil, fmt.Errorf("slice is not sorted")
	}
	keyshares, err := c.generateSSVKeysharesPayload(dkgResults, reshare.Owner, reshare.Nonce, reshare.NewOperators)
	if err != nil {
		return nil, err
	}
	return keyshares, nil
}

func (c *Initiator) prepareDepositSigsAndPubs(dkgResults []dkg.Result) ([]*bls.PublicKey, []*bls.Sign, error) {
	sharePks := make([]*bls.PublicKey, 0)
	sigDepositShares := make([]*bls.Sign, 0)
	for i := 0; i < len(dkgResults); i++ {
		sharePubKey := &bls.PublicKey{}
		if err := sharePubKey.Deserialize(dkgResults[i].SharePubKey); err != nil {
			return nil, nil, err
		}
		sharePks = append(sharePks, sharePubKey)
		depositShareSig := &bls.Sign{}
		if dkgResults[i].DepositPartialSignature != nil {
			if err := depositShareSig.Deserialize(dkgResults[i].DepositPartialSignature); err != nil {
				return nil, nil, err
			}
			sigDepositShares = append(sigDepositShares, depositShareSig)
		}
	}
	return sharePks, sigDepositShares, nil
}

func (c *Initiator) prepareOwnerNonceSigs(dkgResults []dkg.Result, owner [20]byte, nonce uint64) ([]*bls.Sign, error) {
	sharePks := make([]*bls.PublicKey, 0)
	ssvContractOwnerNonceSigShares := make([]*bls.Sign, 0)
	for i := 0; i < len(dkgResults); i++ {
		sharePubKey := &bls.PublicKey{}
		if err := sharePubKey.Deserialize(dkgResults[i].SharePubKey); err != nil {
			return nil, err
		}
		sharePks = append(sharePks, sharePubKey)
		ownerNonceShareSig := &bls.Sign{}
		if err := ownerNonceShareSig.Deserialize(dkgResults[i].OwnerNoncePartialSignature); err != nil {
			return nil, err
		}
		ssvContractOwnerNonceSigShares = append(ssvContractOwnerNonceSigShares, ownerNonceShareSig)
	}
	// Verify partial signatures for SSV contract owner+nonce and recovered threshold signature
	data := []byte(fmt.Sprintf("%s:%d", common.Address(owner).String(), nonce))
	hash := eth_crypto.Keccak256([]byte(data))
	err := crypto.VerifyPartialSigs(ssvContractOwnerNonceSigShares, sharePks, hash)
	if err != nil {
		return nil, err
	}
	c.Logger.Info("✅ verified partial signatures from operators")
	return ssvContractOwnerNonceSigShares, nil
}

func parseDKGResultsFromBytes(responseResult [][]byte, id [24]byte) (dkgResults []dkg.Result, err error) {
	for i := 0; i < len(responseResult); i++ {
		msg := responseResult[i]
		tsp := &wire.SignedTransport{}
		if err := tsp.UnmarshalSSZ(msg); err != nil {
			return nil, err
		}
		// check message type
		if tsp.Message.Type == wire.ErrorMessageType {
			var msgErr string
			err := json.Unmarshal(tsp.Message.Data, &msgErr)
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("%s", msgErr)
		}
		if tsp.Message.Type != wire.OutputMessageType {
			return nil, fmt.Errorf("wrong DKG result message type")
		}
		result := dkg.Result{}
		if err := result.Decode(tsp.Message.Data); err != nil {
			return nil, err
		}
		if !bytes.Equal(result.RequestID[:], id[:]) {
			return nil, fmt.Errorf("DKG result has wrong ID")
		}
		dkgResults = append(dkgResults, result)
	}
	// sort the results by operatorID
	sort.SliceStable(dkgResults, func(i, j int) bool {
		return dkgResults[i].OperatorID < dkgResults[j].OperatorID
	})
	for i := 0; i < len(dkgResults); i++ {
		if len(dkgResults[i].ValidatorPubKey) == 0 || !bytes.Equal(dkgResults[i].ValidatorPubKey, dkgResults[0].ValidatorPubKey) {
			return nil, fmt.Errorf("operator %d sent wrong validator public key", dkgResults[i].OperatorID)
		}
	}
	return dkgResults, nil
}

// SendInitMsg sends initial DKG ceremony message to participating operators from initiator
func (c *Initiator) SendInitMsg(init *wire.Init, id [24]byte, operators []*wire.Operator) ([][]byte, error) {
	signedInitMsgBts, err := c.prepareAndSignMessage(init, wire.InitMessageType, id, c.Version)
	if err != nil {
		return nil, err
	}
	return c.SendToAll(consts.API_INIT_URL, signedInitMsgBts, operators)
}

func (c *Initiator) SendReshareMsg(reshare *wire.Reshare, id [24]byte, ops []*wire.Operator) ([][]byte, error) {
	signedReshareMsgBts, err := c.prepareAndSignMessage(reshare, wire.ReshareMessageType, id, c.Version)
	if err != nil {
		return nil, err
	}
	return c.SendToAll(consts.API_RESHARE_URL, signedReshareMsgBts, ops)
}

func (c *Initiator) SendValidateKeysharesMsg(init *wire.ValidateKeyshares, id [24]byte, operators []*wire.Operator) ([][]byte, error) {
	signedValidateMsgBts, err := c.prepareAndSignMessage(init, wire.ValidateKeysharesType, id, c.Version)
	if err != nil {
		return nil, err
	}
	return c.SendToAll(consts.API_VALIDATE_KEYSHARES, signedValidateMsgBts, operators)
}

// SendExchangeMsgs sends combined exchange messages to each operator participating in DKG ceremony
func (c *Initiator) SendExchangeMsgs(exchangeMsgs [][]byte, id [24]byte, operators []*wire.Operator) ([][]byte, error) {
	mltpl, err := c.MakeMultiple(id, exchangeMsgs)
	if err != nil {
		return nil, err
	}
	mltplbyts, err := mltpl.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return c.SendToAll(consts.API_DKG_URL, mltplbyts, operators)
}

// SendKyberMsgs sends combined kyber messages to each operator participating in DKG ceremony
func (c *Initiator) SendKyberMsgs(kyberDeals [][]byte, id [24]byte, operators []*wire.Operator) ([][]byte, error) {
	mltpl2, err := c.MakeMultiple(id, kyberDeals)
	if err != nil {
		return nil, err
	}

	mltpl2byts, err := mltpl2.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return c.SendToAll(consts.API_DKG_URL, mltpl2byts, operators)
}

func (c *Initiator) SendPingMsg(ping *wire.Ping, operators []*wire.Operator) ([][]byte, error) {
	signedPingMsgBts, err := c.prepareAndSignMessage(ping, wire.PingMessageType, [24]byte{}, c.Version)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	return c.SendToAll(consts.API_HEALTH_CHECK_URL, signedPingMsgBts, operators)
}

func (c *Initiator) sendResult(resData *wire.ResultData, operators []*wire.Operator, method string, id [24]byte) error {
	signedMsgBts, err := c.prepareAndSignMessage(resData, wire.ResultMessageType, id, c.Version)
	if err != nil {
		return err
	}
	_, err = c.SendToAll(method, signedMsgBts, operators)
	if err != nil {
		return err
	}
	return nil
}

// LoadOperatorsJson deserialize operators data from JSON
func LoadOperatorsJson(operatorsMetaData []byte) (Operators, error) {
	opmap := make(map[uint64]Operator)
	var operators []OperatorDataJson
	err := json.Unmarshal(bytes.TrimSpace(operatorsMetaData), &operators)
	if err != nil {
		return nil, err
	}
	for _, opdata := range operators {
		_, err := url.ParseRequestURI(opdata.Addr)
		if err != nil {
			return nil, fmt.Errorf("invalid operator URL %s", err.Error())
		}
		operatorKeyByte, err := base64.StdEncoding.DecodeString(opdata.PubKey)
		if err != nil {
			return nil, err
		}
		pemBlock, _ := pem.Decode(operatorKeyByte)
		if pemBlock == nil {
			return nil, errors.New("decode PEM block")
		}
		pbKey, err := x509.ParsePKIXPublicKey(pemBlock.Bytes)
		if err != nil {
			return nil, err
		}

		opmap[opdata.ID] = Operator{
			Addr:   strings.TrimRight(opdata.Addr, "/"),
			ID:     opdata.ID,
			PubKey: pbKey.(*rsa.PublicKey),
		}
	}
	return opmap, nil
}

func (c *Initiator) Ping(ips []string) error {
	client := req.C()
	// Set timeout for operator responses
	client.SetTimeout(30 * time.Second)
	resc := make(chan pongResult, len(ips))
	for _, ip := range ips {
		go func(ip string) {
			resdata, err := c.GetAndCollect(Operator{Addr: ip}, consts.API_HEALTH_CHECK_URL)
			resc <- pongResult{
				ip:     ip,
				err:    err,
				result: resdata,
			}
		}(ip)
	}
	for i := 0; i < len(ips); i++ {
		res := <-resc
		err := c.processPongMessage(res)
		if err != nil {
			c.Logger.Error("😥 Operator not healthy: ", zap.Error(err), zap.String("IP", res.ip))
			continue
		}
	}
	return nil
}

func (c *Initiator) prepareAndSignMessage(msg wire.SSZMarshaller, msgType wire.TransportType, identifier [24]byte, v []byte) ([]byte, error) {
	// Marshal the provided message
	marshaledMsg, err := msg.MarshalSSZ()
	if err != nil {
		return nil, err
	}

	// Create the transport message
	transportMsg := &wire.Transport{
		Type:       msgType,
		Identifier: identifier,
		Data:       marshaledMsg,
		Version:    v,
	}

	// Marshal the transport message
	tssz, err := transportMsg.MarshalSSZ()
	if err != nil {
		return nil, err
	}

	// Sign the message
	sig, err := crypto.SignRSA(c.PrivateKey, tssz)
	if err != nil {
		return nil, err
	}

	// Create and marshal the signed transport message
	signedTransportMsg := &wire.SignedTransport{
		Message:   transportMsg,
		Signer:    0, // Ensure this value is correctly set as per your application logic
		Signature: sig,
	}
	return signedTransportMsg.MarshalSSZ()
}

func (c *Initiator) processPongMessage(res pongResult) error {
	if res.err != nil {
		return res.err
	}
	signedPongMsg := &wire.SignedTransport{}
	if err := signedPongMsg.UnmarshalSSZ(res.result); err != nil {
		errmsg, parseErr := ParseAsError(res.result)
		if parseErr == nil {
			return fmt.Errorf("operator returned err: %v", errmsg)
		}
		return err
	}
	// Validate that incoming message is an pong message
	if signedPongMsg.Message.Type != wire.PongMessageType {
		return fmt.Errorf("wrong incoming message type from operator")
	}
	pong := &wire.Pong{}
	if err := pong.UnmarshalSSZ(signedPongMsg.Message.Data); err != nil {
		return err
	}
	pongBytes, err := signedPongMsg.Message.MarshalSSZ()
	if err != nil {
		return err
	}
	pub, err := crypto.ParseRSAPubkey(pong.PubKey)
	if err != nil {
		return err
	}
	if err := crypto.VerifyRSA(pub, pongBytes, signedPongMsg.Signature); err != nil {
		return err
	}
	c.Logger.Info("🍎 operator online and healthy", zap.String("ID", fmt.Sprint(signedPongMsg.Signer)), zap.String("IP", res.ip), zap.String("Version", string(signedPongMsg.Message.Version)), zap.String("Public key", string(pong.PubKey)))
	return nil
}

func (c *Initiator) getCeremonySigs(dkgResults []dkg.Result) (*CeremonySigs, error) {
	// order the results by operatorID
	sort.SliceStable(dkgResults, func(i, j int) bool {
		return dkgResults[i].OperatorID < dkgResults[j].OperatorID
	})
	ceremonySigs := &CeremonySigs{}
	var sigsBytes []byte
	for i := 0; i < len(dkgResults); i++ {
		ceremonySigs.OperatorIDs = append(ceremonySigs.OperatorIDs, dkgResults[i].OperatorID)
		sigsBytes = append(sigsBytes, dkgResults[i].CeremonySig...)
	}
	ceremonySigs.Sigs = hex.EncodeToString(sigsBytes)
	encInitPub, err := crypto.EncodePublicKey(&c.PrivateKey.PublicKey)
	if err != nil {
		return nil, err
	}
	ceremonySigs.InitiatorPublicKey = hex.EncodeToString(encInitPub)
	ceremonySigs.ValidatorPubKey = "0x" + hex.EncodeToString(dkgResults[0].ValidatorPubKey)
	return ceremonySigs, nil
}

func (c *Initiator) ValidateDepositJSON(d *DepositDataJson) error {
	// 1. Validate format
	if err := validateFieldFormatting(d); err != nil {
		return err
	}
	// 2. Verify deposit roots and signature
	if err := verifyDepositRoots(d); err != nil {
		return nil
	}
	return nil
}

func validateFieldFormatting(d *DepositDataJson) error {
	// check existence of required keys
	if d.PubKey == "" ||
		d.WithdrawalCredentials == "" ||
		d.Amount == 0 ||
		d.Signature == "" ||
		d.DepositMessageRoot == "" ||
		d.DepositDataRoot == "" ||
		d.ForkVersion == "" ||
		d.DepositCliVersion == "" {
		return fmt.Errorf("resulting deposit data json has wrong format")
	}
	// check type of values
	if reflect.TypeOf(d.PubKey).String() != "string" ||
		reflect.TypeOf(d.WithdrawalCredentials).String() != "string" ||
		reflect.TypeOf(d.Amount).String() != "phase0.Gwei" ||
		reflect.TypeOf(d.Signature).String() != "string" ||
		reflect.TypeOf(d.DepositMessageRoot).String() != "string" ||
		reflect.TypeOf(d.DepositDataRoot).String() != "string" ||
		reflect.TypeOf(d.ForkVersion).String() != "string" ||
		reflect.TypeOf(d.DepositCliVersion).String() != "string" {
		return fmt.Errorf("resulting deposit data json has wrong fields type")
	}
	// check length of strings (note: using string length, so 1 byte = 2 chars)
	if len(d.PubKey) != 96 ||
		len(d.WithdrawalCredentials) != 64 ||
		len(d.Signature) != 192 ||
		len(d.DepositMessageRoot) != 64 ||
		len(d.DepositDataRoot) != 64 ||
		len(d.ForkVersion) != 8 {
		return fmt.Errorf("resulting deposit data json has wrong fields length")
	}
	// check the deposit amount
	if d.Amount != 32000000000 {
		return fmt.Errorf("resulting deposit data json has wrong amount")
	}
	v, err := version.NewVersion(d.DepositCliVersion)
	if err != nil {
		return err
	}
	vMin, err := version.NewVersion("2.7.0")
	if err != nil {
		return err
	}
	// check the deposit-cli version
	if v.LessThan(vMin) {
		return fmt.Errorf("resulting deposit data json has wrong amount")
	}
	return nil
}

func verifyDepositRoots(d *DepositDataJson) error {
	pubKey, err := hex.DecodeString(d.PubKey)
	if err != nil {
		return err
	}
	withdrCreds, err := hex.DecodeString(d.WithdrawalCredentials)
	if err != nil {
		return err
	}
	sig, err := hex.DecodeString(d.Signature)
	if err != nil {
		return err
	}
	fork, err := hex.DecodeString(d.ForkVersion)
	if err != nil {
		return err
	}
	depositData := &phase0.DepositData{
		PublicKey:             phase0.BLSPubKey(pubKey),
		WithdrawalCredentials: withdrCreds,
		Amount:                d.Amount,
		Signature:             phase0.BLSSignature(sig),
	}
	depositVerRes, err := crypto.VerifyDepositData(depositData, utils.GetNetworkByFork([4]byte(fork)))
	if err != nil || !depositVerRes {
		return fmt.Errorf("failed to verify deposit data: %v", err)
	}
	return nil
}

func (c *Initiator) ValidateKeysharesJSON(ks *KeyShares, cSigsBytes []byte, id [24]byte, init *wire.Init, valPub string) error {
	if ks.Version != string(c.KeysharesVersion) {
		return fmt.Errorf("keyshares version mismatch at json file")
	}
	if ks.CreatedAt.String() == "" {
		return fmt.Errorf("keyshares creation time is empty")
	}
	// 1. check operators at json
	for i, op := range ks.Shares[0].Operators {
		if op.ID != init.Operators[i].ID || op.OperatorKey != string(init.Operators[i].PubKey) {
			return fmt.Errorf("incorrect keyshares creation time is empty")
		}
	}
	// 2. check owner address is correct
	owner := common.HexToAddress(ks.Shares[0].OwnerAddress)
	if owner != init.Owner {
		return fmt.Errorf("incorrect keyshares owner")
	}
	// 3. check nonce is correct
	if ks.Shares[0].OwnerNonce != init.Nonce {
		return fmt.Errorf("incorrect keyshares nonce")
	}
	// 4. check validator public key
	validatorPublicKey, err := hex.DecodeString(strings.TrimPrefix(ks.Shares[0].PublicKey, "0x"))
	if err != nil {
		return fmt.Errorf("cant decode validator pub key %w", err)
	}
	if "0x"+valPub != ks.Shares[0].PublicKey {
		return fmt.Errorf("incorrect keyshares validator pub key")
	}
	// 5. check operator IDs
	for i, op := range init.Operators {
		if ks.Shares[0].Payload.OperatorIDs[i] != op.ID {
			return fmt.Errorf("incorrect keyshares operator IDs")
		}
	}
	// 6. check validator public key at payload
	if "0x"+valPub != ks.Shares[0].Payload.PublicKey {
		return fmt.Errorf("incorrect keyshares payload validator pub key")
	}
	// 7. check encrypded shares data
	sharesData, err := hex.DecodeString(strings.TrimPrefix(ks.Shares[0].Payload.SharesData, "0x"))
	if err != nil {
		return fmt.Errorf("cant decode enc shares %w", err)
	}
	operatorCount := len(init.Operators)
	signatureOffset := phase0.SignatureLength
	pubKeysOffset := phase0.PublicKeyLength*operatorCount + signatureOffset
	sharesExpectedLength := crypto.EncryptedKeyLength*operatorCount + pubKeysOffset
	if len(sharesData) != sharesExpectedLength {
		return fmt.Errorf("shares data len is not correct")
	}
	signature := sharesData[:signatureOffset]
	err = crypto.VerifyOwnerNonceSignature(signature, owner, validatorPublicKey, uint16(ks.Shares[0].OwnerNonce))
	if err != nil {
		return fmt.Errorf("owner+nonce signature is invalid at keyshares json %w", err)
	}
	// 8. send encrypted shares to decrypt the share and sign on test data.
	// This way we can verify that operators able to decrypt and BLS threshold holds
	validationMsg := &wire.ValidateKeyshares{
		Operators:          init.Operators,
		T:                  init.T,
		Keyshares:          sharesData,
		CeremonySigs:       cSigsBytes,
		InitiatorPublicKey: init.InitiatorPublicKey,
	}
	results, err := c.SendValidateKeysharesMsg(validationMsg, id, init.Operators)
	if err != nil {
		return err
	}
	err = c.VerifyAll(id, results)
	if err != nil {
		return err
	}
	recon, err := crypto.ReconstructSignatures(ks.Shares[0].Payload.OperatorIDs, results)
	if err != nil {
		return err
	}
	err = crypto.VerifyReconstructedSignature(recon, validatorPublicKey, id[:])
	if err != nil {
		return err
	}
	return nil
}
