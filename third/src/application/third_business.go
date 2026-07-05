package application

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"agnet-project-demo/third/kitex_gen/third"
	"agnet-project-demo/third/src/infrastructure"
)

type ThirdBusinessService struct {
	store *mockStore
}

func NewThirdBusinessService(cfg infrastructure.Config) (*ThirdBusinessService, error) {
	store, err := newMockStore(cfg.MockSeedPath, cfg.MockRuntimePath)
	if err != nil {
		return nil, err
	}
	return &ThirdBusinessService{store: store}, nil
}

func (s *ThirdBusinessService) GetUserByName(ctx context.Context, req *third.GetUserByNameRequest) (*third.GetUserByNameResponse, error) {
	data, err := s.store.load()
	if err != nil {
		return nil, err
	}

	for _, user := range data.Users {
		if user.Name == req.GetName() {
			return &third.GetUserByNameResponse{
				Found: true,
				User: &third.User{
					UserId:       user.UserID,
					Name:         user.Name,
					Level:        user.Level,
					FansCount:    user.FansCount,
					MainCategory: user.MainCategory,
				},
			}, nil
		}
	}

	return &third.GetUserByNameResponse{Found: false}, nil
}

func (s *ThirdBusinessService) GetUserByID(ctx context.Context, req *third.GetUserByIDRequest) (*third.GetUserByIDResponse, error) {
	data, err := s.store.load()
	if err != nil {
		return nil, err
	}

	for _, user := range data.Users {
		if user.UserID == req.GetUserId() {
			return &third.GetUserByIDResponse{
				Found: true,
				User: &third.User{
					UserId:       user.UserID,
					Name:         user.Name,
					Level:        user.Level,
					FansCount:    user.FansCount,
					MainCategory: user.MainCategory,
				},
			}, nil
		}
	}

	return &third.GetUserByIDResponse{Found: false}, nil
}

func (s *ThirdBusinessService) ListIMConversations(ctx context.Context, req *third.ListIMConversationsRequest) (*third.ListIMConversationsResponse, error) {
	data, err := s.store.load()
	if err != nil {
		return nil, err
	}

	conversationIDs := make([]string, 0)
	for _, conversation := range data.IMConversations {
		if conversation.UserID == req.GetUserId() {
			conversationIDs = append(conversationIDs, conversation.ConversationID)
		}
	}

	return &third.ListIMConversationsResponse{
		ConversationIds: conversationIDs,
	}, nil
}

func (s *ThirdBusinessService) GetIMConversationMessages(ctx context.Context, req *third.GetIMConversationMessagesRequest) (*third.GetIMConversationMessagesResponse, error) {
	data, err := s.store.load()
	if err != nil {
		return nil, err
	}

	for _, conversation := range data.IMConversations {
		if conversation.UserID == req.GetUserId() && conversation.ConversationID == req.GetConversationId() {
			return &third.GetIMConversationMessagesResponse{
				Messages: toThirdMessages(conversation.Messages),
			}, nil
		}
	}

	return &third.GetIMConversationMessagesResponse{
		Messages: []*third.IMMessage{},
	}, nil
}

func (s *ThirdBusinessService) SendIMMessage(ctx context.Context, req *third.SendIMMessageRequest) (*third.SendIMMessageResponse, error) {
	msg := mockIMMessage{
		MessageID:   fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		Sender:      "USER",
		MessageType: "TEXT",
		Content:     req.GetContent(),
		SendTime:    time.Now().Format(time.RFC3339),
	}

	if err := s.store.appendMessage(req.GetUserId(), req.GetConversationId(), msg); err != nil {
		return nil, err
	}

	return &third.SendIMMessageResponse{
		Message: toThirdMessage(msg),
	}, nil
}

func (s *ThirdBusinessService) RecommendProducts(ctx context.Context, req *third.RecommendProductsRequest) (*third.RecommendProductsResponse, error) {
	data, err := s.store.load()
	if err != nil {
		return nil, err
	}

	productIDs := data.Recommendations[req.GetUserId()]
	products := make([]*third.Product, 0, len(productIDs))
	for _, productID := range productIDs {
		if product, ok := data.productByID(productID); ok {
			products = append(products, toThirdProduct(product))
		}
	}

	return &third.RecommendProductsResponse{
		Products: products,
	}, nil
}

