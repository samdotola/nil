package execution

import (
	"crypto/ecdsa"
	"fmt"
	"os"

	"github.com/NilFoundation/nil/nil/common"
	"github.com/NilFoundation/nil/nil/common/check"
	"github.com/NilFoundation/nil/nil/common/hexutil"
	"github.com/NilFoundation/nil/nil/internal/config"
	"github.com/NilFoundation/nil/nil/internal/contracts"
	nilcrypto "github.com/NilFoundation/nil/nil/internal/crypto"
	"github.com/NilFoundation/nil/nil/internal/types"
	"github.com/ethereum/go-ethereum/crypto"
	"gopkg.in/yaml.v3"
)

var (
	MainPrivateKey *ecdsa.PrivateKey
	MainPublicKey  []byte
)

var DefaultZeroStateConfig string

func init() {
	var err error
	MainPrivateKey, MainPublicKey, err = nilcrypto.GenerateKeyPair()
	check.PanicIfErr(err)

	DefaultZeroStateConfig, err = CreateDefaultZeroStateConfig(MainPublicKey)
	check.PanicIfErr(err)
}

type ContractDescr struct {
	Name     string         `yaml:"name"`
	Address  *types.Address `yaml:"address,omitempty"`
	Value    types.Value    `yaml:"value"`
	Shard    types.ShardId  `yaml:"shard,omitempty"`
	Contract string         `yaml:"contract"`
	CtorArgs []any          `yaml:"ctorArgs,omitempty"`
}

type MainKeys struct {
	MainPrivateKey string `yaml:"mainPrivateKey"`
	MainPublicKey  string `yaml:"mainPublicKey"`
}

type ConfigParams struct {
	Validators config.ParamValidators `yaml:"validators,omitempty"`
	GasPrice   config.ParamGasPrice   `yaml:"gasPrice"`
}

type ZeroStateConfig struct {
	ConfigParams ConfigParams     `yaml:"config,omitempty"`
	Contracts    []*ContractDescr `yaml:"contracts"`
}

func (cfg *ZeroStateConfig) GetValidators() []config.ListValidators {
	return cfg.ConfigParams.Validators.Validators
}

func CreateDefaultZeroStateConfig(mainPublicKey []byte) (string, error) {
	zerostate := `
contracts:
- name: MainSmartAccount
  address: {{ .MainSmartAccountAddress }}
  value: 10000000000000000000000
  contract: SmartAccount
  ctorArgs: [{{ .MainPublicKey }}]
- name: Faucet
  address: {{ .FaucetAddress }}
  value: 20000000000000000000000
  contract: Faucet
- name: EthFaucet
  address: {{ .EthFaucetAddress }}
  value: 100000000000000
  contract: FaucetToken
- name: UsdtFaucet
  address: {{ .UsdtFaucetAddress }}
  value: 100000000000000
  contract: FaucetToken
- name: BtcFaucet
  address: {{ .BtcFaucetAddress }}
  value: 100000000000000
  contract: FaucetToken
- name: L1BlockInfo
  address: {{ .L1BlockInfoAddress }}
  value: 0
  contract: system/L1BlockInfo
`

	return common.ParseTemplate(zerostate, map[string]interface{}{
		"MainSmartAccountAddress": types.MainSmartAccountAddress.Hex(),
		"L1BlockInfoAddress":      types.L1BlockInfoAddress.Hex(),
		"MainPublicKey":           hexutil.Encode(MainPublicKey),
		"MainSmartAccountPubKey":  hexutil.Encode(MainPublicKey),
		"FaucetAddress":           types.FaucetAddress.Hex(),
		"EthFaucetAddress":        types.EthFaucetAddress.Hex(),
		"UsdtFaucetAddress":       types.UsdtFaucetAddress.Hex(),
		"BtcFaucetAddress":        types.BtcFaucetAddress.Hex(),
	})
}

func DumpMainKeys(fname string) error {
	keys := MainKeys{"0x" + nilcrypto.PrivateKeyToEthereumFormat(MainPrivateKey), hexutil.Encode(MainPublicKey)}

	data, err := yaml.Marshal(&keys)
	if err != nil {
		return err
	}

	file, err := os.Create(fname)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(data)
	return err
}

func LoadMainKeys(fname string) error {
	var keys MainKeys

	data, err := os.ReadFile(fname)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, &keys); err != nil {
		return err
	}
	MainPrivateKey, err = crypto.HexToECDSA(keys.MainPrivateKey[2:])
	if err != nil {
		return err
	}
	MainPublicKey, err = hexutil.Decode(keys.MainPublicKey)
	return err
}

