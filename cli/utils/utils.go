package utils

import (
	"context"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	keystorev4 "github.com/wealdtech/go-eth2-wallet-encryptor-keystorev4"
	"go.uber.org/zap"

	"github.com/bloxapp/ssv-dkg/cli/flags"
	"github.com/bloxapp/ssv-dkg/pkgs/crypto"
	"github.com/bloxapp/ssv-dkg/pkgs/initiator"
	"github.com/bloxapp/ssv-dkg/pkgs/utils"
	"github.com/bloxapp/ssv/logging"
	"github.com/bloxapp/ssv/storage/basedb"
	"github.com/bloxapp/ssv/utils/rsaencryption"
)

// global base flags
var (
	ConfigPath     string
	OutputPath     string
	LogLevel       string
	LogFormat      string
	LogLevelFormat string
	LogFilePath    string
)

// init flags
var (
	OperatorsInfo                     string
	OperatorIDs                       []string
	GenerateInitiatorKeyIfNotExisting bool
	WithdrawAddress                   common.Address
	Network                           string
	OwnerAddress                      common.Address
	Nonce                             uint64
	Validators                        uint64
)

// reshare flags
var (
	NewOperatorIDs []string
	CeremonyID     [24]byte
)

// operator flags
var (
	PrivKey         string
	PrivKeyPassword string
	Port            uint64
	OperatorID      uint64
	DBPath          string
	DBReporting     bool
	DBGCInterval    string
)

// SetViperConfig reads a yaml config file if provided
func SetViperConfig(cmd *cobra.Command) error {
	if err := viper.BindPFlag("configPath", cmd.PersistentFlags().Lookup("configPath")); err != nil {
		return err
	}
	ConfigPath = viper.GetString("configPath")
	var configPathYAML string
	switch cmd.Use {
	case "init":
		configPathYAML = fmt.Sprintf("%s/init.yaml", ConfigPath)
	case "reshare":
		configPathYAML = fmt.Sprintf("%s/reshare.yaml", ConfigPath)
	case "start-operator":
		configPathYAML = fmt.Sprintf("%s/config.yaml", ConfigPath)
	}
	_, err := os.Stat(configPathYAML)
	if !os.IsNotExist(err) {
		viper.SetConfigType("yaml")
		viper.SetConfigFile(configPathYAML)
		if err := viper.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return err
			}
		}
		return nil
	}
	return nil
}

// SetGlobalLogger creates a logger
func SetGlobalLogger(cmd *cobra.Command, name string) (*zap.Logger, error) {
	// If the log file doesn't exist, create it
	_, err := os.OpenFile(LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := logging.SetGlobalLogger(LogLevel, LogFormat, LogLevelFormat, &logging.LogFileOptions{FileName: LogFilePath}); err != nil {
		return nil, fmt.Errorf("logging.SetGlobalLogger: %w", err)
	}
	logger := zap.L().Named(name)
	return logger, nil
}

// OpenPrivateKey reads an RSA key from file.
// If passwordFilePath is provided, treats privKeyPath as encrypted
func OpenPrivateKey(passwordFilePath, privKeyPath string) (*rsa.PrivateKey, error) {
	// check if a password string a valid path, then read password from the file
	if _, err := os.Stat(passwordFilePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("😥 Password file doesn`t exist: %s", err)
	}
	encryptedRSAJSON, err := os.ReadFile(privKeyPath)
	if err != nil {
		return nil, fmt.Errorf("😥 Cant read operator`s key file: %s", err)
	}
	keyStorePassword, err := os.ReadFile(passwordFilePath)
	if err != nil {
		return nil, fmt.Errorf("😥 Error reading password file: %s", err)
	}
	privateKey, err := crypto.ReadEncryptedPrivateKey(encryptedRSAJSON, string(keyStorePassword))
	if err != nil {
		return nil, fmt.Errorf("😥 Error converting pem to priv key: %s", err)
	}
	return privateKey, nil
}

// GenerateRSAKeyPair generates a RSA key pair. Password either supplied as path or generated at random.
func GenerateRSAKeyPair(passwordFilePath, privKeyPath string, logger *zap.Logger) (*rsa.PrivateKey, []byte, error) {
	var privateKey *rsa.PrivateKey
	var err error
	var password string
	_, priv, err := rsaencryption.GenerateKeys()
	if err != nil {
		return nil, nil, fmt.Errorf("😥 Failed to generate operator keys: %s", err)
	}
	if passwordFilePath != "" {
		logger.Info("🔑 path to password file is provided")
		// check if a password string a valid path, then read password from the file
		if _, err := os.Stat(passwordFilePath); os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("😥 Password file doesn`t exist: %s", err)
		}
		keyStorePassword, err := os.ReadFile(passwordFilePath)
		if err != nil {
			return nil, nil, fmt.Errorf("😥 Error reading password file: %s", err)
		}
		password = string(keyStorePassword)
	} else {
		password, err = crypto.GenerateSecurePassword()
		if err != nil {
			return nil, nil, fmt.Errorf("😥 Failed to generate operator keys: %s", err)
		}
	}
	encryptedData, err := keystorev4.New().Encrypt(priv, password)
	if err != nil {
		return nil, nil, fmt.Errorf("😥 Failed to encrypt private key: %s", err)
	}
	encryptedRSAJSON, err := json.Marshal(encryptedData)
	if err != nil {
		return nil, nil, fmt.Errorf("😥 Failed to marshal encrypted data to JSON: %s", err)
	}
	privateKey, err = crypto.ReadEncryptedPrivateKey(encryptedRSAJSON, password)
	if err != nil {
		return nil, nil, fmt.Errorf("😥 Error converting pem to priv key: %s", err)
	}
	return privateKey, encryptedRSAJSON, nil
}

