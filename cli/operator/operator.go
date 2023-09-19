package operator

import (
	"crypto/rsa"
	"fmt"
	"log"
	"os"

	"github.com/bloxapp/ssv-dkg/cli/flags"
	"github.com/bloxapp/ssv-dkg/pkgs/crypto"
	"github.com/bloxapp/ssv-dkg/pkgs/operator"

	"github.com/bloxapp/ssv/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func init() {
	flags.OperatorPrivateKeyFlag(StartDKGOperator)
	flags.OperatorPrivateKeyPassFlag(StartDKGOperator)
	flags.OperatorPortFlag(StartDKGOperator)
	flags.AddStoreShareFlag(StartDKGOperator)
	flags.ConfigPathFlag(StartDKGOperator)
	if err := viper.BindPFlag("privKey", StartDKGOperator.PersistentFlags().Lookup("privKey")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("password", StartDKGOperator.PersistentFlags().Lookup("password")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("port", StartDKGOperator.PersistentFlags().Lookup("port")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("storeShare", StartDKGOperator.PersistentFlags().Lookup("storeShare")); err != nil {
		panic(err)
	}
}

var StartDKGOperator = &cobra.Command{
	Use:   "start-operator",
	Short: "Starts an instance of DKG operator",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(`
		██████╗ ██╗  ██╗ ██████╗      ██████╗ ██████╗ ███████╗██████╗  █████╗ ████████╗ ██████╗ ██████╗ 
		██╔══██╗██║ ██╔╝██╔════╝     ██╔═══██╗██╔══██╗██╔════╝██╔══██╗██╔══██╗╚══██╔══╝██╔═══██╗██╔══██╗
		██║  ██║█████╔╝ ██║  ███╗    ██║   ██║██████╔╝█████╗  ██████╔╝███████║   ██║   ██║   ██║██████╔╝
		██║  ██║██╔═██╗ ██║   ██║    ██║   ██║██╔═══╝ ██╔══╝  ██╔══██╗██╔══██║   ██║   ██║   ██║██╔══██╗
		██████╔╝██║  ██╗╚██████╔╝    ╚██████╔╝██║     ███████╗██║  ██║██║  ██║   ██║   ╚██████╔╝██║  ██║
		╚═════╝ ╚═╝  ╚═╝ ╚═════╝      ╚═════╝ ╚═╝     ╚══════╝╚═╝  ╚═╝╚═╝  ╚═╝   ╚═╝    ╚═════╝ ╚═╝  ╚═╝`)
		if err := logging.SetGlobalLogger("debug", "capital", "console"); err != nil {
			log.Fatal(err)
		}
		logger := zap.L().Named("dkg-operator")
		viper.SetConfigType("yaml")
		configPath, err := flags.GetConfigPathFlagValue(cmd)
		if err != nil {
			logger.Fatal(err.Error())
		}
		if configPath != "" {
			viper.AddConfigPath(configPath)
		} else {
			viper.AddConfigPath("./config")
		}
		err = viper.ReadInConfig()
		if err != nil {
			logger.Warn("couldn't find config file, its ok if you are using cli params")
		}
		privKeyPath := viper.GetString("privKey")
		if privKeyPath == "" {
			logger.Fatal("failed to get operator private key flag value", zap.Error(err))
		}
		var privateKey *rsa.PrivateKey
		pass := viper.GetString("password")
		if pass != "" {
			// check if a password string a valid path, then read password from the file
			if _, err := os.Stat(pass); err != nil {
				logger.Fatal("Cant read password file", zap.Error(err))
			}
			keyStorePassword, err := os.ReadFile(pass)
			if err != nil {
				logger.Fatal("Error reading Password file", zap.Error(err))
			}
			encryptedJSON, err := os.ReadFile(privKeyPath)
			if err != nil {
				logger.Fatal("cant read operator`s key file", zap.Error(err))
			}
			privateKey, err = crypto.ConvertEncryptedPemToPrivateKey(encryptedJSON, string(keyStorePassword))
			if err != nil {
				logger.Fatal(err.Error())
			}
		} else {
			logger.Fatal("please provide password string or path to password file", zap.Error(err))
		}
		srv := operator.New(privateKey)
		port := viper.GetUint64("port")
		if port == 0 {
			logger.Fatal("failed to get operator info file path flag value", zap.Error(err))
		}
		pubKey, err := crypto.EncodePublicKey(&privateKey.PublicKey)
		if err != nil {
			logger.Fatal(err.Error())
		}
		logger.Info("starting DKG operator", zap.Uint64("port", port), zap.String("public key", string(pubKey)))
		if err := srv.Start(uint16(port)); err != nil {
			log.Fatalf("Error in operator %v", err)
		}
	},
}
