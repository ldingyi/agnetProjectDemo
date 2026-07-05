<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'

type Role = 'user' | 'assistant'

interface ChatMessage {
  id: string
  role: Role
  content: string
}

interface SessionInfo {
  id: string
  title: string
  created_at: string
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

function newID() {
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function normalizeMessages(rawMessages: Array<{ role: string; content: string }>): ChatMessage[] {
  return rawMessages
    .filter((message) => message.role === 'user' || message.role === 'assistant')
    .map((message) => ({
      id: newID(),
      role: message.role as Role,
      content: message.content,
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

  error.value = ''
  input.value = ''
  const userMessage: ChatMessage = { id: newID(), role: 'user', content }
  const assistantMessage: ChatMessage = { id: newID(), role: 'assistant', content: '' }
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
        messages: [{ role: 'user', content }],
      }),
    })

    if (!response.ok || !response.body) {
      const text = await response.text()
      throw new Error(text || `Request failed with ${response.status}`)
    }

    await readEventStream(response, (delta) => {
      assistantMessage.content += delta
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

async function readEventStream(response: Response, onData: (data: string) => void) {
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

function handleSSEFrame(frame: string, onData: (data: string) => void) {
  const lines = frame.split(/\r?\n/)
  const data = lines
    .filter((line) => line.startsWith('data:'))
    .map((line) => line.slice(5).trimStart())
    .join('\n')

  if (data) {
    onData(data)
  }
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
          v-for="message in messages"
          :key="message.id"
          class="message"
          :class="message.role"
        >
          <div class="label">{{ message.role }}</div>
          <p>{{ message.content || (message.role === 'assistant' ? '...' : '') }}</p>
        </article>
      </div>

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
