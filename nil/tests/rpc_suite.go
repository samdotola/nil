//go:build test

package tests

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/NilFoundation/nil/nil/client"
	rpc_client "github.com/NilFoundation/nil/nil/client/rpc"
	"github.com/NilFoundation/nil/nil/common"
	"github.com/NilFoundation/nil/nil/common/check"
	"github.com/NilFoundation/nil/nil/common/logging"
	"github.com/NilFoundation/nil/nil/internal/abi"
	"github.com/NilFoundation/nil/nil/internal/collate"
	"github.com/NilFoundation/nil/nil/internal/db"
	"github.com/NilFoundation/nil/nil/internal/execution"
	"github.com/NilFoundation/nil/nil/internal/keys"
	"github.com/NilFoundation/nil/nil/internal/types"
	"github.com/NilFoundation/nil/nil/services/nilservice"
	"github.com/NilFoundation/nil/nil/services/rpc/jsonrpc"
	"github.com/NilFoundation/nil/nil/services/rpc/transport"
	"github.com/rs/zerolog"
)

var DefaultContractValue = types.GasToValue(500_000_000)

type RpcSuite struct {
	CliRunner

	Context   context.Context
	CtxCancel context.CancelFunc
	Wg        sync.WaitGroup

	DbInit func() db.DB
	Db     db.DB

	Client    client.Client
	ShardsNum uint32
	Endpoint  string
}

func init() {
	logging.SetupGlobalLogger("debug")
}

func PatchConfigWithTestDefaults(cfg *nilservice.Config) {
	if cfg.Topology == "" {
		cfg.Topology = collate.TrivialShardTopologyId
	}
	if cfg.CollatorTickPeriodMs == 0 {
		cfg.CollatorTickPeriodMs = 100
	}
}

func (s *RpcSuite) Start(cfg *nilservice.Config) {
	s.T().Helper()

	s.ShardsNum = cfg.NShards
	s.Context, s.CtxCancel = context.WithCancel(context.Background())

	if s.DbInit == nil {
		s.DbInit = func() db.DB {
			db, err := db.NewBadgerDbInMemory()
			s.Require().NoError(err)
			return db
		}
	}
	s.Db = s.DbInit()

	if cfg.L1Fetcher == nil {
		cfg.L1Fetcher = GetDummyL1Fetcher()
	}

	var serviceInterop chan nilservice.ServiceInterop
	if cfg.RunMode == nilservice.CollatorsOnlyRunMode {
		serviceInterop = make(chan nilservice.ServiceInterop, 1)
	}

	if cfg.ValidatorKeysPath == "" {
		keysPath := s.T().TempDir() + "/validator-keys.yaml"
		km := keys.NewValidatorKeyManager(keysPath)
		s.Require().NotNil(km)
		s.Require().NoError(km.InitKey())
		cfg.ValidatorKeysPath = keysPath
	}

	PatchConfigWithTestDefaults(cfg)

	s.Wg.Add(1)
	go func() {
		defer s.Wg.Done()
		rc := nilservice.Run(s.Context, cfg, s.Db, serviceInterop)
		s.Require().Zero(rc)
	}()

	if cfg.RunMode == nilservice.CollatorsOnlyRunMode {
		service := <-serviceInterop
		c, err := client.NewEthClient(s.Context, s.Db, types.ShardId(s.ShardsNum), service.TxnPools, zerolog.New(os.Stderr))
		s.Require().NoError(err)
		s.Client = c
	} else {
		s.Endpoint = strings.Replace(cfg.HttpUrl, "tcp://", "http://", 1)
		s.Client = rpc_client.NewClient(s.Endpoint, zerolog.New(os.Stderr))
	}

	if cfg.RunMode == nilservice.BlockReplayRunMode {
		s.Require().Eventually(func() bool {
			block, err := s.Client.GetBlock(s.Context, cfg.Replay.ShardId, "latest", false)
			return err == nil && block != nil
		}, BlockWaitTimeout, BlockPollInterval)
	} else {
		s.waitZerostate()
	}
}