func (s *ThirdBusinessService) CheckFreeSample(ctx context.Context, req *third.CheckFreeSampleRequest) (*third.CheckFreeSampleResponse, error) {
	data, err := s.store.load()
	if err != nil {
		return nil, err
	}

	_, reason := data.checkFreeSampleEligibility(req.GetUserId(), req.GetProductId())
	return &third.CheckFreeSampleResponse{Available: reason == ""}, nil
}

func (s *ThirdBusinessService) AddSelectionCart(ctx context.Context, req *third.AddSelectionCartRequest) (*third.AddSelectionCartResponse, error) {
	result, err := s.store.applyAction(req.GetUserId(), req.GetProductIds(), actionTypeSelectionCart)
	if err != nil {
		return nil, err
	}

	return &third.AddSelectionCartResponse{
		SuccessProductIds: result.SuccessProductIDs,
		Failures:          toThirdFailures(result.Failures),
	}, nil
}

func (s *ThirdBusinessService) GetSelectionCart(ctx context.Context, req *third.GetSelectionCartRequest) (*third.GetSelectionCartResponse, error) {
	data, err := s.store.load()
	if err != nil {
		return nil, err
	}

	productIDs := data.UserSelectionCarts[req.GetUserId()]
	result := make([]string, len(productIDs))
	copy(result, productIDs)
	return &third.GetSelectionCartResponse{ProductIds: result}, nil
}

func (s *ThirdBusinessService) ApplyFreeSample(ctx context.Context, req *third.ApplyFreeSampleRequest) (*third.ApplyFreeSampleResponse, error) {
	result, err := s.store.applyAction(req.GetUserId(), req.GetProductIds(), actionTypeFreeSample)
	if err != nil {
		return nil, err
	}

	return &third.ApplyFreeSampleResponse{
		SuccessProductIds: result.SuccessProductIDs,
		Failures:          toThirdFailures(result.Failures),
	}, nil
}

func (s *ThirdBusinessService) BuySample(ctx context.Context, req *third.BuySampleRequest) (*third.BuySampleResponse, error) {
	result, err := s.store.applyAction(req.GetUserId(), req.GetProductIds(), actionTypeBuySample)
	if err != nil {
		return nil, err
	}

	return &third.BuySampleResponse{
		SuccessProductIds: result.SuccessProductIDs,
		Failures:          toThirdFailures(result.Failures),
	}, nil
}

func (s *ThirdBusinessService) GetFulfillmentStatus(ctx context.Context, req *third.GetFulfillmentStatusRequest) (*third.GetFulfillmentStatusResponse, error) {
	data, err := s.store.load()
	if err != nil {
		return nil, err
	}

	productIDs := req.GetProductIds()
	if len(productIDs) == 0 {
		productIDs = data.userRelatedProductIDs(req.GetUserId())
	}
	statuses := make([]*third.ProductFulfillmentStatus, 0, len(productIDs))
	for _, productID := range productIDs {
		statuses = append(statuses, data.deriveFulfillmentStatus(req.GetUserId(), productID))
	}

	return &third.GetFulfillmentStatusResponse{
		Statuses: statuses,
	}, nil
}

type mockStore struct {
	seedPath    string
	runtimePath string
	mu          sync.Mutex
}

type actionType string

const (
	actionTypeSelectionCart actionType = "selection_cart"
	actionTypeFreeSample    actionType = "free_sample"
	actionTypeBuySample     actionType = "buy_sample"
)

var levelRank = map[string]int{
	"new":    1,
	"silver": 2,
	"gold":   3,
}

type mockData struct {
	Users                  []mockUser                         `json:"users"`
	Products               []mockProduct                      `json:"products"`
	Recommendations        map[string][]string                `json:"recommendations"`
	UserSelectionCarts     map[string][]string                `json:"user_selection_carts"`
	FreeSampleApplications map[string][]mockSampleApplication `json:"free_sample_applications"`
	BuySampleOrders        map[string][]mockSampleOrder       `json:"buy_sample_orders"`
	ProductRules           map[string]mockProductRule         `json:"product_rules"`
	IMConversations        []mockIMConversation               `json:"im_conversations"`
}

