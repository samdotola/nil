package nil_load_generator

import (
	"github.com/NilFoundation/nil/nil/internal/types"
)

type NilLoadGeneratorAPI interface {
	HealthCheck() bool
	SmartAccountsAddr() []types.Address
}

type NilLoadGeneratorAPIImpl struct{}

var _ NilLoadGeneratorAPI = (*NilLoadGeneratorAPIImpl)(nil)

func NewNilLoadGeneratorAPI() *NilLoadGeneratorAPIImpl {
	return &NilLoadGeneratorAPIImpl{}
}

func (c NilLoadGeneratorAPIImpl) HealthCheck() bool {
	return isInitialized.Load()
}

func (c NilLoadGeneratorAPIImpl) SmartAccountsAddr() []types.Address {
	if !isInitialized.Load() {
		return nil
	}

	smartAccountsAddr := make([]types.Address, len(smartAccounts))
	for i, smartAccount := range smartAccounts {
		smartAccountsAddr[i] = smartAccount.Addr
	}
	return smartAccountsAddr
}
