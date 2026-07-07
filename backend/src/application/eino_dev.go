package application

import (
	"context"
	"fmt"
	"log"
	"os"

	"agnet-project-demo/backend/src/agent/tools/imsummary"

	"github.com/cloudwego/eino-ext/devops"
)

// InitEinoDev 初始化 Eino DevOps 服务，供本地查看 Graph 和类型结构。
func InitEinoDev(ctx context.Context) {
	ip := getEnv("EINO_DEV_SERVER_IP", "127.0.0.1")
	port := getEnv("EINO_DEV_SERVER_PORT", "52538")

	// 注册 IM 总结相关类型后，Eino Dev 可以识别 Graph 输入输出结构。
	if err := devops.Init(ctx,
		devops.WithDevServerIP(ip),
		devops.WithDevServerPort(port),
		devops.AppendType(&imsummary.Request{}),
		devops.AppendType(&imsummary.Summary{}),
		devops.AppendType(&imsummary.SummaryCard{}),
		devops.AppendType(&imsummary.ConversationSummaryGroups{}),
	); err != nil {
		log.Printf("init eino dev failed: %v", err)
		return
	}

	if os.Getenv("EINO_DEV_QUIET") != "1" {
		log.Printf("Eino DevOps server: http://%s/eino/devops", fmt.Sprintf("%s:%s", ip, port))
	}
}

// getEnv 读取环境变量；变量为空时使用指定默认值。
func getEnv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
