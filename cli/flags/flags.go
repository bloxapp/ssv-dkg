package flags

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Flag names.
const (
	threshold       = "threshold"
	withdrawAddress = "withdrawAddress"
	operatorIDs     = "operatorIDs"
	operatorsInfo   = "operatorsInfoPath"
	operatorPrivKey = "privKey"
	operatorPort    = "port"
	owner           = "owner"
	nonce           = "nonce"
	fork            = "fork"
)

// ThresholdFlag adds threshold flag to the command
func ThresholdFlag(c *cobra.Command) {
	AddPersistentIntFlag(c, threshold, 3, "Threshold for distributed signature", true)
}

// GetThresholdFlagValue gets threshold flag from the command
func GetThresholdFlagValue(c *cobra.Command) (uint64, error) {
	return c.Flags().GetUint64(threshold)
}

// WithdrawAddressFlag  adds withdraw address flag to the command
func WithdrawAddressFlag(c *cobra.Command) {
	AddPersistentStringFlag(c, withdrawAddress, "", "Withdrawal address", true)
}

// GetWithdrawAddressFlagValue gets withdraw address flag from the command
func GetWithdrawAddressFlagValue(c *cobra.Command) (string, error) {
	return c.Flags().GetString(withdrawAddress)
}

// operatorIDsFlag adds operators IDs flag to the command
func OperatorIDsFlag(c *cobra.Command) {
	AddPersistentStringSliceFlag(c, operatorIDs, []string{"1", "2", "3"}, "Operator IDs", true)
}

// GetThresholdFlagValue gets operators IDs flag from the command
func GetoperatorIDsFlagValue(c *cobra.Command) ([]string, error) {
	return c.Flags().GetStringSlice(operatorIDs)
}

// OperatorsInfoFlag  adds path to operators' ifo file flag to the command
func OperatorsInfoFlag(c *cobra.Command) {
	AddPersistentStringFlag(c, operatorsInfo, "", "Path to operators' public keys, IDs and IPs file", true)
}

// GetOperatorsInfoFlagValue gets path to operators' ifo file flag from the command
func GetOperatorsInfoFlagValue(c *cobra.Command) (string, error) {
	return c.Flags().GetString(operatorsInfo)
}

// OwnerAddressFlag  adds owner address flag to the command
func OwnerAddressFlag(c *cobra.Command) {
	AddPersistentStringFlag(c, owner, "", "Owner address", true)
}

// GetOwnerAddressFlagValue gets owner address flag from the command
func GetOwnerAddressFlagValue(c *cobra.Command) (string, error) {
	return c.Flags().GetString(owner)
}

// NonceFlag  owner nonce flag to the command
func NonceFlag(c *cobra.Command) {
	AddPersistentIntFlag(c, nonce, 0, "Owner nonce", true)
}

// GetNonceFlagValue gets owner nonce flag from the command
func GetNonceFlagValue(c *cobra.Command) (uint64, error) {
	return c.Flags().GetUint64(nonce)
}

// ForkVersionFlag  adds the fork version of the network flag to the command
func ForkVersionFlag(c *cobra.Command) {
	AddPersistentStringFlag(c, fork, "", "Fork version", true)
}

// GetForkVersionFlagValue gets the fork version of the network flag from the command
func GetForkVersionFlagValue(c *cobra.Command) ([4]byte, string, error) {
	fork, err := c.Flags().GetString(fork)
	if err != nil {
		return [4]byte{}, "", err
	}
	switch fork {
	case "prater":
		return [4]byte{0x00, 0x00, 0x10, 0x20}, "prater", nil
	case "mainnet":
		return [4]byte{0, 0, 0, 0}, "mainnet", nil
	case "now_test_network":
		return [4]byte{0x99, 0x99, 0x99, 0x99}, "now_test_network", nil
	default:
		return [4]byte{0, 0, 0, 0}, "mainnet", nil
	}
}

// OperatorPrivateKeyFlag  adds private key flag to the command
func OperatorPrivateKeyFlag(c *cobra.Command) {
	AddPersistentStringFlag(c, operatorPrivKey, "", "Path to operator Private Key file", false)
}

// GetOperatorPrivateKeyFlagValue gets private key flag from the command
func GetOperatorPrivateKeyFlagValue(c *cobra.Command) (string, error) {
	return c.Flags().GetString(operatorPrivKey)
}

// OperatorPortFlag  adds operator listening port flag to the command
func OperatorPortFlag(c *cobra.Command) {
	AddPersistentIntFlag(c, operatorPort, 3030, "Operator Private Key hex", false)
}

// GetOperatorPortFlagValue gets operator listening port flag from the command
func GetOperatorPortFlagValue(c *cobra.Command) (uint64, error) {
	return c.Flags().GetUint64(operatorPort)
}

// AddPersistentStringFlag adds a string flag to the command
func AddPersistentStringFlag(c *cobra.Command, flag string, value string, description string, isRequired bool) {
	req := ""
	if isRequired {
		req = " (required)"
	}

	c.PersistentFlags().String(flag, value, fmt.Sprintf("%s%s", description, req))

	if isRequired {
		_ = c.MarkPersistentFlagRequired(flag)
	}
}

// AddPersistentIntFlag adds a int flag to the command
func AddPersistentIntFlag(c *cobra.Command, flag string, value uint64, description string, isRequired bool) {
	req := ""
	if isRequired {
		req = " (required)"
	}

	c.PersistentFlags().Uint64(flag, value, fmt.Sprintf("%s%s", description, req))

	if isRequired {
		_ = c.MarkPersistentFlagRequired(flag)
	}
}

// AddPersistentStringArrayFlag adds a string array flag to the command
func AddPersistentStringArrayFlag(c *cobra.Command, flag string, value []string, description string, isRequired bool) {
	req := ""
	if isRequired {
		req = " (required)"
	}

	c.PersistentFlags().StringArray(flag, value, fmt.Sprintf("%s%s", description, req))

	if isRequired {
		_ = c.MarkPersistentFlagRequired(flag)
	}
}

// AddPersistentStringArrayFlag adds a string slice flag to the command
func AddPersistentStringSliceFlag(c *cobra.Command, flag string, value []string, description string, isRequired bool) {
	req := ""
	if isRequired {
		req = " (required)"
	}

	c.PersistentFlags().StringSlice(flag, value, fmt.Sprintf("%s%s", description, req))

	if isRequired {
		_ = c.MarkPersistentFlagRequired(flag)
	}
}