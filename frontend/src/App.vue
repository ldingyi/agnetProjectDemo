<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'

type Role = 'user' | 'assistant'

interface ChatMessage {
  id: string
  role: Role
  content: string
  content_type: string
  payload: string
}

interface DisplayChatMessage extends ChatMessage {
  im_summary: IMChatSummaryResponse | null
}

interface SessionInfo {
  id: string
  title: string
  created_at: string
  updated_at: string
}

interface IMSummaryCard {
  conversation_id: string
  title: string
  summary: string
  latest_time: string
  product_ids: string[]
  product_names: string[]
  evidence: string[]
  answer_status: string
  next_action: string
}

interface IMConversationSummaryGroups {
  agreed: IMSummaryCard[]
  rejected: IMSummaryCard[]
  need_follow_up: IMSummaryCard[]
}

interface IMChatSummaryResponse {
  success: boolean
  error: string
  new_offers: IMSummaryCard[]
  conversation_summaries: IMConversationSummaryGroups
  updated_at: string
}

const input = ref('')
const loginUserID = ref('')
const userID = ref('')
const loginName = ref('')
const loading = ref(false)
const loggingIn = ref(false)
const error = ref('')
const messages = ref<ChatMessage[]>([])
const sessions = ref<SessionInfo[]>([])
const activeSessionID = ref('')
const messageList = ref<HTMLElement | null>(null)

const canSend = computed(() => input.value.trim().length > 0 && !loading.value)
const isLoggedIn = computed(() => userID.value !== '')
const activeSession = computed(() =>
  sessions.value.find((session) => session.id === activeSessionID.value),
)
const displayMessages = computed<DisplayChatMessage[]>(() =>
  messages.value.map((message) => ({
    ...message,
    im_summary: parseIMSummary(message),
  })),
)
function newID() {
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function normalizeMessages(rawMessages: Array<{ role: string; content: string; content_type?: string; payload?: string }>): ChatMessage[] {
  return rawMessages
    .filter((message) => message.role === 'user' || message.role === 'assistant')
    .map((message) => ({
      id: newID(),
      role: message.role as Role,
      content: message.content,
      content_type: message.content_type || 'text',
      payload: message.payload || '',
    }))
}

async function scrollToBottom() {
  await nextTick()
  if (messageList.value) {
    messageList.value.scrollTop = messageList.value.scrollHeight
  }
}

async function requestJSON<T>(url: string, options?: RequestInit): Promise<T> {
  const response = await fetch(url, options)
  if (!response.ok) {
    const text = await response.text()
    throw new Error(text || `Request failed with ${response.status}`)
  }
  return (await response.json()) as T
}

async function loadSessions() {
  const resp = await requestJSON<{ sessions: SessionInfo[] }>(
    `/api/sessions?user_id=${encodeURIComponent(userID.value)}`,
  )
  sessions.value = resp.sessions ?? []
}

async function login() {
  const id = loginUserID.value.trim()
  if (!id || loggingIn.value) {
    return
  }

  error.value = ''
  loggingIn.value = true
  try {
    const resp = await requestJSON<{
      success: boolean
      user_id: string
      username: string
      message: string
    }>('/api/login', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ user_id: id }),
    })
    userID.value = resp.user_id
    loginName.value = resp.username || id
    await initializeChat()
  } catch (err) {
    error.value = 'User not found.'
  } finally {
    loggingIn.value = false
  }
}

function logout() {
  userID.value = ''
  loginName.value = ''
  loginUserID.value = ''
  messages.value = []
  sessions.value = []
  activeSessionID.value = ''
  error.value = ''
}

async function initializeChat() {
  await loadSessions()
  if (sessions.value.length > 0) {
    await openSession(sessions.value[0].id)
  } else {
    await createSession()
  }
}

async function createSession() {
  error.value = ''
  const sessionResp = await requestJSON<{ session: SessionInfo }>(
    `/api/sessions?user_id=${encodeURIComponent(userID.value)}`,
    {
      method: 'POST',
    },
  )
  sessions.value = [
    sessionResp.session,
    ...sessions.value.filter((session) => session.id !== sessionResp.session.id),
  ]
  activeSessionID.value = sessionResp.session.id
  messages.value = []
  await scrollToBottom()
}

