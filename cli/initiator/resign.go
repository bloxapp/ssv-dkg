package initiator

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	cli_utils "github.com/bloxapp/ssv-dkg/cli/utils"
	"github.com/bloxapp/ssv-dkg/pkgs/crypto"
	"github.com/bloxapp/ssv-dkg/pkgs/initiator"
)

func init() {
	cli_utils.SetResignFlags(StartReSign)
}

var StartReSign = &cobra.Command{
	Use:   "resign",
	Short: "Resign data at existing operators",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(`
		▓█████▄  ██ ▄█▀  ▄████     ██▀███  ▓█████   ██████  ██░ ██  ▄▄▄       ██▀███  ▓█████ 
		▒██▀ ██▌ ██▄█▒  ██▒ ▀█▒   ▓██ ▒ ██▒▓█   ▀ ▒██    ▒ ▓██░ ██▒▒████▄    ▓██ ▒ ██▒▓█   ▀ 
		░██   █▌▓███▄░ ▒██░▄▄▄░   ▓██ ░▄█ ▒▒███   ░ ▓██▄   ▒██▀▀██░▒██  ▀█▄  ▓██ ░▄█ ▒▒███   
		░▓█▄   ▌▓██ █▄ ░▓█  ██▓   ▒██▀▀█▄  ▒▓█  ▄   ▒   ██▒░▓█ ░██ ░██▄▄▄▄██ ▒██▀▀█▄  ▒▓█  ▄ 
		░▒████▓ ▒██▒ █▄░▒▓███▀▒   ░██▓ ▒██▒░▒████▒▒██████▒▒░▓█▒░██▓ ▓█   ▓██▒░██▓ ▒██▒░▒████▒
		▒▒▓  ▒ ▒ ▒▒ ▓▒ ░▒   ▒    ░ ▒▓ ░▒▓░░░ ▒░ ░▒ ▒▓▒ ▒ ░ ▒ ░░▒░▒ ▒▒   ▓▒█░░ ▒▓ ░▒▓░░░ ▒░ ░
		░ ▒  ▒ ░ ░▒ ▒░  ░   ░      ░▒ ░ ▒░ ░ ░  ░░ ░▒  ░ ░ ▒ ░▒░ ░  ▒   ▒▒ ░  ░▒ ░ ▒░ ░ ░  ░
		░ ░  ░ ░ ░░ ░ ░ ░   ░      ░░   ░    ░   ░  ░  ░   ░  ░░ ░  ░   ▒     ░░   ░    ░   
		░    ░  ░         ░       ░        ░  ░      ░   ░  ░  ░      ░  ░   ░        ░  ░
		░`)
		if err := cli_utils.SetViperConfig(cmd); err != nil {
			return err
		}
		if err := cli_utils.BindResignFlags(cmd); err != nil {
			return err
		}
		logger, err := cli_utils.SetGlobalLogger(cmd, "dkg-initiator")
		if err != nil {
			return err
		}
		defer logger.Sync()
		logger.Info("🪛 Initiator`s", zap.String("Version", cmd.Version))
		opMap, err := cli_utils.LoadOperators(logger)
		if err != nil {
			logger.Fatal("😥 Failed to load operators: ", zap.Error(err))
		}
		logger.Info("🔑 opening initiator RSA private key file")
		privateKey, err := cli_utils.LoadInitiatorRSAPrivKey(false)
		if err != nil {
			logger.Fatal("😥 Failed to load private key: ", zap.Error(err))
		}
		keyshares, err := cli_utils.LoadKeyShares(cli_utils.KeysharesFilePath)
		if err != nil {
			logger.Fatal("😥 Failed to read keyshares json file:", zap.Error(err))
		}
		// Start resigning
		ctx := context.Background()
		pool := pool.NewWithResults[*ResignResult]().WithContext(ctx).WithFirstError().WithMaxGoroutines(maxConcurrency)
		for i := 0; i < len(keyshares.Shares); i++ {
			i := i
			pool.Go(func(ctx context.Context) (*ResignResult, error) {
				// Create new DKG initiator
				dkgInitiator := initiator.New(privateKey, opMap.Clone(), logger, cmd.Version)
				// Create a new ID.
				id := crypto.NewID()
				// Perform the ceremony.
				exitSig, validator, err := dkgInitiator.StartResigning(id, &keyshares.Shares[i], [32]byte{})
				if err != nil {
					return nil, err
				}
				logger.Debug("DKG ceremony completed",
					zap.String("id", hex.EncodeToString(id[:])),
				)
				return &ResignResult{
					id:      id,
					valPub:  validator,
					exitSig: exitSig,
				}, nil
			})
		}
		results, err := pool.Wait()
		if err != nil {
			logger.Fatal("😥 Failed to initiate DKG ceremony: ", zap.Error(err))
		}
		for _, res := range results {
			logger.Info("Exit message sig", zap.String("validator", hex.EncodeToString(res.valPub)), zap.String("full sig", hex.EncodeToString(res.exitSig)))
		}
		return nil
	},
}

type ResignResult struct {
	id      [24]byte
	valPub  []byte
	exitSig []byte
}
