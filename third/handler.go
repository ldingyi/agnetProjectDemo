package main

import (
	third "agnet-project-demo/third/kitex_gen/third"
	"agnet-project-demo/third/src/application"
	"agnet-project-demo/third/src/infrastructure"
	"context"
	"log"
)

// ThirdBusinessServiceImpl implements the last service interface defined in the IDL.
type ThirdBusinessServiceImpl struct {
	inner *application.ThirdBusinessService
}

func newThirdBusinessServiceImpl(cfg infrastructure.Config) *ThirdBusinessServiceImpl {
	inner, err := application.NewThirdBusinessService(cfg)
	if err != nil {
		log.Fatalf("init third business service: %v", err)
	}
	return &ThirdBusinessServiceImpl{inner: inner}
}

// GetUserByName implements the ThirdBusinessServiceImpl interface.
func (s *ThirdBusinessServiceImpl) GetUserByName(ctx context.Context, req *third.GetUserByNameRequest) (resp *third.GetUserByNameResponse, err error) {
	return s.inner.GetUserByName(ctx, req)
}

// RecommendProducts implements the ThirdBusinessServiceImpl interface.
func (s *ThirdBusinessServiceImpl) RecommendProducts(ctx context.Context, req *third.RecommendProductsRequest) (resp *third.RecommendProductsResponse, err error) {
	return s.inner.RecommendProducts(ctx, req)
}

// CheckFreeSample implements the ThirdBusinessServiceImpl interface.
func (s *ThirdBusinessServiceImpl) CheckFreeSample(ctx context.Context, req *third.CheckFreeSampleRequest) (resp *third.CheckFreeSampleResponse, err error) {
	return s.inner.CheckFreeSample(ctx, req)
}

// AddSelectionCart implements the ThirdBusinessServiceImpl interface.
func (s *ThirdBusinessServiceImpl) AddSelectionCart(ctx context.Context, req *third.AddSelectionCartRequest) (resp *third.AddSelectionCartResponse, err error) {
	return s.inner.AddSelectionCart(ctx, req)
}

// ApplyFreeSample implements the ThirdBusinessServiceImpl interface.
func (s *ThirdBusinessServiceImpl) ApplyFreeSample(ctx context.Context, req *third.ApplyFreeSampleRequest) (resp *third.ApplyFreeSampleResponse, err error) {
	return s.inner.ApplyFreeSample(ctx, req)
}

// BuySample implements the ThirdBusinessServiceImpl interface.
func (s *ThirdBusinessServiceImpl) BuySample(ctx context.Context, req *third.BuySampleRequest) (resp *third.BuySampleResponse, err error) {
	return s.inner.BuySample(ctx, req)
}

// GetFulfillmentStatus implements the ThirdBusinessServiceImpl interface.
func (s *ThirdBusinessServiceImpl) GetFulfillmentStatus(ctx context.Context, req *third.GetFulfillmentStatusRequest) (resp *third.GetFulfillmentStatusResponse, err error) {
	return s.inner.GetFulfillmentStatus(ctx, req)
}

// ListIMConversations implements the ThirdBusinessServiceImpl interface.
func (s *ThirdBusinessServiceImpl) ListIMConversations(ctx context.Context, req *third.ListIMConversationsRequest) (resp *third.ListIMConversationsResponse, err error) {
	return s.inner.ListIMConversations(ctx, req)
}

// GetIMConversationMessages implements the ThirdBusinessServiceImpl interface.
func (s *ThirdBusinessServiceImpl) GetIMConversationMessages(ctx context.Context, req *third.GetIMConversationMessagesRequest) (resp *third.GetIMConversationMessagesResponse, err error) {
	return s.inner.GetIMConversationMessages(ctx, req)
}

// SendIMMessage implements the ThirdBusinessServiceImpl interface.
func (s *ThirdBusinessServiceImpl) SendIMMessage(ctx context.Context, req *third.SendIMMessageRequest) (resp *third.SendIMMessageResponse, err error) {
	return s.inner.SendIMMessage(ctx, req)
}

// GetUserByID implements the ThirdBusinessServiceImpl interface.
func (s *ThirdBusinessServiceImpl) GetUserByID(ctx context.Context, req *third.GetUserByIDRequest) (resp *third.GetUserByIDResponse, err error) {
	return s.inner.GetUserByID(ctx, req)
}

// GetSelectionCart implements the ThirdBusinessServiceImpl interface.
func (s *ThirdBusinessServiceImpl) GetSelectionCart(ctx context.Context, req *third.GetSelectionCartRequest) (resp *third.GetSelectionCartResponse, err error) {
	return s.inner.GetSelectionCart(ctx, req)
}