async function openSession(id: string) {
  if (loading.value || id === activeSessionID.value) {
    return
  }
  error.value = ''
  const resp = await requestJSON<{
    session: SessionInfo
    messages: Array<{ role: string; content: string }>
  }>(`/api/sessions/${encodeURIComponent(id)}?user_id=${encodeURIComponent(userID.value)}`)
  activeSessionID.value = resp.session.id
  sessions.value = [
    resp.session,
    ...sessions.value.filter((session) => session.id !== resp.session.id),
  ].sort((a, b) => Date.parse(b.updated_at || b.created_at) - Date.parse(a.updated_at || a.created_at))
  messages.value = normalizeMessages(resp.messages ?? [])
  await scrollToBottom()
}

async function refreshActiveSession() {
  await loadSessions()
  if (activeSessionID.value) {
    const latest = sessions.value.find((session) => session.id === activeSessionID.value)
    if (latest) {
      sessions.value = [
        latest,
        ...sessions.value.filter((session) => session.id !== activeSessionID.value),
      ]
    }
  }
}

async function sendMessage() {
  const content = input.value.trim()
  if (!content || loading.value || !activeSessionID.value) {
    return
  }

  await sendChatAction(content)
}

async function summarizeIMChats() {
  if (loading.value || !activeSessionID.value) {
    return
  }
  await sendChatAction('帮我总结当前 IM 会话')
}

async function sendChatAction(content: string) {
  error.value = ''
  input.value = ''
  const userMessage: ChatMessage = { id: newID(), role: 'user', content, content_type: 'text', payload: '' }
  const assistantMessage: ChatMessage = { id: newID(), role: 'assistant', content: '', content_type: 'text', payload: '' }
  messages.value.push(userMessage, assistantMessage)
  loading.value = true
  await scrollToBottom()

  try {
    const response = await fetch('/api/chat/stream', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        conversation_id: activeSessionID.value,
        user_id: userID.value,
        messages: [{ role: 'user', content, content_type: 'text' }],
      }),
    })

    if (!response.ok || !response.body) {
      const text = await response.text()
      throw new Error(text || `Request failed with ${response.status}`)
    }

    await readEventStream(response, (event) => {
      if (event.content_type) {
        assistantMessage.content_type = event.content_type
      }
      if (event.payload) {
        assistantMessage.payload = event.payload
      }
      assistantMessage.content += event.delta
      messages.value = messages.value.map((message) =>
        message.id === assistantMessage.id ? { ...assistantMessage } : message,
      )
      scrollToBottom()
    })
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    if (!assistantMessage.content) {
      assistantMessage.content = 'Request failed.'
    }
  } finally {
    loading.value = false
    await refreshActiveSession()
    await scrollToBottom()
  }
}

interface ChatStreamEvent {
  conversation_id: string
  delta: string
  done: boolean
  content_type: string
  payload: string
}

async function readEventStream(response: Response, onData: (data: ChatStreamEvent) => void) {
  const reader = response.body?.getReader()
  if (!reader) {
    return
  }

  const decoder = new TextDecoder()
  let buffer = ''

  for (;;) {
    const { value, done } = await reader.read()
    if (done) {
      break
    }
    buffer += decoder.decode(value, { stream: true })

    let boundary = buffer.indexOf('\n\n')
    while (boundary >= 0) {
      const frame = buffer.slice(0, boundary)
      buffer = buffer.slice(boundary + 2)
      handleSSEFrame(frame, onData)
      boundary = buffer.indexOf('\n\n')
    }
  }

  if (buffer.trim()) {
    handleSSEFrame(buffer, onData)
  }
}

function handleSSEFrame(frame: string, onData: (data: ChatStreamEvent) => void) {
  const lines = frame.split(/\r?\n/)
  const data = lines
    .filter((line) => line.startsWith('data:'))
    .map((line) => line.slice(5).trimStart())
    .join('\n')

  if (data) {
    onData(parseStreamData(data))
  }
}

