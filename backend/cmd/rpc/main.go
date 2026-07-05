package main

import (
	"agnet-project-demo/backend/src/application"
	"agnet-project-demo/backend/src/infrastructure"
	"context"
	"log"
	"net"

	"agnet-project-demo/backend/internal/config"
	"agnet-project-demo/backend/kitex_gen/chat/agentchatservice"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	addr, err := net.ResolveTCPAddr("tcp", cfg.RPCAddr)
	if err != nil {
		log.Fatalf("resolve rpc addr: %v", err)
	}

	app, err := application.NewAgentChatService(ctx, infrastructure.LoadConfig())
	if err != nil {
		log.Fatalf("init agents chat service: %v", err)
	}

	svr := agentchatservice.NewServer(
		app,
		server.WithServiceAddr(addr),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: cfg.ServiceName}),
	)

	if err := svr.Run(); err != nil {
		log.Fatalf("run rpc server: %v", err)
	}
}