func (s *RpcSuite) Cancel() {
	s.T().Helper()

	s.CtxCancel()
	s.Wg.Wait()
	s.Db.Close()
}

func (s *RpcSuite) WaitForReceipt(hash common.Hash) *jsonrpc.RPCReceipt {
	s.T().Helper()

	return WaitForReceipt(s.T(), s.Context, s.Client, hash)
}

func (s *RpcSuite) WaitIncludedInMain(hash common.Hash) *jsonrpc.RPCReceipt {
	s.T().Helper()

	return WaitIncludedInMain(s.T(), s.Context, s.Client, hash)
}

func (s *RpcSuite) waitZerostate() {
	s.T().Helper()

	s.waitZerostateFunc(func(i uint32) bool {
		block, err := s.Client.GetBlock(s.Context, types.ShardId(i), transport.BlockNumber(0), false)
		return err == nil && block != nil
	})
}

func (s *RpcSuite) waitZerostateFunc(fun func(i uint32) bool) {
	s.T().Helper()

	for i := range s.ShardsNum {
		s.Require().Eventually(func() bool { return fun(i) }, BlockWaitTimeout, BlockPollInterval)
	}
}

func (s *RpcSuite) DeployContractViaMainSmartAccount(shardId types.ShardId, payload types.DeployPayload, initialAmount types.Value) (types.Address, *jsonrpc.RPCReceipt) {
	s.T().Helper()

	return DeployContractViaSmartAccount(s.T(), s.Context, s.Client, types.MainSmartAccountAddress, execution.MainPrivateKey, shardId, payload, initialAmount)
}

func (s *RpcSuite) SendTransactionViaSmartAccount(addrFrom types.Address, addrTo types.Address, key *ecdsa.PrivateKey,
	calldata []byte,
) *jsonrpc.RPCReceipt {
	s.T().Helper()

	txHash, err := s.Client.SendTransactionViaSmartAccount(s.Context, addrFrom, calldata, types.NewFeePackFromGas(1_000_000), types.NewZeroValue(),
		[]types.TokenBalance{}, addrTo, key)
	s.Require().NoError(err)

	receipt := s.WaitIncludedInMain(txHash)
	s.Require().True(receipt.Success)
	s.Require().Equal("Success", receipt.Status)
	s.Require().Len(receipt.OutReceipts, 1)

	return receipt
}

func (s *RpcSuite) SendExternalTransaction(bytecode types.Code, contractAddress types.Address) *jsonrpc.RPCReceipt {
	s.T().Helper()

	receipt := s.SendExternalTransactionNoCheck(bytecode, contractAddress)

	s.Require().True(receipt.Success)
	s.Require().Equal("Success", receipt.Status)

	return receipt
}

func (s *RpcSuite) SendExternalTransactionNoCheck(bytecode types.Code, contractAddress types.Address) *jsonrpc.RPCReceipt {
	s.T().Helper()
	return SendExternalTransactionNoCheck(s.T(), s.Context, s.Client, bytecode, contractAddress)
}

// SendTransactionViaSmartAccountNoCheck sends a transaction via a smart account contract. Doesn't require the receipt be successful.
func (s *RpcSuite) SendTransactionViaSmartAccountNoCheck(addrSmartAccount types.Address, addrTo types.Address, key *ecdsa.PrivateKey,
	calldata []byte, fee types.FeePack, value types.Value, tokens []types.TokenBalance,
) *jsonrpc.RPCReceipt {
	s.T().Helper()

	// Send the raw transaction
	txHash, err := s.Client.SendTransactionViaSmartAccount(s.Context, addrSmartAccount, calldata, fee, value, tokens, addrTo, key)
	s.Require().NoError(err)

	receipt := s.WaitIncludedInMain(txHash)
	// We don't check the receipt for success here, as it can be failed on purpose
	if receipt.Success {
		// But if it is successful, we expect exactly one out receipt
		s.Require().Len(receipt.OutReceipts, 1)
	} else {
		s.Require().NotEqual("Success", receipt.Status)
	}

	return receipt
}

