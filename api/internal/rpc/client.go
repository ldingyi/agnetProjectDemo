package rpc

import (
	"fmt"

	"agnet-project-demo/api/internal/config"
	"agnet-project-demo/backend/kitex_gen/chat/agentchatservice"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/client/streamclient"
)

type Client struct {
	chat       agentchatservice.Client
	chatStream agentchatservice.StreamClient
}

func NewClient(cfg config.Config) (*Client, error) {
	chatClient, err := agentchatservice.NewClient(
		cfg.BackendService,
		client.WithHostPorts(cfg.BackendTargetAddr),
	)
	if err != nil {
		return nil, fmt.Errorf("new chat client: %w", err)
	}

	streamClient, err := agentchatservice.NewStreamClient(
		cfg.BackendService,
		streamclient.WithHostPorts(cfg.BackendTargetAddr),
	)
	if err != nil {
		return nil, fmt.Errorf("new chat stream client: %w", err)
	}

	return &Client{
		chat:       chatClient,
		chatStream: streamClient,
	}, nil
}

func (c *Client) Chat() agentchatservice.Client {
	return c.chat
}

func (c *Client) ChatStream() agentchatservice.StreamClient {
	return c.chatStream
}