// ReadOperatorsInfoFile reads operators data from path
func ReadOperatorsInfoFile(operatorsInfoPath string, logger *zap.Logger) (initiator.Operators, error) {
	var opMap initiator.Operators
	fmt.Printf("📖 looking operators info 'operators_info.json' file: %s \n", operatorsInfoPath)
	_, err := os.Stat(operatorsInfoPath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("😥 Failed to read operator info file: %s", err)
	}
	logger.Info("📖 reading operators info JSON file")
	opsfile, err := os.ReadFile(operatorsInfoPath)
	if err != nil {
		return nil, fmt.Errorf("😥 Failed to read operator info file: %s", err)
	}
	opMap, err = initiator.LoadOperatorsJson(opsfile)
	if err != nil {
		return nil, fmt.Errorf("😥 Failed to load operators: %s", err)
	}
	return opMap, nil
}

func SetBaseFlags(cmd *cobra.Command) {
	flags.ResultPathFlag(cmd)
	flags.ConfigPathFlag(cmd)
	flags.LogLevelFlag(cmd)
	flags.LogFormatFlag(cmd)
	flags.LogLevelFormatFlag(cmd)
	flags.LogFilePathFlag(cmd)

}

func SetInitFlags(cmd *cobra.Command) {
	SetBaseFlags(cmd)
	flags.OperatorsInfoFlag(cmd)
	flags.OperatorIDsFlag(cmd)
	flags.OwnerAddressFlag(cmd)
	flags.NonceFlag(cmd)
	flags.NetworkFlag(cmd)
	flags.GenerateInitiatorKeyIfNotExistingFlag(cmd)
	flags.WithdrawAddressFlag(cmd)
	flags.ValidatorsFlag(cmd)
}

func SetReshareFlags(cmd *cobra.Command) {
	SetInitFlags(cmd)
	flags.OldIDFlag(cmd)
	flags.NewOperatorIDsFlag(cmd)
}

func SetOperatorFlags(cmd *cobra.Command) {
	SetBaseFlags(cmd)
	flags.PrivateKeyFlag(cmd)
	flags.PrivateKeyPassFlag(cmd)
	flags.OperatorPortFlag(cmd)
	flags.OperatorIDFlag(cmd)
	flags.DBPathFlag(cmd)
	flags.DBReportingFlag(cmd)
	flags.DBGCIntervalFlag(cmd)
}

func SetHealthCheckFlags(cmd *cobra.Command) {
	flags.AddPersistentStringSliceFlag(cmd, "ip", []string{}, "Operator ip:port", true)
}

