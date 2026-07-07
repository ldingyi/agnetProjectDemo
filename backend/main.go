package main

import (
	"agnet-project-demo/backend/internal/config"
	chat "agnet-project-demo/backend/kitex_gen/chat/agentchatservice"
	"agnet-project-demo/backend/src/application"
	"context"
	"log"
	"net"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()
	application.InitEinoDev(ctx)

	// Kitex 服务只承载 AgentChatService，HTTP/SSE 转发由 api 模块负责。
	addr, err := net.ResolveTCPAddr("tcp", cfg.RPCAddr)
	if err != nil {
		log.Fatalf("resolve rpc addr: %v", err)
	}

	svr := chat.NewServer(
		newAgentChatServiceImpl(),
		server.WithServiceAddr(addr),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: cfg.ServiceName}),
	)

	err = svr.Run()

	if err != nil {
		log.Println(err.Error())
	}
}