type mockUser struct {
	UserID       string `json:"user_id"`
	Name         string `json:"name"`
	Level        string `json:"level"`
	FansCount    int64  `json:"fans_count"`
	MainCategory string `json:"main_category"`
}

type mockProduct struct {
	ProductID  string  `json:"product_id"`
	Name       string  `json:"name"`
	Commission float64 `json:"commission"`
	Category   string  `json:"category"`
	Price      float64 `json:"price"`
	Stock      int64   `json:"stock"`
}

type mockActionResult struct {
	SuccessProductIDs []string            `json:"success_product_ids"`
	Failures          []mockFailureReason `json:"failures"`
}

type mockFailureReason struct {
	ProductID string `json:"product_id"`
	Reason    string `json:"reason"`
}

type mockSampleApplication struct {
	ProductID string `json:"product_id"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
}

type mockSampleOrder struct {
	ProductID string `json:"product_id"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
}

type mockProductRule struct {
	SelectionCartEnabled              bool   `json:"selection_cart_enabled"`
	MinSelectionLevel                 string `json:"min_selection_level"`
	RequireCategoryMatchForSelection  bool   `json:"require_category_match_for_selection"`
	FreeSampleEnabled                 bool   `json:"free_sample_enabled"`
	MinFreeSampleLevel                string `json:"min_free_sample_level"`
	RequireCategoryMatchForFreeSample bool   `json:"require_category_match_for_free_sample"`
	FreeSampleQuota                   int    `json:"free_sample_quota"`
	PaidSampleEnabled                 bool   `json:"paid_sample_enabled"`
	MinPaidSampleLevel                string `json:"min_paid_sample_level"`
}

type mockIMConversation struct {
	UserID         string          `json:"user_id"`
	ConversationID string          `json:"conversation_id"`
	MerchantID     string          `json:"merchant_id"`
	MerchantName   string          `json:"merchant_name"`
	Scenario       string          `json:"scenario"`
	Messages       []mockIMMessage `json:"messages"`
}

type mockIMMessage struct {
	MessageID      string                `json:"message_id"`
	Sender         string                `json:"sender"`
	MessageType    string                `json:"message_type"`
	Content        string                `json:"content"`
	SendTime       string                `json:"send_time"`
	InvitationCard *mockIMInvitationCard `json:"invitation_card,omitempty"`
	ContactCard    *mockIMContactInfo    `json:"contact_card,omitempty"`
}

type mockIMInvitationCard struct {
	Intro    string              `json:"intro"`
	Products []mockIMProductInfo `json:"products"`
}

type mockIMProductInfo struct {
	ProductID  string  `json:"product_id"`
	Name       string  `json:"name"`
	Commission float64 `json:"commission"`
}

type mockIMContactInfo struct {
	Name   string `json:"name"`
	Phone  string `json:"phone"`
	Wechat string `json:"wechat"`
}

func newMockStore(seedPath string, runtimePath string) (*mockStore, error) {
	store := &mockStore{
		seedPath:    seedPath,
		runtimePath: runtimePath,
	}
	if err := store.resetRuntimeData(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *mockStore) resetRuntimeData() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	content, err := os.ReadFile(s.seedPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.runtimePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.runtimePath, content, 0o644)
}

func (s *mockStore) load() (*mockData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *mockStore) appendMessage(userID string, conversationID string, msg mockIMMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadLocked()
	if err != nil {
		return err
	}

	for i := range data.IMConversations {
		conversation := &data.IMConversations[i]
		if conversation.UserID == userID && conversation.ConversationID == conversationID {
			conversation.Messages = append(conversation.Messages, msg)
			return s.saveLocked(data)
		}
	}

	return fmt.Errorf("conversation not found: user_id=%s conversation_id=%s", userID, conversationID)
}

func (s *mockStore) applyAction(userID string, productIDs []string, action actionType) (mockActionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadLocked()
	if err != nil {
		return mockActionResult{}, err
	}

	result := data.applyAction(userID, productIDs, action)

	if err := s.saveLocked(data); err != nil {
		return mockActionResult{}, err
	}
	return result, nil
}

func (s *mockStore) loadLocked() (*mockData, error) {
	content, err := os.ReadFile(s.runtimePath)
	if err != nil {
		return nil, err
	}

	var data mockData
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func (s *mockStore) saveLocked(data *mockData) error {
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.runtimePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.runtimePath, append(content, '\n'), 0o644)
}