// BindFlags binds flags to yaml config parameters
func BindBaseFlags(cmd *cobra.Command) error {
	if err := viper.BindPFlag("outputPath", cmd.PersistentFlags().Lookup("outputPath")); err != nil {
		return err
	}
	if err := viper.BindPFlag("logLevel", cmd.PersistentFlags().Lookup("logLevel")); err != nil {
		return err
	}
	if err := viper.BindPFlag("logFormat", cmd.PersistentFlags().Lookup("logFormat")); err != nil {
		return err
	}
	if err := viper.BindPFlag("logLevelFormat", cmd.PersistentFlags().Lookup("logLevelFormat")); err != nil {
		return err
	}
	if err := viper.BindPFlag("logFilePath", cmd.PersistentFlags().Lookup("logFilePath")); err != nil {
		return err
	}
	OutputPath = viper.GetString("outputPath")
	if strings.Contains(OutputPath, "../") {
		return fmt.Errorf("😥 outputPath should not contain traversal")
	}
	if stat, err := os.Stat(OutputPath); err != nil || !stat.IsDir() {
		return fmt.Errorf("😥 Error to to open path to store results %s", err.Error())
	}
	LogLevel = viper.GetString("logLevel")
	LogFormat = viper.GetString("logFormat")
	LogLevelFormat = viper.GetString("logLevelFormat")
	LogFilePath = viper.GetString("logFilePath")
	if strings.Contains(LogFilePath, "../") {
		return fmt.Errorf("😥 logFilePath should not contain traversal")
	}
	return nil
}

// BindInitiatorBaseFlags binds flags to yaml config parameters
func BindInitiatorBaseFlags(cmd *cobra.Command) error {
	var err error
	if err := BindBaseFlags(cmd); err != nil {
		return err
	}
	if err := viper.BindPFlag("configPath", cmd.PersistentFlags().Lookup("configPath")); err != nil {
		return err
	}
	if err := viper.BindPFlag("operatorIDs", cmd.PersistentFlags().Lookup("operatorIDs")); err != nil {
		return err
	}
	if err := viper.BindPFlag("operatorsInfo", cmd.PersistentFlags().Lookup("operatorsInfo")); err != nil {
		return err
	}
	if err := viper.BindPFlag("owner", cmd.PersistentFlags().Lookup("owner")); err != nil {
		return err
	}
	if err := viper.BindPFlag("nonce", cmd.PersistentFlags().Lookup("nonce")); err != nil {
		return err
	}
	ConfigPath = viper.GetString("configPath")
	if strings.Contains(ConfigPath, "../") {
		return fmt.Errorf("😥 configPath should not contain traversal")
	}
	stat, err := os.Stat(ConfigPath)
	if err != nil {
		return fmt.Errorf("😥 %s", err)
	}
	if !stat.IsDir() {
		return fmt.Errorf("😥 configPath isnt a folder path")
	}
	OperatorIDs = viper.GetStringSlice("operatorIDs")
	if len(OperatorIDs) == 0 {
		return fmt.Errorf("😥 Operator IDs flag cant be empty")
	}
	OperatorsInfo = viper.GetString("operatorsInfo")
	owner := viper.GetString("owner")
	if owner == "" {
		return fmt.Errorf("😥 Failed to get owner address flag value")
	}
	OwnerAddress, err = utils.HexToAddress(owner)
	if err != nil {
		return fmt.Errorf("😥 Failed to parse owner address: %s", err)
	}
	Nonce = viper.GetUint64("nonce")
	return nil
}

// BindInitFlags binds flags to yaml config parameters for the initial DKG
func BindInitFlags(cmd *cobra.Command) error {
	if err := BindInitiatorBaseFlags(cmd); err != nil {
		return err
	}
	if err := viper.BindPFlag("generateInitiatorKeyIfNotExisting", cmd.PersistentFlags().Lookup("generateInitiatorKeyIfNotExisting")); err != nil {
		return err
	}
	if err := viper.BindPFlag("withdrawAddress", cmd.PersistentFlags().Lookup("withdrawAddress")); err != nil {
		return err
	}
	if err := viper.BindPFlag("network", cmd.Flags().Lookup("network")); err != nil {
		return err
	}
	if err := viper.BindPFlag("validators", cmd.Flags().Lookup("validators")); err != nil {
		return err
	}
	withdrawAddr := viper.GetString("withdrawAddress")
	if withdrawAddr == "" {
		return fmt.Errorf("😥 Failed to get withdrawal address flag value")
	}
	var err error
	WithdrawAddress, err = utils.HexToAddress(withdrawAddr)
	if err != nil {
		return fmt.Errorf("😥 Failed to parse withdraw address: %s", err.Error())
	}
	Network = viper.GetString("network")
	if Network == "" {
		return fmt.Errorf("😥 Failed to get fork version flag value")
	}
	Validators = viper.GetUint64("validators")
	if Validators > 100 || Validators == 0 {
		return fmt.Errorf("🚨 Amount of generated validators should be 1 to 100")
	}
	GenerateInitiatorKeyIfNotExisting = viper.GetBool("generateInitiatorKeyIfNotExisting")
	return nil
}