func (s *RpcSuite) SendRawTransaction(data []byte) *jsonrpc.RPCReceipt {
	txHash, err := s.Client.SendRawTransaction(s.Context, data)
	s.Require().NoError(err)
	receipt := s.WaitIncludedInMain(txHash)
	return receipt
}

func (s *RpcSuite) CallGetter(addr types.Address, calldata []byte, blockId any, overrides *jsonrpc.StateOverrides) []byte {
	s.T().Helper()
	return CallGetter(s.T(), s.Context, s.Client, addr, calldata, blockId, overrides)
}

func (s *RpcSuite) LoadContract(path string, name string) (types.Code, abi.ABI) {
	s.T().Helper()
	return LoadContract(s.T(), path, name)
}

func (s *RpcSuite) GetBalance(address types.Address) types.Value {
	s.T().Helper()
	return GetBalance(s.T(), s.Context, s.Client, address)
}

func (s *RpcSuite) AbiPack(abi *abi.ABI, name string, args ...any) []byte {
	s.T().Helper()
	return AbiPack(s.T(), abi, name, args...)
}

func (s *RpcSuite) AbiUnpack(abi *abi.ABI, name string, data []byte) []interface{} {
	s.T().Helper()
	res, err := abi.Unpack(name, data)
	s.Require().NoError(err)
	return res
}

func (s *RpcSuite) PrepareDefaultDeployPayload(abi abi.ABI, code []byte, args ...any) types.DeployPayload {
	s.T().Helper()
	return PrepareDefaultDeployPayload(s.T(), abi, code, args...)
}

func (s *RpcSuite) GasToValue(gas uint64) types.Value {
	return GasToValue(gas)
}

type ReceiptInfo map[types.Address]*ContractInfo

type ContractInfo struct {
	Name            string
	OutTransactions map[types.Address]*jsonrpc.RPCInTransaction

	ValueUsed      types.Value
	ValueForwarded types.Value

	ValueReceived  types.Value
	RefundReceived types.Value
	BounceReceived types.Value

	ValueSent  types.Value
	RefundSent types.Value
	BounceSent types.Value

	NumSuccess int
	NumFail    int
}

func (r *ReceiptInfo) Dump() {
	data, err := json.MarshalIndent(r, "", "  ")
	check.PanicIfErr(err)
	fmt.Printf("info: %s\n", string(data))
}

// AllSuccess checks if the contract info contains only successful receipts.
func (r *ReceiptInfo) AllSuccess() bool {
	for _, info := range *r {
		if !info.IsSuccess() {
			return false
		}
	}
	return true
}

// ContainsOnly checks that the receipt info contains only the specified addresses.
func (r *ReceiptInfo) ContainsOnly(addresses ...types.Address) bool {
	if len(*r) != len(addresses) {
		return false
	}
	for _, addr := range addresses {
		if _, found := (*r)[addr]; !found {
			return false
		}
	}
	return true
}

func (c *ContractInfo) IsSuccess() bool {
	return c.NumFail == 0 && c.NumSuccess > 0
}

// GetValueSpent returns the total value spent by the contract.
func (c *ContractInfo) GetValueSpent() types.Value {
	return c.ValueSent.Sub(c.RefundReceived).Sub(c.BounceReceived)
}

func (s *RpcSuite) GetRandomBytes(size int) []byte {
	s.T().Helper()
	data := make([]byte, size)
	_, err := rand.Read(data)
	s.Require().NoError(err)
	return data
}

func (s *RpcSuite) AnalyzeReceipt(receipt *jsonrpc.RPCReceipt, namesMap map[types.Address]string) ReceiptInfo {
	s.T().Helper()
	return AnalyzeReceipt(s.T(), s.Context, s.Client, receipt, namesMap)
}

func getContractInfo(addr types.Address, valuesMap ReceiptInfo) *ContractInfo {
	value, found := valuesMap[addr]
	if !found {
		value = &ContractInfo{}
		value.OutTransactions = make(map[types.Address]*jsonrpc.RPCInTransaction)
		valuesMap[addr] = value
	}
	return value
}
