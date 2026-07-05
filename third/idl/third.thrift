namespace go third

enum IMSenderType {
    UNKNOWN = 0
    USER = 1
    MERCHANT = 2
}

enum IMMessageType {
    TEXT = 1
    INVITATION_CARD = 2
    CONTACT_CARD = 3
}

struct User {
    1: string user_id
    2: string name
    3: string level
    4: i64 fans_count
    5: string main_category
}

struct GetUserByNameRequest {
    1: string name
}

struct GetUserByNameResponse {
    1: bool found
    2: optional User user
}

struct GetUserByIDRequest {
    1: string user_id
}

struct GetUserByIDResponse {
    1: bool found
    2: optional User user
}

struct IMProductInfo {
    1: string product_id
    2: string name
    3: double commission
}

struct IMContactInfo {
    1: string name
    2: string phone
    3: string wechat
}

struct IMInvitationCard {
    1: string intro
    2: list<IMProductInfo> products
}

struct IMMessage {
    1: string message_id
    2: IMSenderType sender
    3: IMMessageType message_type
    4: string content
    5: string send_time
    6: optional IMInvitationCard invitation_card
    7: optional IMContactInfo contact_card
}

struct ListIMConversationsRequest {
    1: string user_id
}

struct ListIMConversationsResponse {
    1: list<string> conversation_ids
}

struct GetIMConversationMessagesRequest {
    1: string user_id
    2: string conversation_id
}

struct GetIMConversationMessagesResponse {
    1: list<IMMessage> messages
}

struct SendIMMessageRequest {
    1: string user_id
    2: string conversation_id
    3: string content
}

struct SendIMMessageResponse {
    1: IMMessage message
}

struct Product {
    1: string product_id
    2: string name
    3: double commission
}

struct RecommendProductsRequest {
    1: string user_id
}

struct RecommendProductsResponse {
    1: list<Product> products
}

struct CheckFreeSampleRequest {
    1: string user_id
    2: string product_id
}

struct CheckFreeSampleResponse {
    1: bool available
}

struct ProductFailureReason {
    1: string product_id
    2: string reason
}

struct AddSelectionCartRequest {
    1: string user_id
    2: list<string> product_ids
}

struct AddSelectionCartResponse {
    1: list<string> success_product_ids
    2: list<ProductFailureReason> failures
}

struct GetSelectionCartRequest {
    1: string user_id
}

struct GetSelectionCartResponse {
    1: list<string> product_ids
}

struct ApplyFreeSampleRequest {
    1: string user_id
    2: list<string> product_ids
}

struct ApplyFreeSampleResponse {
    1: list<string> success_product_ids
    2: list<ProductFailureReason> failures
}

struct BuySampleRequest {
    1: string user_id
    2: list<string> product_ids
}

struct BuySampleResponse {
    1: list<string> success_product_ids
    2: list<ProductFailureReason> failures
}

enum FulfillmentStage {
    UNKNOWN = 0
    CARTING = 1
    FREE_SAMPLE = 2
    PAID_SAMPLE = 3
    DELIVERY_FULFILLMENT = 4
}

enum FulfillmentStatus {
    NOT_STARTED = 0
    PROCESSING = 1
    SUCCESS = 2
    FAILED = 3
    CANCELLED = 4
}

struct ProductFulfillmentStatus {
    1: string product_id
    2: FulfillmentStage stage
    3: FulfillmentStatus status
    4: string description
    5: string updated_at
}

struct GetFulfillmentStatusRequest {
    1: string user_id
    2: list<string> product_ids
}

struct GetFulfillmentStatusResponse {
    1: list<ProductFulfillmentStatus> statuses
}

service ThirdBusinessService {
    GetUserByNameResponse GetUserByName(1: GetUserByNameRequest req)
    GetUserByIDResponse GetUserByID(1: GetUserByIDRequest req)
    ListIMConversationsResponse ListIMConversations(1: ListIMConversationsRequest req)
    GetIMConversationMessagesResponse GetIMConversationMessages(1: GetIMConversationMessagesRequest req)
    SendIMMessageResponse SendIMMessage(1: SendIMMessageRequest req)
    RecommendProductsResponse RecommendProducts(1: RecommendProductsRequest req)
    CheckFreeSampleResponse CheckFreeSample(1: CheckFreeSampleRequest req)
    AddSelectionCartResponse AddSelectionCart(1: AddSelectionCartRequest req)
    GetSelectionCartResponse GetSelectionCart(1: GetSelectionCartRequest req)
    ApplyFreeSampleResponse ApplyFreeSample(1: ApplyFreeSampleRequest req)
    BuySampleResponse BuySample(1: BuySampleRequest req)
    GetFulfillmentStatusResponse GetFulfillmentStatus(1: GetFulfillmentStatusRequest req)
}