// BindReshareFlags binds flags to yaml config parameters for the resharing ceremony of DKG
func BindReshareFlags(cmd *cobra.Command) error {
	if err := BindInitiatorBaseFlags(cmd); err != nil {
		return err
	}
	if err := viper.BindPFlag("newOperatorIDs", cmd.PersistentFlags().Lookup("newOperatorIDs")); err != nil {
		return err
	}
	if err := viper.BindPFlag("oldID", cmd.PersistentFlags().Lookup("oldID")); err != nil {
		return err
	}
	NewOperatorIDs = viper.GetStringSlice("newOperatorIDs")
	if len(NewOperatorIDs) == 0 {
		return fmt.Errorf("😥 New operator IDs flag cant be empty")
	}
	var err error
	id := viper.GetString("oldID")
	oldIDFlagValue, err := hex.DecodeString(id)
	if err != nil {
		return err
	}
	copy(CeremonyID[:], oldIDFlagValue)
	return nil
}

// BindOperatorFlags binds flags to yaml config parameters for the resharing ceremony of DKG
func BindOperatorFlags(cmd *cobra.Command) error {
	if err := BindBaseFlags(cmd); err != nil {
		return err
	}
	if err := viper.BindPFlag("privKey", cmd.PersistentFlags().Lookup("privKey")); err != nil {
		return err
	}
	if err := viper.BindPFlag("privKeyPassword", cmd.PersistentFlags().Lookup("privKeyPassword")); err != nil {
		return err
	}
	if err := viper.BindPFlag("port", cmd.PersistentFlags().Lookup("port")); err != nil {
		return err
	}
	if err := viper.BindPFlag("operatorID", cmd.PersistentFlags().Lookup("operatorID")); err != nil {
		return err
	}
	if err := viper.BindPFlag("DBPath", cmd.PersistentFlags().Lookup("DBPath")); err != nil {
		return err
	}
	if err := viper.BindPFlag("DBReporting", cmd.PersistentFlags().Lookup("DBReporting")); err != nil {
		return err
	}
	if err := viper.BindPFlag("DBGCInterval", cmd.PersistentFlags().Lookup("DBGCInterval")); err != nil {
		return err
	}
	PrivKey = viper.GetString("privKey")
	PrivKeyPassword = viper.GetString("privKeyPassword")
	if PrivKey == "" {
		return fmt.Errorf("😥 Failed to get private key path flag value")
	}
	if PrivKeyPassword == "" {
		return fmt.Errorf("😥 Failed to get password for private key flag value")
	}
	Port = viper.GetUint64("port")
	if Port == 0 {
		return fmt.Errorf("😥 Wrong port provided")
	}
	OperatorID = viper.GetUint64("operatorID")
	if OperatorID == 0 {
		return fmt.Errorf("😥 Wrong operator ID provided")
	}
	DBPath = viper.GetString("DBPath")
	if strings.Contains(DBPath, "../") {
		return fmt.Errorf("😥 DBPath should not contain traversal")
	}
	DBReporting = viper.GetBool("DBReporting")
	DBGCInterval = viper.GetString("DBGCInterval")
	return nil
}

// StingSliceToUintArray converts the string slice to uint64 slice
func StingSliceToUintArray(flagdata []string) ([]uint64, error) {
	partsarr := make([]uint64, 0, len(flagdata))
	for i := 0; i < len(flagdata); i++ {
		opid, err := strconv.ParseUint(flagdata[i], 10, strconv.IntSize)
		if err != nil {
			return nil, fmt.Errorf("😥 cant load operator err: %v , data: %v, ", err, flagdata[i])
		}
		partsarr = append(partsarr, opid)
	}
	// sort array
	sort.SliceStable(partsarr, func(i, j int) bool {
		return partsarr[i] < partsarr[j]
	})
	sorted := sort.SliceIsSorted(partsarr, func(p, q int) bool {
		return partsarr[p] < partsarr[q]
	})
	if !sorted {
		return nil, fmt.Errorf("slice isnt sorted")
	}
	return partsarr, nil
}