function parseStreamData(data: string): ChatStreamEvent {
  try {
    const parsed = JSON.parse(data) as Partial<ChatStreamEvent>
    return {
      conversation_id: parsed.conversation_id || '',
      delta: parsed.delta || '',
      done: Boolean(parsed.done),
      content_type: parsed.content_type || '',
      payload: parsed.payload || '',
    }
  } catch {
    return {
      conversation_id: '',
      delta: data,
      done: false,
      content_type: '',
      payload: '',
    }
  }
}

function parseIMSummary(message: ChatMessage): IMChatSummaryResponse | null {
  if (message.content_type !== 'im_chat_summary' || !message.payload) {
    return null
  }
  try {
    return JSON.parse(message.payload) as IMChatSummaryResponse
  } catch {
    return null
  }
}

function summaryGroupsFor(summary: IMChatSummaryResponse | null): IMConversationSummaryGroups {
  return {
    agreed: summary?.conversation_summaries?.agreed ?? [],
    rejected: summary?.conversation_summaries?.rejected ?? [],
    need_follow_up: summary?.conversation_summaries?.need_follow_up ?? [],
  }
}

function hasSummaryCards(summary: IMChatSummaryResponse | null) {
  const groups = summaryGroupsFor(summary)
  return (
    (summary?.new_offers?.length ?? 0) > 0 ||
    groups.agreed.length > 0 ||
    groups.rejected.length > 0 ||
    groups.need_follow_up.length > 0
  )
}

function productsText(card: IMSummaryCard) {
  const names = card.product_names ?? []
  const ids = card.product_ids ?? []
  if (names.length > 0) {
    return names.join(' / ')
  }
  if (ids.length > 0) {
    return ids.join(' / ')
  }
  return ''
}

function formatSummaryTime(value: string) {
  if (!value) {
    return ''
  }
  const ts = Date.parse(value)
  if (Number.isNaN(ts)) {
    return value
  }
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(ts))
}

onMounted(async () => {
  loginUserID.value = ''
})
</script>

