package wire

import "github.com/ethereum/go-ethereum/common"

type MultipleSignedTransports struct {
	Identifier [24]byte           `ssz-size:"24"` // this is kinda wasteful, maybe take it out of the msgs?
	Messages   []*SignedTransport `ssz-max:"13"`  // max num of operators
}

type ErrSSZ struct {
	Error []byte `ssz-max:"512"`
}

type TransportType uint64

const (
	InitMessageType TransportType = iota
	KyberMessageType
	InitReshareMessageType
	ExchangeMessageType
	OutputMessageType
	KyberDealBundleMessageType
	KyberResponseBundleMessageType
	KyberJustificationBundleMessageType
	BlsSignRequestType
	ErrorMessageType
)

func (t TransportType) String() string {
	switch t {
	case InitMessageType:
		return "InitMessageType"
	case KyberMessageType:
		return "KyberMessageType"
	case ExchangeMessageType:
		return "ExchangeMessageType"
	case OutputMessageType:
		return "OutputMessageType"
	case KyberDealBundleMessageType:
		return "KyberDealBundleMessageType"
	case KyberResponseBundleMessageType:
		return "KyberResponseBundleMessageType"
	case KyberJustificationBundleMessageType:
		return "KyberJustificationBundleMessageType"
	case BlsSignRequestType:
		return "BlsSignRequestType"
	case ErrorMessageType:
		return "ErrorMessageType"
	default:
		return "no type impl"
	}
}

type Transport struct {
	Type       TransportType
	Identifier [24]byte `ssz-size:"24"`
	Data       []byte   `ssz-max:"8388608"` // 2^23
}

type SignedTransport struct {
	Message   *Transport
	Signer    uint64
	Signature []byte `ssz-max:"2048"`
}

//
//const (
//	KyberDealBundleMessageType TransportType = iota
//	KyberResponseBundleMessageType
//	KyberJustificationBundleMessageType
//)

type KyberMessage struct {
	Type TransportType
	Data []byte `ssz-max:"2048"`
}

type Operator struct {
	ID     uint64
	PubKey []byte `ssz-max:"2048"`
}

type Init struct {
	// Operators involved in the DKG
	Operators []*Operator `ssz-max:"13"`
	// T is the threshold for signing
	T uint64
	// WithdrawalCredentials for deposit data
	WithdrawalCredentials []byte `ssz-max:"32"` // 2^23
	// Fork ethereum fork for signing
	Fork [4]byte `ssz-size:"4"`
	// Owner address
	Owner common.Address `ssz-size:"20"`
	// Owner nonce
	Nonce uint64
}

// Exchange contains the session auth/ encryption key for each node
type Exchange struct {
	PK []byte `ssz-max:"2048"`
}

type Output struct {
	EncryptedShare              []byte `ssz-max:"2048"`
	SharePK                     []byte `ssz-max:"2048"`
	ValidatorPK                 []byte `ssz-size:"48"`
	DepositDataPartialSignature []byte `ssz-size:"96"`
}