// LoadOperators loads operators data from raw json or file path
func LoadOperators(logger *zap.Logger) (initiator.Operators, error) {
	var opmap map[uint64]initiator.Operator
	var err error
	if OperatorsInfo != "" {
		opmap, err = initiator.LoadOperatorsJson([]byte(OperatorsInfo))
		if err != nil {
			return nil, err
		}
	} else {
		operatorsInfoPath := fmt.Sprintf("%s/operators_info.json", ConfigPath)
		opmap, err = ReadOperatorsInfoFile(operatorsInfoPath, logger)
		if err != nil {
			return nil, err
		}
	}
	return opmap, nil
}

// LoadInitiatorRSAPrivKey loads RSA private key from path or generates a new key pair
func LoadInitiatorRSAPrivKey(generate bool) (*rsa.PrivateKey, error) {
	var privateKey *rsa.PrivateKey
	privKeyPath := fmt.Sprintf("%s/initiator_encrypted_key.json", ConfigPath)
	privKeyPassPath := fmt.Sprintf("%s/initiator_password", ConfigPath)
	if generate {
		if _, err := os.Stat(privKeyPath); os.IsNotExist(err) {
			_, priv, err := rsaencryption.GenerateKeys()
			if err != nil {
				return nil, fmt.Errorf("😥 Failed to generate operator keys: %s", err)
			}
			if _, err := os.Stat(privKeyPassPath); os.IsNotExist(err) {
				password, err := crypto.GenerateSecurePassword()
				if err != nil {
					return nil, err
				}
				err = os.WriteFile(privKeyPassPath, []byte(password), 0o600)
				if err != nil {
					return nil, err
				}
			}
			keyStorePassword, err := os.ReadFile(privKeyPassPath)
			if err != nil {
				return nil, fmt.Errorf("😥 Error reading password file: %s", err)
			}
			encryptedRSAJSON, err := crypto.EncryptPrivateKey(priv, string(keyStorePassword))
			if err != nil {
				return nil, fmt.Errorf("😥 Failed to marshal encrypted data to JSON: %s", err)
			}
			privateKey, err = crypto.ReadEncryptedPrivateKey(encryptedRSAJSON, string(keyStorePassword))
			if err != nil {
				return nil, fmt.Errorf("😥 Error converting pem to priv key: %s", err)
			}
			err = os.WriteFile(privKeyPath, encryptedRSAJSON, 0o600)
			if err != nil {
				return nil, err
			}
		} else if err == nil {
			return crypto.ReadEncryptedRSAKey(privKeyPath, privKeyPassPath)
		}
	} else {
		// check if a password string a valid path, then read password from the file
		if _, err := os.Stat(privKeyPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("🔑 private key file: %s", err)
		}
		if _, err := os.Stat(privKeyPassPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("🔑 password file: %s", err)
		}
		return crypto.ReadEncryptedRSAKey(privKeyPath, privKeyPassPath)
	}
	return privateKey, nil
}

// GetOperatorDB creates a new Badger DB instance at provided path
func GetOperatorDB() (basedb.Options, error) {
	var DBOptions basedb.Options
	var err error
	DBOptions.Path = DBPath
	DBOptions.Reporting = DBReporting
	DBOptions.GCInterval, err = time.ParseDuration(DBGCInterval)
	if err != nil {
		return basedb.Options{}, fmt.Errorf("😥 Failed to parse DBGCInterval: %s", err)
	}
	DBOptions.Ctx = context.Background()
	if err != nil {
		return basedb.Options{}, fmt.Errorf("😥 Failed to open DB: %s", err)
	}
	return DBOptions, nil
}