func toThirdMessages(messages []mockIMMessage) []*third.IMMessage {
	result := make([]*third.IMMessage, 0, len(messages))
	for _, message := range messages {
		result = append(result, toThirdMessage(message))
	}
	return result
}

func toThirdMessage(message mockIMMessage) *third.IMMessage {
	return &third.IMMessage{
		MessageId:      message.MessageID,
		Sender:         toSender(message.Sender),
		MessageType:    toMessageType(message.MessageType),
		Content:        message.Content,
		SendTime:       message.SendTime,
		InvitationCard: toInvitationCard(message.InvitationCard),
		ContactCard:    toContactCard(message.ContactCard),
	}
}

func toSender(sender string) third.IMSenderType {
	switch sender {
	case "USER":
		return third.IMSenderType_USER
	case "MERCHANT":
		return third.IMSenderType_MERCHANT
	default:
		return third.IMSenderType_UNKNOWN
	}
}

func toMessageType(messageType string) third.IMMessageType {
	switch messageType {
	case "INVITATION_CARD":
		return third.IMMessageType_INVITATION_CARD
	case "CONTACT_CARD":
		return third.IMMessageType_CONTACT_CARD
	default:
		return third.IMMessageType_TEXT
	}
}

func toInvitationCard(card *mockIMInvitationCard) *third.IMInvitationCard {
	if card == nil {
		return nil
	}
	products := make([]*third.IMProductInfo, 0, len(card.Products))
	for _, product := range card.Products {
		products = append(products, &third.IMProductInfo{
			ProductId:  product.ProductID,
			Name:       product.Name,
			Commission: product.Commission,
		})
	}
	return &third.IMInvitationCard{
		Intro:    card.Intro,
		Products: products,
	}
}

func toContactCard(card *mockIMContactInfo) *third.IMContactInfo {
	if card == nil {
		return nil
	}
	return &third.IMContactInfo{
		Name:   card.Name,
		Phone:  card.Phone,
		Wechat: card.Wechat,
	}
}

func (d *mockData) productByID(productID string) (mockProduct, bool) {
	for _, product := range d.Products {
		if product.ProductID == productID {
			return product, true
		}
	}
	return mockProduct{}, false
}

func (d *mockData) userByID(userID string) (mockUser, bool) {
	for _, user := range d.Users {
		if user.UserID == userID {
			return user, true
		}
	}
	return mockUser{}, false
}

func (d *mockData) applyAction(userID string, productIDs []string, action actionType) mockActionResult {
	result := mockActionResult{
		SuccessProductIDs: make([]string, 0, len(productIDs)),
		Failures:          make([]mockFailureReason, 0),
	}
	for _, productID := range productIDs {
		if productID == "" {
			continue
		}
		if reason := d.checkAction(userID, productID, action); reason != "" {
			result.Failures = append(result.Failures, mockFailureReason{ProductID: productID, Reason: reason})
			continue
		}
		d.recordAction(userID, productID, action)
		result.SuccessProductIDs = append(result.SuccessProductIDs, productID)
	}
	return result
}

func (d *mockData) checkAction(userID string, productID string, action actionType) string {
	user, ok := d.userByID(userID)
	if !ok {
		return "用户不存在"
	}
	product, ok := d.productByID(productID)
	if !ok {
		return "商品不存在"
	}
	if product.Stock <= 0 {
		return "商品库存不足"
	}
	rule, ok := d.ProductRules[productID]
	if !ok {
		return "商品规则不存在"
	}

	switch action {
	case actionTypeSelectionCart:
		return d.checkSelectionCart(user, product, rule)
	case actionTypeFreeSample:
		return d.checkFreeSample(user, product, rule)
	case actionTypeBuySample:
		return d.checkBuySample(user, product, rule)
	default:
		return "未知操作"
	}
}

func (d *mockData) checkSelectionCart(user mockUser, product mockProduct, rule mockProductRule) string {
	if hasString(d.UserSelectionCarts[user.UserID], product.ProductID) {
		return "商品已在选品车中"
	}
	if !rule.SelectionCartEnabled {
		return "商品暂不支持加选"
	}
	if !levelAtLeast(user.Level, rule.MinSelectionLevel) {
		return fmt.Sprintf("用户等级不足，需要达到%s等级", rule.MinSelectionLevel)
	}
	if rule.RequireCategoryMatchForSelection && user.MainCategory != product.Category {
		return "用户主营类目与商品类目不匹配"
	}
	return ""
}