<template>
  <main v-if="!isLoggedIn" class="login-shell">
    <form class="login-panel" @submit.prevent="login">
      <p class="eyebrow">Agent</p>
      <h1>DeepSeek Chat</h1>
      <label>
        <span>User ID</span>
        <input v-model="loginUserID" type="text" autocomplete="off" placeholder="Enter User ID" />
      </label>
      <p v-if="error" class="error">{{ error }}</p>
      <button type="submit" :disabled="!loginUserID.trim() || loggingIn">
        <span>{{ loggingIn ? 'Signing in' : 'Sign in' }}</span>
      </button>
    </form>
  </main>

  <main v-else class="shell">
    <aside class="sidebar">
      <div class="brand">
        <div>
          <p class="eyebrow">Agent</p>
          <h1>DeepSeek Chat</h1>
        </div>
        <button class="new-session" type="button" :disabled="loading" @click="createSession">
          New
        </button>
      </div>
      <nav class="session-list">
        <button
          v-for="session in sessions"
          :key="session.id"
          class="session-item"
          :class="{ selected: session.id === activeSessionID }"
          type="button"
          :disabled="loading"
          @click="openSession(session.id)"
        >
          <span>{{ session.title || 'New Session' }}</span>
        </button>
      </nav>
      <div class="status">
        <span class="dot" :class="{ active: loading }"></span>
        <span>{{ loading ? 'Streaming' : loginName }}</span>
      </div>
      <button class="logout" type="button" :disabled="loading" @click="logout">Logout</button>
    </aside>

    <section class="workspace">
      <div ref="messageList" class="messages">
        <div v-if="messages.length === 0" class="empty">
          <h2>{{ activeSession ? activeSession.title : 'Start a conversation' }}</h2>
        </div>
        <article
          v-for="message in displayMessages"
          :key="message.id"
          class="message"
          :class="message.role"
        >
          <div class="label">{{ message.role }}</div>
          <template v-if="message.content_type === 'im_chat_summary'">
            <p>{{ message.content }}</p>
            <div v-if="message.im_summary" class="summary-message">
              <p v-if="!message.im_summary.success" class="summary-error">
                {{ message.im_summary.error || 'IM 聊天总结暂时不可用，请稍后再试。' }}
              </p>
              <p v-else-if="!hasSummaryCards(message.im_summary)" class="summary-empty">
                暂无需要提醒的 IM 会话。
              </p>
              <div v-else class="summary-sections">
                <section v-if="message.im_summary.new_offers?.length" class="summary-section">
                  <h3>新邀约</h3>
                  <div class="summary-cards">
                    <article
                      v-for="card in message.im_summary.new_offers"
                      :key="`msg-new-${message.id}-${card.conversation_id}`"
                      class="summary-card"
                    >
                      <div class="summary-card-top">
                        <strong>{{ card.title || card.conversation_id }}</strong>
                        <span>{{ formatSummaryTime(card.latest_time) }}</span>
                      </div>
                      <p>{{ card.summary }}</p>
                      <small v-if="productsText(card)">{{ productsText(card) }}</small>
                      <small v-if="card.next_action">{{ card.next_action }}</small>
                    </article>
                  </div>
                </section>

                <section v-if="summaryGroupsFor(message.im_summary).agreed.length" class="summary-section">
                  <h3>已同意</h3>
                  <div class="summary-cards">
                    <article
                      v-for="card in summaryGroupsFor(message.im_summary).agreed"
                      :key="`msg-agreed-${message.id}-${card.conversation_id}`"
                      class="summary-card"
                    >
                      <div class="summary-card-top">
                        <strong>{{ card.title || card.conversation_id }}</strong>
                        <span>{{ formatSummaryTime(card.latest_time) }}</span>
                      </div>
                      <p>{{ card.summary }}</p>
                      <small v-if="card.next_action">{{ card.next_action }}</small>
                    </article>
                  </div>
                </section>

                <section v-if="summaryGroupsFor(message.im_summary).rejected.length" class="summary-section">
                  <h3>已拒绝</h3>
                  <div class="summary-cards">
                    <article
                      v-for="card in summaryGroupsFor(message.im_summary).rejected"
                      :key="`msg-rejected-${message.id}-${card.conversation_id}`"
                      class="summary-card"
                    >
                      <div class="summary-card-top">
                        <strong>{{ card.title || card.conversation_id }}</strong>
                        <span>{{ formatSummaryTime(card.latest_time) }}</span>
                      </div>
                      <p>{{ card.summary }}</p>
                      <small v-if="card.next_action">{{ card.next_action }}</small>
                    </article>
                  </div>
                </section>

                <section v-if="summaryGroupsFor(message.im_summary).need_follow_up.length" class="summary-section">
                  <h3>需继续沟通</h3>
                  <div class="summary-cards">
                    <article
                      v-for="card in summaryGroupsFor(message.im_summary).need_follow_up"
                      :key="`msg-follow-${message.id}-${card.conversation_id}`"
                      class="summary-card"
                    >
                      <div class="summary-card-top">
                        <strong>{{ card.title || card.conversation_id }}</strong>
                        <span>{{ formatSummaryTime(card.latest_time) }}</span>
                      </div>
                      <p>{{ card.summary }}</p>
                      <small v-if="card.next_action">{{ card.next_action }}</small>
                    </article>
                  </div>
                </section>
              </div>
            </div>
          </template>
          <p v-else>{{ message.content || (message.role === 'assistant' ? '...' : '') }}</p>
        </article>
      </div>

      <section class="system-prompt-actions" aria-label="系统提示">
        <div>
          <p class="summary-kicker">系统提示</p>
          <h2>IM 聊天总结</h2>
        </div>
        <button class="summary-trigger" type="button" :disabled="loading" @click="summarizeIMChats">
          {{ loading ? '处理中' : '一键总结' }}
        </button>
      </section>

      <p v-if="error" class="error">{{ error }}</p>

      <form class="composer" @submit.prevent="sendMessage">
        <textarea
          v-model="input"
          rows="3"
          :disabled="!activeSessionID"
          placeholder="Ask the agent..."
          @keydown.enter.exact.prevent="sendMessage"
        ></textarea>
        <button type="submit" :disabled="!canSend">
          <span>{{ loading ? 'Sending' : 'Send' }}</span>
        </button>
      </form>
    </section>
  </main>
</template>