func WriteInitResults(depositDataArr []*initiator.DepositDataJson, keySharesArr []*initiator.KeyShares, nonces []uint64, ids [][24]byte, logger *zap.Logger) {
	if len(depositDataArr) != int(Validators) || len(keySharesArr) != int(Validators) {
		logger.Fatal("Incoming result arrays have inconsistent length")
	}
	timestamp := time.Now().Format(time.RFC3339)
	dir := fmt.Sprintf("%s/ceremony-%s", OutputPath, timestamp)
	err := os.Mkdir(dir, os.ModePerm)
	if err != nil {
		logger.Fatal("Failed to create a ceremony directory: ", zap.Error(err))
	}
	for i := 0; i < int(Validators); i++ {
		nestedDir := fmt.Sprintf("%s/0x%s", dir, depositDataArr[i].PubKey)
		err := os.Mkdir(nestedDir, os.ModePerm)
		if err != nil {
			logger.Fatal("Failed to create a validator key directory: ", zap.Error(err))
		}
		logger.Info("💾 Writing deposit data json to file", zap.String("path", nestedDir))
		err = WriteDepositResult(depositDataArr[i], nestedDir)
		if err != nil {
			logger.Fatal("Failed writing deposit data file: ", zap.Error(err), zap.String("path", nestedDir), zap.Any("deposit", depositDataArr[i]))
		}
		logger.Info("💾 Writing keyshares payload to file", zap.String("path", nestedDir))
		err = WriteKeysharesResult(keySharesArr[i], nestedDir, ids[i])
		if err != nil {
			logger.Fatal("Failed writing keyshares file: ", zap.Error(err), zap.String("path", nestedDir), zap.Any("deposit", keySharesArr[i]))
		}
		logger.Info("💾 Writing instance ID to file", zap.String("path", nestedDir))
		err = WriteInstanceID(nestedDir, ids[i])
		if err != nil {
			logger.Fatal("Failed writing instance ID file: ", zap.Error(err), zap.String("path", nestedDir), zap.String("ID", hex.EncodeToString(ids[i][:])))
		}
	}

	// Write aggregated JSON files
	depositFinalPath := fmt.Sprintf("%s/deposit_data.json", dir)
	logger.Info("💾 Writing deposit data json to file", zap.String("path", depositFinalPath))
	err = utils.WriteJSON(depositFinalPath, depositDataArr)
	if err != nil {
		logger.Fatal("Failed writing deposit data file: ", zap.Error(err), zap.String("path", depositFinalPath), zap.Any("deposits", depositDataArr))
	}
	keysharesFinalPath := fmt.Sprintf("%s/keyshares.json", dir)
	logger.Info("💾 Writing keyshares payload to file", zap.String("path", keysharesFinalPath))
	aggrKeySharesArr, err := initiator.GenerateAggregatesKeyshares(keySharesArr)
	if err != nil {
		logger.Fatal("error: ", zap.Error(err))
	}
	err = utils.WriteJSON(keysharesFinalPath, aggrKeySharesArr)
	if err != nil {
		logger.Fatal("Failed writing instance IDs to file: ", zap.Error(err), zap.String("path", keysharesFinalPath), zap.Any("keyshares", keySharesArr))
	}
	instanceIdsPath := fmt.Sprintf("%s/instance_id.json", dir)
	logger.Info("💾 Writing instance IDs to file", zap.String("path", keysharesFinalPath))
	var idsArr []string
	for _, id := range ids {
		idsArr = append(idsArr, hex.EncodeToString(id[:]))
	}
	err = utils.WriteJSON(instanceIdsPath, idsArr)
	if err != nil {
		logger.Fatal("Failed writing instance IDs to file: ", zap.Error(err), zap.String("path", instanceIdsPath), zap.Strings("IDs", idsArr))
	}
}

func WriteKeysharesResult(keyShares *initiator.KeyShares, dir string, id [24]byte) error {
	keysharesFinalPath := fmt.Sprintf("%s/keyshares-%s-%s-%d-%v.json", dir, keyShares.Shares[0].Payload.PublicKey, keyShares.Shares[0].OwnerAddress, keyShares.Shares[0].OwnerNonce, hex.EncodeToString(id[:]))
	err := utils.WriteJSON(keysharesFinalPath, keyShares)
	if err != nil {
		return fmt.Errorf("failed writing keyshares file: %w, %v", err, keyShares)
	}
	return nil
}

func WriteDepositResult(depositData *initiator.DepositDataJson, dir string) error {
	depositFinalPath := fmt.Sprintf("%s/deposit_data-0x%s.json", dir, depositData.PubKey)
	err := utils.WriteJSON(depositFinalPath, []*initiator.DepositDataJson{depositData})
	if err != nil {
		return fmt.Errorf("failed writing deposit data file: %w, %v", err, depositData)
	}
	return nil
}

func WriteInstanceID(dir string, id [24]byte) error {
	instanceIdPath := fmt.Sprintf("%s/instance_id.json", dir)
	err := utils.WriteJSON(instanceIdPath, hex.EncodeToString(id[:]))
	if err != nil {
		return fmt.Errorf("failed writing instance ID file: %w, %s", err, hex.EncodeToString(id[:]))
	}
	return nil
}