func (d *mockData) checkFreeSampleEligibility(userID string, productID string) (bool, string) {
	user, ok := d.userByID(userID)
	if !ok {
		return false, "用户不存在"
	}
	product, ok := d.productByID(productID)
	if !ok {
		return false, "商品不存在"
	}
	rule, ok := d.ProductRules[productID]
	if !ok {
		return false, "商品规则不存在"
	}
	reason := d.checkFreeSample(user, product, rule)
	return reason == "", reason
}

func (d *mockData) checkFreeSample(user mockUser, product mockProduct, rule mockProductRule) string {
	if hasApplication(d.FreeSampleApplications[user.UserID], product.ProductID) {
		return "用户已申请过该商品免费样品"
	}
	if !hasString(d.UserSelectionCarts[user.UserID], product.ProductID) {
		return "商品未加入用户选品车，不能申请免费样品"
	}
	if !rule.FreeSampleEnabled {
		return "商品暂不支持免费申样"
	}
	if !levelAtLeast(user.Level, rule.MinFreeSampleLevel) {
		return fmt.Sprintf("用户等级不足，需要达到%s等级", rule.MinFreeSampleLevel)
	}
	if rule.RequireCategoryMatchForFreeSample && user.MainCategory != product.Category {
		return "用户主营类目与商品类目不匹配"
	}
	if rule.FreeSampleQuota <= countFreeSampleApplicationsForProduct(d.FreeSampleApplications, product.ProductID) {
		return "该商品免费样品名额已满"
	}
	return ""
}

func (d *mockData) checkBuySample(user mockUser, product mockProduct, rule mockProductRule) string {
	if hasOrder(d.BuySampleOrders[user.UserID], product.ProductID) {
		return "用户已买样该商品"
	}
	if !hasString(d.UserSelectionCarts[user.UserID], product.ProductID) {
		return "商品未加入用户选品车，不能买样"
	}
	if !rule.PaidSampleEnabled {
		return "商品暂不支持买样"
	}
	if !levelAtLeast(user.Level, rule.MinPaidSampleLevel) {
		return fmt.Sprintf("用户等级不足，需要达到%s等级", rule.MinPaidSampleLevel)
	}
	return ""
}

func (d *mockData) recordAction(userID string, productID string, action actionType) {
	now := time.Now().Format(time.RFC3339)
	switch action {
	case actionTypeSelectionCart:
		d.UserSelectionCarts[userID] = appendIfMissing(d.UserSelectionCarts[userID], productID)
	case actionTypeFreeSample:
		d.FreeSampleApplications[userID] = append(d.FreeSampleApplications[userID], mockSampleApplication{
			ProductID: productID,
			Status:    "PROCESSING",
			CreatedAt: now,
		})
	case actionTypeBuySample:
		d.BuySampleOrders[userID] = append(d.BuySampleOrders[userID], mockSampleOrder{
			ProductID: productID,
			Status:    "PROCESSING",
			CreatedAt: now,
		})
	}
}

func toThirdProduct(product mockProduct) *third.Product {
	return &third.Product{
		ProductId:  product.ProductID,
		Name:       product.Name,
		Commission: product.Commission,
	}
}

func toThirdFailures(failures []mockFailureReason) []*third.ProductFailureReason {
	result := make([]*third.ProductFailureReason, 0, len(failures))
	for _, failure := range failures {
		result = append(result, &third.ProductFailureReason{
			ProductId: failure.ProductID,
			Reason:    failure.Reason,
		})
	}
	return result
}

func hasString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func appendIfMissing(values []string, target string) []string {
	if hasString(values, target) {
		return values
	}
	return append(values, target)
}

func levelAtLeast(current string, required string) bool {
	if required == "" {
		return true
	}
	return levelRank[current] >= levelRank[required]
}

func hasApplication(applications []mockSampleApplication, productID string) bool {
	for _, application := range applications {
		if application.ProductID == productID {
			return true
		}
	}
	return false
}

