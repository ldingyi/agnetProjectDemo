package rpc

import (
	"fmt"

	"agnet-project-demo/backend/src/infrastructure"
	"agnet-project-demo/third/kitex_gen/third/thirdbusinessservice"

	"github.com/cloudwego/kitex/client"
)

// NewThirdClient 根据配置创建第三方业务 RPC 客户端。
func NewThirdClient(cfg infrastructure.Config) (thirdbusinessservice.Client, error) {
	// mock third 服务没有服务发现，这里显式指定本地地址，便于 demo 独立运行。
	thirdClient, err := thirdbusinessservice.NewClient(
		cfg.Third.ServiceName,
		client.WithHostPorts(cfg.Third.TargetAddr),
	)
	if err != nil {
		return nil, fmt.Errorf("new third rpc client: %w", err)
	}
	return thirdClient, nil
}
