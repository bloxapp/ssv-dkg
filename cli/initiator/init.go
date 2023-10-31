package initiator

import (
	"crypto/rsa"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/bloxapp/ssv-dkg/cli/flags"
	cli_utils "github.com/bloxapp/ssv-dkg/cli/utils"
	"github.com/bloxapp/ssv-dkg/pkgs/crypto"
	"github.com/bloxapp/ssv-dkg/pkgs/initiator"
	"github.com/bloxapp/ssv-dkg/pkgs/utils"
)

func init() {
	flags.InitiatorPrivateKeyFlag(StartDKG)
	flags.InitiatorPrivateKeyPassFlag(StartDKG)
	flags.GenerateInitiatorKeyFlag(StartDKG)
	flags.WithdrawAddressFlag(StartDKG)
	flags.OperatorsInfoFlag(StartDKG)
	flags.OperatorsInfoPathFlag(StartDKG)
	flags.OperatorIDsFlag(StartDKG)
	flags.OwnerAddressFlag(StartDKG)
	flags.NonceFlag(StartDKG)
	flags.NetworkFlag(StartDKG)
	flags.ResultPathFlag(StartDKG)
	flags.ConfigPathFlag(StartDKG)
	flags.LogLevelFlag(StartDKG)
	flags.LogFormatFlag(StartDKG)
	flags.LogLevelFormatFlag(StartDKG)
	flags.LogFilePathFlag(StartDKG)
	if err := viper.BindPFlag("withdrawAddress", StartDKG.PersistentFlags().Lookup("withdrawAddress")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("operatorIDs", StartDKG.PersistentFlags().Lookup("operatorIDs")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("operatorsInfo", StartDKG.PersistentFlags().Lookup("operatorsInfo")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("operatorsInfoPath", StartDKG.PersistentFlags().Lookup("operatorsInfoPath")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("owner", StartDKG.PersistentFlags().Lookup("owner")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("nonce", StartDKG.PersistentFlags().Lookup("nonce")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("network", StartDKG.PersistentFlags().Lookup("network")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("outputPath", StartDKG.PersistentFlags().Lookup("outputPath")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("initiatorPrivKey", StartDKG.PersistentFlags().Lookup("initiatorPrivKey")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("initiatorPrivKeyPassword", StartDKG.PersistentFlags().Lookup("initiatorPrivKeyPassword")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("generateInitiatorKey", StartDKG.PersistentFlags().Lookup("generateInitiatorKey")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("logLevel", StartDKG.PersistentFlags().Lookup("logLevel")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("logFormat", StartDKG.PersistentFlags().Lookup("logFormat")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("logLevelFormat", StartDKG.PersistentFlags().Lookup("logLevelFormat")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("logFilePath", StartDKG.PersistentFlags().Lookup("logFilePath")); err != nil {
		panic(err)
	}
}

var StartDKG = &cobra.Command{
	Use:   "init",
	Short: "Initiates a DKG protocol",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(`
		█████╗ ██╗  ██╗ ██████╗     ██╗███╗   ██╗██╗████████╗██╗ █████╗ ████████╗ ██████╗ ██████╗ 
		██╔══██╗██║ ██╔╝██╔════╝     ██║████╗  ██║██║╚══██╔══╝██║██╔══██╗╚══██╔══╝██╔═══██╗██╔══██╗
		██║  ██║█████╔╝ ██║  ███╗    ██║██╔██╗ ██║██║   ██║   ██║███████║   ██║   ██║   ██║██████╔╝
		██║  ██║██╔═██╗ ██║   ██║    ██║██║╚██╗██║██║   ██║   ██║██╔══██║   ██║   ██║   ██║██╔══██╗
		██████╔╝██║  ██╗╚██████╔╝    ██║██║ ╚████║██║   ██║   ██║██║  ██║   ██║   ╚██████╔╝██║  ██║
		╚═════╝ ╚═╝  ╚═╝ ╚═════╝     ╚═╝╚═╝  ╚═══╝╚═╝   ╚═╝   ╚═╝╚═╝  ╚═╝   ╚═╝    ╚═════╝ ╚═╝  ╚═╝`)
		err := cli_utils.SetViperConfig(cmd)
		if err != nil {
			return err
		}
		logger, err := cli_utils.SetGlobalLogger(cmd, "dkg-initiator")
		if err != nil {
			return err
		}
		// workaround for https://github.com/spf13/viper/issues/233
		if err := viper.BindPFlag("outputPath", cmd.Flags().Lookup("outputPath")); err != nil {
			logger.Fatal("😥 Failed to bind a flag: ", zap.Error(err))
		}
		outputPath := viper.GetString("outputPath")
		if outputPath == "" {
			logger.Fatal("😥 Failed to get deposit result path flag value: ", zap.Error(err))
		}
		if stat, err := os.Stat(outputPath); err != nil || !stat.IsDir() {
			logger.Fatal("😥 Error to to open path to store results", zap.Error(err))
		}
		// Load operators TODO: add more sources.
		if err := viper.BindPFlag("operatorsInfo", cmd.Flags().Lookup("operatorsInfo")); err != nil {
			logger.Fatal("😥 Failed to bind a flag: ", zap.Error(err))
		}
		operatorsInfo := viper.GetString("operatorsInfo")
		if err := viper.BindPFlag("operatorsInfoPath", cmd.Flags().Lookup("operatorsInfoPath")); err != nil {
			logger.Fatal("😥 Failed to bind a flag: ", zap.Error(err))
		}
		operatorsInfoPath := viper.GetString("operatorsInfoPath")
		if operatorsInfo == "" && operatorsInfoPath == "" {
			logger.Fatal("😥 Operators string or path have not provided")
		}
		if operatorsInfo != "" && operatorsInfoPath != "" {
			logger.Fatal("😥 Please provide either operator info string or path, not both")
		}
		var opMap initiator.Operators
		if operatorsInfo != "" {
			logger.Info("📖 reading raw JSON string of operators info")
			opMap, err = initiator.LoadOperatorsJson([]byte(operatorsInfo))
			if err != nil {
				logger.Fatal("😥 Failed to load operators: ", zap.Error(err))
			}
		}
		if operatorsInfoPath != "" {
			opMap, err = cli_utils.ReadOperatorsInfoFile(operatorsInfoPath)
			if err != nil {
				logger.Fatal(err.Error())
			}
		}
		if err := viper.BindPFlag("operatorIDs", cmd.Flags().Lookup("operatorIDs")); err != nil {
			logger.Fatal("😥 Failed to bind a flag: ", zap.Error(err))
		}
		participants := viper.GetStringSlice("operatorIDs")
		if participants == nil {
			logger.Fatal("😥 Failed to get operator IDs flag value: ", zap.Error(err))
		}
		parts, err := loadParticipants(participants)
		if err != nil {
			logger.Fatal("😥 Failed to load participants: ", zap.Error(err))
		}
		if err := viper.BindPFlag("initiatorPrivKey", cmd.Flags().Lookup("initiatorPrivKey")); err != nil {
			logger.Fatal("😥 Failed to bind a flag: ", zap.Error(err))
		}
		initiatorPrivKey := viper.GetString("initiatorPrivKey")
		generateInitiatorKey := viper.GetBool("generateInitiatorKey")
		if initiatorPrivKey == "" && !generateInitiatorKey {
			logger.Fatal("😥 Initiator key flag should be provided")
		}
		if initiatorPrivKey != "" && generateInitiatorKey {
			logger.Fatal("😥 Please provide either private key path or generate command, not both")
		}
		var privateKey *rsa.PrivateKey
		var encryptedRSAJSON []byte
		var password string
		if err := viper.BindPFlag("initiatorPrivKeyPassword", cmd.Flags().Lookup("initiatorPrivKeyPassword")); err != nil {
			logger.Fatal("😥 Failed to bind a flag: ", zap.Error(err))
		}
		initiatorPrivKeyPassword := viper.GetString("initiatorPrivKeyPassword")
		if initiatorPrivKey != "" && !generateInitiatorKey {
			logger.Info("🔑 opening initiator RSA private key file")
			privateKey, err = cli_utils.OpenPrivateKey(initiatorPrivKeyPassword, initiatorPrivKey)
			if err != nil {
				logger.Fatal(err.Error())
			}
		}
		if initiatorPrivKey == "" && generateInitiatorKey {
			logger.Info("🔑 generating new initiator RSA key pair + password")
			privateKey, encryptedRSAJSON, err = cli_utils.GenerateRSAKeyPair(initiatorPrivKeyPassword, initiatorPrivKey)
			if err != nil {
				logger.Fatal(err.Error())
			}
		}
		dkgInitiator := initiator.New(privateKey, opMap, logger)
		withdrawAddr := viper.GetString("withdrawAddress")
		if withdrawAddr == "" {
			logger.Fatal("😥 Failed to get withdrawal address flag value: ", zap.Error(err))
		}
		if err := viper.BindPFlag("network", cmd.Flags().Lookup("network")); err != nil {
			logger.Fatal("😥 Failed to bind a flag: ", zap.Error(err))
		}
		network := viper.GetString("network")
		if network == "" {
			logger.Fatal("😥 Failed to get fork version flag value: ", zap.Error(err))
		}
		var forkHEX [4]byte
		switch network {
		case "prater":
			forkHEX = [4]byte{0x00, 0x00, 0x10, 0x20}
		case "pyrmont":
			forkHEX = [4]byte{0x00, 0x00, 0x20, 0x09}
		case "mainnet":
			forkHEX = [4]byte{0, 0, 0, 0}
		default:
			logger.Fatal("😥 Please provide a valid network name: mainnet/prater/pyrmont")
		}
		if err := viper.BindPFlag("owner", cmd.Flags().Lookup("owner")); err != nil {
			logger.Fatal("😥 Failed to bind a flag: ", zap.Error(err))
		}
		owner := viper.GetString("owner")
		if owner == "" {
			logger.Fatal("😥 Failed to get owner address flag value: ", zap.Error(err))
		}
		ownerAddress, err := utils.HexToAddress(owner)
		if err != nil {
			logger.Fatal("😥 Failed to parse owner address: ", zap.Error(err))
		}
		if err := viper.BindPFlag("nonce", cmd.Flags().Lookup("nonce")); err != nil {
			logger.Fatal("😥 Failed to bind a flag: ", zap.Error(err))
		}
		nonce := viper.GetUint64("nonce")
		withdrawAddress, err := utils.HexToAddress(withdrawAddr)
		if err != nil {
			logger.Fatal("😥 Failed to parse withdraw address: ", zap.Error(err))
		}
		// create a new ID
		id := crypto.NewID()
		// Start the ceremony
		depositData, keyShares, err := dkgInitiator.StartDKG(id, withdrawAddress.Bytes(), parts, forkHEX, network, ownerAddress, nonce)
		if err != nil {
			logger.Fatal("😥 Failed to initiate DKG ceremony: ", zap.Error(err))
		}
		// Save deposit file
		logger.Info("🎯  All data is validated.")
		depositFinalPath := fmt.Sprintf("%s/deposit_%s.json", outputPath, depositData.PubKey)
		logger.Info("💾 Writing deposit data json to file", zap.String("path", depositFinalPath))
		err = utils.WriteJSON(depositFinalPath, []initiator.DepositDataJson{*depositData})
		if err != nil {
			logger.Warn("Failed writing deposit data file: ", zap.Error(err))
		}
		keysharesFinalPath := fmt.Sprintf("%s/keyshares-%v-%v.json", outputPath, depositData.PubKey, hex.EncodeToString(id[:]))
		logger.Info("💾 Writing keyshares payload to file", zap.String("path", keysharesFinalPath))
		err = utils.WriteJSON(keysharesFinalPath, keyShares)
		if err != nil {
			logger.Warn("Failed writing keyshares file: ", zap.Error(err))
		}
		if initiatorPrivKey == "" && generateInitiatorKey {
			rsaKeyPath := fmt.Sprintf("%s/encrypted_private_key-%v.json", outputPath, depositData.PubKey)
			err = os.WriteFile(rsaKeyPath, encryptedRSAJSON, 0644)
			if err != nil {
				logger.Fatal("Failed to write encrypted private key to file", zap.Error(err))
			}
			if initiatorPrivKeyPassword == "" {
				rsaKeyPasswordPath := fmt.Sprintf("%s/password-%v.txt", outputPath, depositData.PubKey)
				err = os.WriteFile(rsaKeyPasswordPath, []byte(password), 0644)
				if err != nil {
					logger.Fatal("Failed to write encrypted private key to file", zap.Error(err))
				}
			}
			logger.Info("Private key encrypted and stored at", zap.String("path", outputPath))
		}

		fmt.Println(`
		▓█████▄  ██▓  ██████  ▄████▄   ██▓    ▄▄▄       ██▓ ███▄ ▄███▓▓█████  ██▀███  
		▒██▀ ██▌▓██▒▒██    ▒ ▒██▀ ▀█  ▓██▒   ▒████▄    ▓██▒▓██▒▀█▀ ██▒▓█   ▀ ▓██ ▒ ██▒
		░██   █▌▒██▒░ ▓██▄   ▒▓█    ▄ ▒██░   ▒██  ▀█▄  ▒██▒▓██    ▓██░▒███   ▓██ ░▄█ ▒
		░▓█▄   ▌░██░  ▒   ██▒▒▓▓▄ ▄██▒▒██░   ░██▄▄▄▄██ ░██░▒██    ▒██ ▒▓█  ▄ ▒██▀▀█▄  
		░▒████▓ ░██░▒██████▒▒▒ ▓███▀ ░░██████▒▓█   ▓██▒░██░▒██▒   ░██▒░▒████▒░██▓ ▒██▒
		 ▒▒▓  ▒ ░▓  ▒ ▒▓▒ ▒ ░░ ░▒ ▒  ░░ ▒░▓  ░▒▒   ▓▒█░░▓  ░ ▒░   ░  ░░░ ▒░ ░░ ▒▓ ░▒▓░
		 ░ ▒  ▒  ▒ ░░ ░▒  ░ ░  ░  ▒   ░ ░ ▒  ░ ▒   ▒▒ ░ ▒ ░░  ░      ░ ░ ░  ░  ░▒ ░ ▒░
		 ░ ░  ░  ▒ ░░  ░  ░  ░          ░ ░    ░   ▒    ▒ ░░      ░      ░     ░░   ░ 
		   ░     ░        ░  ░ ░          ░  ░     ░  ░ ░         ░      ░  ░   ░     
		 ░                   ░                                                        
		 
		 This tool was not audited.
		 When using distributed key generation you understand all the risks involved with
		 experimental cryptography.  
		 `)
		return nil
	},
}

func loadParticipants(flagdata []string) ([]uint64, error) {
	partsarr := make([]uint64, 0, len(flagdata))
	for i := 0; i < len(flagdata); i++ {
		opid, err := strconv.ParseUint(flagdata[i], 10, strconv.IntSize)
		if err != nil {
			return nil, fmt.Errorf("😥 cant load operator err: %v , data: %v, ", err, flagdata[i])
		}
		partsarr = append(partsarr, opid)
	}
	return partsarr, nil
}