func hasOrder(orders []mockSampleOrder, productID string) bool {
	for _, order := range orders {
		if order.ProductID == productID {
			return true
		}
	}
	return false
}

func countFreeSampleApplicationsForProduct(applicationsByUser map[string][]mockSampleApplication, productID string) int {
	count := 0
	for _, applications := range applicationsByUser {
		for _, application := range applications {
			if application.ProductID == productID {
				count++
			}
		}
	}
	return count
}

func (d *mockData) userRelatedProductIDs(userID string) []string {
	seen := map[string]bool{}
	add := func(productID string) {
		if productID != "" {
			seen[productID] = true
		}
	}
	for _, productID := range d.UserSelectionCarts[userID] {
		add(productID)
	}
	for _, application := range d.FreeSampleApplications[userID] {
		add(application.ProductID)
	}
	for _, order := range d.BuySampleOrders[userID] {
		add(order.ProductID)
	}

	productIDs := make([]string, 0, len(seen))
	for productID := range seen {
		productIDs = append(productIDs, productID)
	}
	return productIDs
}

func (d *mockData) deriveFulfillmentStatus(userID string, productID string) *third.ProductFulfillmentStatus {
	if order, ok := findOrder(d.BuySampleOrders[userID], productID); ok {
		return &third.ProductFulfillmentStatus{
			ProductId:   productID,
			Stage:       third.FulfillmentStage_PAID_SAMPLE,
			Status:      toFulfillmentStatus(order.Status),
			Description: deriveBuySampleDescription(order),
			UpdatedAt:   order.CreatedAt,
		}
	}

	if application, ok := findApplication(d.FreeSampleApplications[userID], productID); ok {
		return &third.ProductFulfillmentStatus{
			ProductId:   productID,
			Stage:       third.FulfillmentStage_FREE_SAMPLE,
			Status:      toFulfillmentStatus(application.Status),
			Description: deriveFreeSampleDescription(application),
			UpdatedAt:   application.CreatedAt,
		}
	}

	if hasString(d.UserSelectionCarts[userID], productID) {
		return &third.ProductFulfillmentStatus{
			ProductId:   productID,
			Stage:       third.FulfillmentStage_CARTING,
			Status:      third.FulfillmentStatus_SUCCESS,
			Description: "商品已加入选品车",
		}
	}

	return &third.ProductFulfillmentStatus{
		ProductId:   productID,
		Stage:       third.FulfillmentStage_UNKNOWN,
		Status:      third.FulfillmentStatus_NOT_STARTED,
		Description: "用户尚未对该商品产生履约动作",
	}
}

func findApplication(applications []mockSampleApplication, productID string) (mockSampleApplication, bool) {
	for _, application := range applications {
		if application.ProductID == productID {
			return application, true
		}
	}
	return mockSampleApplication{}, false
}

func findOrder(orders []mockSampleOrder, productID string) (mockSampleOrder, bool) {
	for _, order := range orders {
		if order.ProductID == productID {
			return order, true
		}
	}
	return mockSampleOrder{}, false
}

func toFulfillmentStatus(status string) third.FulfillmentStatus {
	switch status {
	case "PROCESSING":
		return third.FulfillmentStatus_PROCESSING
	case "APPROVED", "SUCCESS":
		return third.FulfillmentStatus_SUCCESS
	case "FAILED":
		return third.FulfillmentStatus_FAILED
	case "CANCELLED":
		return third.FulfillmentStatus_CANCELLED
	default:
		return third.FulfillmentStatus_NOT_STARTED
	}
}

func deriveFreeSampleDescription(application mockSampleApplication) string {
	if application.Reason != "" {
		return application.Reason
	}
	switch application.Status {
	case "APPROVED", "SUCCESS":
		return "免费申样已通过"
	case "PROCESSING":
		return "免费申样已提交，等待商家审核"
	case "FAILED":
		return "免费申样失败"
	default:
		return "免费申样状态未知"
	}
}

func deriveBuySampleDescription(order mockSampleOrder) string {
	if order.Reason != "" {
		return order.Reason
	}
	switch order.Status {
	case "SUCCESS":
		return "买样已完成"
	case "PROCESSING":
		return "买样已提交，等待商家处理"
	case "FAILED":
		return "买样失败"
	default:
		return "买样状态未知"
	}
}