func CreateZeroStateConfigWithMainPublicKey(mainKeysPath string) (string, error) {
	if err := LoadMainKeys(mainKeysPath); err != nil {
		logger.Debug().Str("path", mainKeysPath).Msg("Failed to load main keys, using default")
	}
	return CreateDefaultZeroStateConfig(MainPublicKey)
}

func (c *ZeroStateConfig) FindContractByName(name string) *ContractDescr {
	for _, contract := range c.Contracts {
		if contract.Name == name {
			return contract
		}
	}
	return nil
}

func (c *ZeroStateConfig) GetContractAddress(name string) *types.Address {
	contract := c.FindContractByName(name)
	if contract != nil {
		return contract.Address
	}
	return nil
}

func ParseZeroStateConfig(configYaml string) (*ZeroStateConfig, error) {
	var config ZeroStateConfig
	err := yaml.Unmarshal([]byte(configYaml), &config)
	return &config, err
}

func (es *ExecutionState) GenerateZeroStateYaml(configYaml string) error {
	config, err := ParseZeroStateConfig(configYaml)
	if err != nil {
		return err
	}
	return es.GenerateZeroState(config)
}

func (es *ExecutionState) GenerateMergedZeroState(leftConfig *ZeroStateConfig, configYaml string) error {
	var rightConfig *ZeroStateConfig
	var err error
	if configYaml != "" {
		if rightConfig, err = ParseZeroStateConfig(configYaml); err != nil {
			return err
		}
	} else {
		rightConfig = &ZeroStateConfig{}
	}
	if leftConfig == nil {
		leftConfig = &ZeroStateConfig{}
	}
	return es.GenerateZeroState(
		&ZeroStateConfig{
			ConfigParams: leftConfig.ConfigParams,
			Contracts:    append(leftConfig.Contracts, rightConfig.Contracts...),
		},
	)
}

func (es *ExecutionState) GenerateZeroState(stateConfig *ZeroStateConfig) error {
	var err error

	if es.ShardId == types.MainShardId {
		cfgAccessor := es.GetConfigAccessor()
		config.InitParams(cfgAccessor)
		err = config.SetParamValidators(cfgAccessor, &stateConfig.ConfigParams.Validators)
		if err != nil {
			return err
		}
		err = config.SetParamGasPrice(cfgAccessor, &stateConfig.ConfigParams.GasPrice)
		if err != nil {
			return err
		}
	}

	if len(stateConfig.ConfigParams.GasPrice.Shards) != 0 {
		check.PanicIfNot(len(stateConfig.ConfigParams.GasPrice.Shards) > int(es.ShardId))
		es.GasPrice = types.Value{Uint256: &stateConfig.ConfigParams.GasPrice.Shards[es.ShardId]}
	} else {
		es.GasPrice = types.DefaultGasPrice
	}

	for _, contract := range stateConfig.Contracts {
		code, err := contracts.GetCode(contract.Contract)
		if err != nil {
			return err
		}
		var addr types.Address
		if contract.Address != nil {
			addr = *contract.Address
		} else {
			addr = types.CreateAddress(contract.Shard, types.BuildDeployPayload(code, common.EmptyHash))
		}

		if addr.ShardId() != es.ShardId {
			continue
		}

		abi, err := contracts.GetAbi(contract.Contract)
		if err != nil {
			return err
		}

		args := make([]any, 0)
		for _, arg := range contract.CtorArgs {
			switch arg := arg.(type) {
			case string:
				switch {
				case arg == "MainPublicKey":
					args = append(args, MainPublicKey)
				case arg[:2] == "0x":
					args = append(args, hexutil.FromHex(arg))
				default:
					return fmt.Errorf("unknown constructor argument string pattern: %s", arg)
				}
			default:
				args = append(args, arg)
			}
		}
		argsPacked, err := abi.Pack("", args...)
		if err != nil {
			return fmt.Errorf("[ZeroState] ctorArgs pack failed: %w", err)
		}
		code = append(code, argsPacked...)

		mainDeployTxn := &types.Transaction{
			TransactionDigest: types.TransactionDigest{
				Flags:        types.NewTransactionFlags(types.TransactionFlagInternal),
				Seqno:        0,
				Data:         code,
				MaxFeePerGas: types.MaxFeePerGasDefault,
			},
		}

		if err := es.CreateAccount(addr); err != nil {
			return err
		}
		if err := es.CreateContract(addr); err != nil {
			return err
		}
		if err := es.SetBalance(addr, contract.Value); err != nil {
			return err
		}
		if err := es.SetInitState(addr, mainDeployTxn); err != nil {
			return err
		}

		logger.Debug().Str("name", contract.Name).Stringer("address", addr).Msg("Created zero state contract")
	}
	return nil
}
