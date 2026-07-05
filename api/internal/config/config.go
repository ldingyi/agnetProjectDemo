package config

import "os"

const (
	defaultHTTPAddr          = ":8080"
	defaultBackendService    = "agents.chat.rpc"
	defaultBackendTargetAddr = "127.0.0.1:8888"
)

type Config struct {
	HTTPAddr          string
	BackendService    string
	BackendTargetAddr string
}

func Load() Config {
	return Config{
		HTTPAddr:          getEnv("API_HTTP_ADDR", defaultHTTPAddr),
		BackendService:    getEnv("BACKEND_SERVICE_NAME", defaultBackendService),
		BackendTargetAddr: getEnv("BACKEND_TARGET_ADDR", defaultBackendTargetAddr),
	}
}

func getEnv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
