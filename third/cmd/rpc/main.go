package main

import (
	"log"
	"net"

	"agnet-project-demo/third/kitex_gen/third/thirdbusinessservice"
	"agnet-project-demo/third/src/application"
	"agnet-project-demo/third/src/infrastructure"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
)

func main() {
	cfg := infrastructure.LoadConfig()

	addr, err := net.ResolveTCPAddr("tcp", cfg.RPCAddr)
	if err != nil {
		log.Fatalf("resolve rpc addr: %v", err)
	}

	app, err := application.NewThirdBusinessService(cfg)
	if err != nil {
		log.Fatalf("init third business service: %v", err)
	}

	svr := thirdbusinessservice.NewServer(
		app,
		server.WithServiceAddr(addr),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: cfg.ServiceName}),
	)

	if err := svr.Run(); err != nil {
		log.Fatalf("run rpc server: %v", err)
	}
}
