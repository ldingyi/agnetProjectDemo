package infrastructure

import (
	"os"
)

const (
	defaultDeepSeekAPIKey  = "sk-65922688938949be82484348b65e717f"
	defaultDeepSeekBaseURL = "https://api.deepseek.com"
	defaultDeepSeekModel   = "deepseek-chat"
	defaultSessionDir      = "data/sessions"
	defaultTracePath       = "data/traces/model_trace.jsonl"
	defaultThirdService    = "third.business.rpc"
	defaultThirdTargetAddr = "127.0.0.1:8890"
)

type Config struct {
	DeepSeek DeepSeekConfig
	Session  SessionConfig
	Trace    TraceConfig
	Third    ThirdConfig
}

type DeepSeekConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

type SessionConfig struct {
	Dir string
}

type TraceConfig struct {
	Path string
}

type ThirdConfig struct {
	ServiceName string
	TargetAddr  string
}

func LoadConfig() Config {
	return Config{
		DeepSeek: DeepSeekConfig{
			APIKey:  getEnv("DEEPSEEK_API_KEY", defaultDeepSeekAPIKey),
			BaseURL: getEnv("DEEPSEEK_BASE_URL", defaultDeepSeekBaseURL),
			Model:   getEnv("DEEPSEEK_MODEL", defaultDeepSeekModel),
		},
		Session: SessionConfig{
			Dir: getEnv("SESSION_DIR", defaultSessionDir),
		},
		Trace: TraceConfig{
			Path: getEnv("MODEL_TRACE_DIR", getEnv("MODEL_TRACE_PATH", defaultTracePath)),
		},
		Third: ThirdConfig{
			ServiceName: getEnv("THIRD_SERVICE_NAME", defaultThirdService),
			TargetAddr:  getEnv("THIRD_TARGET_ADDR", defaultThirdTargetAddr),
		},
	}
}

func getEnv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
