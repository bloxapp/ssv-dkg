package initiator

import (
	"fmt"
	"sync"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	e2m_core "github.com/bloxapp/eth2-key-manager/core"
	cli_utils "github.com/bloxapp/ssv-dkg/cli/utils"
	"github.com/bloxapp/ssv-dkg/pkgs/crypto"
	"github.com/bloxapp/ssv-dkg/pkgs/initiator"
)

func init() {
	cli_utils.SetInitFlags(StartDKG)
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
		if err := cli_utils.SetViperConfig(cmd); err != nil {
			return err
		}
		if err := cli_utils.BindInitFlags(cmd); err != nil {
			return err
		}
		logger, err := cli_utils.SetGlobalLogger(cmd, "dkg-initiator")
		if err != nil {
			return err
		}
		// Load operators TODO: add more sources.
		operatorIDs, err := cli_utils.StingSliceToUintArray(cli_utils.OperatorIDs)
		if err != nil {
			logger.Fatal("😥 Failed to load participants: ", zap.Error(err))
		}
		opMap, err := cli_utils.LoadOperators()
		if err != nil {
			logger.Fatal("😥 Failed to load operators: ", zap.Error(err))
		}
		logger.Info("🔑 opening initiator RSA private key file")
		privateKey, encryptedRSAJSON, err := cli_utils.LoadRSAPrivKey()
		if err != nil {
			logger.Fatal("😥 Failed to load private key: ", zap.Error(err))
		}
		dkgInitiator := initiator.New(privateKey, opMap, logger)
		ethnetwork := e2m_core.MainNetwork
		if cli_utils.Network != "now_test_network" {
			ethnetwork = e2m_core.NetworkFromString(cli_utils.Network)
		}
		// start the ceremony
		nonce := cli_utils.Nonce
		var wg sync.WaitGroup
		resultChan := make(chan *Result)
		for i := 0; i < int(cli_utils.Validators); i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// create a new ID
				id := crypto.NewID()
				depositData, keyShares, err := dkgInitiator.StartDKG(id, cli_utils.WithdrawAddress.Bytes(), operatorIDs, ethnetwork, cli_utils.OwnerAddress, nonce)
				resultChan <- &Result{
					id:          id,
					depositData: depositData,
					keyShares:   keyShares,
					nonce:       nonce,
					err:         err,
				}
			}()
			nonce++
		}
		go func() {
			wg.Wait()
			close(resultChan)
		}()
		var depositDataArr []*initiator.DepositDataJson
		var keySharesArr []*initiator.KeyShares
		var ids [][24]byte
		var nonces []uint64
		for res := range resultChan {
			if res.err != nil {
				logger.Fatal("😥 Failed to initiate DKG ceremony: ", zap.Error(res.err))
			}
			logger.Info("🎯  Received result.")
			depositDataArr = append(depositDataArr, res.depositData)
			keySharesArr = append(keySharesArr, res.keyShares)
			ids = append(ids, res.id)
			nonces = append(nonces, res.nonce)
		}
		// Save deposit file
		logger.Info("🎯  All data is validated.")
		cli_utils.WriteInitResults(depositDataArr, keySharesArr, nonces, ids, encryptedRSAJSON, logger)
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

type Result struct {
	id          [24]byte
	nonce       uint64
	depositData *initiator.DepositDataJson
	keyShares   *initiator.KeyShares
	err         error
}
