package infrastructure

import "os"

const (
	defaultServiceName     = "third.business.rpc"
	defaultRPCAddr         = ":8890"
	defaultMockSeedPath    = "data/mock/mock_data.json"
	defaultMockRuntimePath = "data/runtime/mock_data.json"
)

type Config struct {
	ServiceName     string
	RPCAddr         string
	MockSeedPath    string
	MockRuntimePath string
}

func LoadConfig() Config {
	return Config{
		ServiceName:     getEnv("THIRD_SERVICE_NAME", defaultServiceName),
		RPCAddr:         getEnv("THIRD_RPC_ADDR", defaultRPCAddr),
		MockSeedPath:    getEnv("THIRD_MOCK_SEED_PATH", defaultMockSeedPath),
		MockRuntimePath: getEnv("THIRD_MOCK_RUNTIME_PATH", defaultMockRuntimePath),
	}
}

func getEnv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
