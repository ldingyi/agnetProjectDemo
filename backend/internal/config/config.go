package config

import "os"

const (
	defaultServiceName = "agents.chat.rpc"
	defaultRPCAddr     = ":8888"
)

type Config struct {
	ServiceName string
	RPCAddr     string
}

func Load() Config {
	return Config{
		ServiceName: getEnv("BACKEND_SERVICE_NAME", defaultServiceName),
		RPCAddr:     getEnv("BACKEND_RPC_ADDR", defaultRPCAddr),
	}
}

func getEnv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
