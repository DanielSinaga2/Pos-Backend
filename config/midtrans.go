package config

import (
	"encoding/base64"
	"os"
	"strconv"

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

// MidtransConfig holds Midtrans credentials and base URL config
type MidtransConfig struct {
	ServerKey    string
	ClientKey    string
	IsProduction bool
	BaseURL      string
}

// LoadMidtransConfig loads Midtrans configuration from environment variables
func LoadMidtransConfig() *MidtransConfig {
	serverKey := os.Getenv("MIDTRANS_SERVER_KEY")
	clientKey := os.Getenv("MIDTRANS_CLIENT_KEY")
	isProd, _ := strconv.ParseBool(os.Getenv("MIDTRANS_IS_PRODUCTION"))

	baseURL := "https://api.sandbox.midtrans.com"
	if isProd {
		baseURL = "https://api.midtrans.com"
	}

	return &MidtransConfig{
		ServerKey:    serverKey,
		ClientKey:    clientKey,
		IsProduction: isProd,
		BaseURL:      baseURL,
	}
}

// GetBasicAuthHeader returns the Authorization header for Midtrans API
func (c *MidtransConfig) GetBasicAuthHeader() string {
	auth := c.ServerKey + ":"
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
}
