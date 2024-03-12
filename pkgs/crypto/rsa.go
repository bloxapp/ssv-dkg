package crypto

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	keystorev4 "github.com/wealdtech/go-eth2-wallet-encryptor-keystorev4"
)

// GenerateSecurePassword randomly generates a password consisting of digits + english letters
func GenerateSecurePassword() (string, error) {
	const alpha = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	var pass []rune
	p := make([]byte, 64)
	if _, err := rand.Reader.Read(p); err != nil {
		return "", err
	}
	hash := sha512.Sum512(p)
	for _, r := range string(hash[:]) {
		if unicode.IsDigit(r) || strings.Contains(alpha, strings.ToLower(string(r))) {
			pass = append(pass, r)
		}
	}
	return string(pass), nil
}

// GenerateRSAKeys creates a random RSA key pair
func GenerateRSAKeys() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	pv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	return pv, &pv.PublicKey, nil

}

// SignRSA create a RSA signature for incoming bytes
func SignRSA(sk *rsa.PrivateKey, byts []byte) ([]byte, error) {
	r := sha256.Sum256(byts)
	return sk.Sign(rand.Reader, r[:], &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthAuto,
		Hash:       crypto.SHA256,
	})
}

// VerifyRSA verifies RSA signature for incoming message
func VerifyRSA(pk *rsa.PublicKey, msg, signature []byte) error {
	r := sha256.Sum256(msg)
	return rsa.VerifyPSS(pk, crypto.SHA256, r[:], signature, nil)
}

// ParseRSAPublicKey parses encoded to base64 x509 RSA public key
func ParseRSAPublicKey(pk []byte) (*rsa.PublicKey, error) {
	operatorKeyByte, err := base64.StdEncoding.DecodeString(string(pk))
	if err != nil {
		return nil, err
	}
	pemblock, _ := pem.Decode(operatorKeyByte)
	if pemblock == nil {
		return nil, errors.New("decode PEM block")
	}
	pbkey, err := x509.ParsePKIXPublicKey(pemblock.Bytes)
	if err != nil {
		return nil, err
	}
	return pbkey.(*rsa.PublicKey), nil
}

func EncodeRSAPublicKey(pk *rsa.PublicKey) ([]byte, error) {
	pkBytes, err := x509.MarshalPKIXPublicKey(pk)
	if err != nil {
		return nil, err
	}
	pemByte := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PUBLIC KEY",
			Bytes: pkBytes,
		},
	)
	if pemByte == nil {
		return nil, fmt.Errorf("failed to encode pub key to pem")
	}

	return []byte(base64.StdEncoding.EncodeToString(pemByte)), nil
}

// EncryptRSAKeystore encrypts an RSA private key using the given password.
func EncryptRSAKeystore(priv []byte, keyStorePassword string) ([]byte, error) {
	encryptedData, err := keystorev4.New().Encrypt(priv, keyStorePassword)
	if err != nil {
		return nil, fmt.Errorf("😥 Failed to encrypt private key: %s", err)
	}
	return json.Marshal(encryptedData)
}

// DecryptRSAKeystore reads an encrypted RSA private key using the given password.
func DecryptRSAKeystore(keyData []byte, password string) (*rsa.PrivateKey, error) {
	if strings.TrimSpace(password) == "" {
		return nil, errors.New("Password required for encrypted PEM block")
	}

	// Unmarshal the JSON-encoded data
	var data map[string]interface{}
	if err := json.Unmarshal(keyData, &data); err != nil {
		return nil, fmt.Errorf("parse JSON data: %w", err)
	}

	// Decrypt the private key using keystorev4
	decryptedBytes, err := keystorev4.New().Decrypt(data, password)
	if err != nil {
		return nil, fmt.Errorf("decrypt private key: %w", err)
	}

	// Parse the decrypted PEM data
	block, _ := pem.Decode(decryptedBytes)
	if block == nil {
		return nil, errors.New("parse PEM block")
	}

	// Parse the RSA private key
	rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse RSA private key: %w", err)
	}

	return rsaKey, nil
}

// OpenRSAKeystore reads an encrypted RSA private key from the given key file and password file.
func OpenRSAKeystore(privKeyPath, privKeyPassPath string) (*rsa.PrivateKey, error) {
	privKey, err := os.ReadFile(filepath.Clean(privKeyPath))
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	keyStorePassword, err := os.ReadFile(filepath.Clean(privKeyPassPath))
	if err != nil {
		return nil, fmt.Errorf("😥 Cant read operator's key file: %s", err)
	}
	return DecryptRSAKeystore(privKey, string(keyStorePassword))
}
