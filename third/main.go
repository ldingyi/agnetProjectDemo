package main

import (
	third "agnet-project-demo/third/kitex_gen/third/thirdbusinessservice"
	"agnet-project-demo/third/src/infrastructure"
	"log"
	"net"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
)

func main() {
	cfg := infrastructure.LoadConfig()

	addr, err := net.ResolveTCPAddr("tcp", cfg.RPCAddr)
	if err != nil {
		log.Fatalf("resolve rpc addr: %v", err)
	}

	svr := third.NewServer(
		newThirdBusinessServiceImpl(cfg),
		server.WithServiceAddr(addr),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: cfg.ServiceName}),
	)

	err = svr.Run()

	if err != nil {
		log.Println(err.Error())
	}
}
