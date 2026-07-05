package main

import (
	"agnet-project-demo/backend/internal/config"
	chat "agnet-project-demo/backend/kitex_gen/chat/agentchatservice"
	"log"
	"net"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
)

func main() {
	cfg := config.Load()

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
