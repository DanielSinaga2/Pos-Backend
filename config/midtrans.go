package config

import (
	"github.com/midtrans/midtrans-go"
	"github.com/midtrans/midtrans-go/coreapi"
)

// NewMidtransCoreClient creates an isolated Core API client without changing
// the SDK's package-level configuration.
func NewMidtransCoreClient(cfg *Config) *coreapi.Client {
	environment := midtrans.Sandbox
	if cfg.MidtransIsProduction {
		environment = midtrans.Production
	}

	client := &coreapi.Client{}
	client.New(cfg.MidtransServerKey, environment)
	client.ClientKey = cfg.MidtransClientKey
	return client
}
