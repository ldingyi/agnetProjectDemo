namespace go chat

struct ChatMessage {
    1: string role
    2: string content
    3: string content_type
    4: string payload
}

struct ChatRequest {
    1: string conversation_id
    2: list<ChatMessage> messages
    3: string user_id
}

struct ChatResponse {
    1: string conversation_id
    2: string message
    3: string content_type
    4: string payload
}

struct ChatStreamChunk {
    1: string conversation_id
    2: string delta
    3: bool done
    4: string content_type
    5: string payload
}

struct SessionInfo {
    1: string id
    2: string title
    3: string created_at
    4: string updated_at
}

struct CreateSessionRequest {
    1: string user_id
}

struct CreateSessionResponse {
    1: SessionInfo session
}

struct ListSessionsRequest {
    1: string user_id
}

struct ListSessionsResponse {
    1: list<SessionInfo> sessions
}

struct GetSessionRequest {
    1: string id
    2: string user_id
}

struct GetSessionResponse {
    1: SessionInfo session
    2: list<ChatMessage> messages
}

struct LoginRequest {
    1: string user_id
}

struct LoginResponse {
    1: bool success
    2: string user_id
    3: string username
    4: string message
}

struct IMSummaryCard {
    1: string conversation_id
    2: string title
    3: string summary
    4: string latest_time
    5: list<string> product_ids
    6: list<string> product_names
    7: list<string> evidence
    8: string answer_status
    9: string next_action
}

struct IMConversationSummaryGroups {
    1: list<IMSummaryCard> agreed
    2: list<IMSummaryCard> rejected
    3: list<IMSummaryCard> need_follow_up
}

struct IMChatSummaryRequest {
    1: string user_id
}

struct IMChatSummaryResponse {
    1: bool success
    2: string error
    3: list<IMSummaryCard> new_offers
    4: IMConversationSummaryGroups conversation_summaries
    5: string updated_at
}

service AgentChatService {
    LoginResponse Login(1: LoginRequest req)
    ChatResponse Chat(1: ChatRequest req)
    ChatStreamChunk ChatStream(1: ChatRequest req) (streaming.mode="server")
    CreateSessionResponse CreateSession(1: CreateSessionRequest req)
    ListSessionsResponse ListSessions(1: ListSessionsRequest req)
    GetSessionResponse GetSession(1: GetSessionRequest req)
    IMChatSummaryResponse GetIMChatSummary(1: IMChatSummaryRequest req)
}
