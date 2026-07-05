package main

import (
	"log"

	"agnet-project-demo/api/internal/config"
	"agnet-project-demo/api/internal/handler"
	"agnet-project-demo/api/internal/rpc"

	"github.com/cloudwego/hertz/pkg/app/server"
)

func main() {
	cfg := config.Load()

	rpcClient, err := rpc.NewClient(cfg)
	if err != nil {
		log.Fatalf("init rpc client: %v", err)
	}

	h := server.Default(server.WithHostPorts(cfg.HTTPAddr))
	h.GET("/health", handler.Health)
	handler.RegisterRoutes(h, handler.NewChatHandler(rpcClient))

	h.Spin()
}
