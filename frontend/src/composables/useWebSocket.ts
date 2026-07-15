import { ref, computed, onUnmounted, type Ref } from 'vue'
import type { WSMessage, StateUpdate } from '../types'

// ─── Logging ─────────────────────────────────────────
const log = (msg: string, ...params: any[]) => {
  console.log(`${new Date().toLocaleString()} ${msg}`, ...params)
}

const logerror = (msg: string, err: any) => {
  console.error(`${new Date().toLocaleString()} ${msg}`, err)
}

// ─── Connection Pool (module-level singleton) ────────
//
// Connections are keyed by type ('control' | 'player').
// Multiple components calling useWebSocket() share the same
// reactive state and underlying WebSocket for a given type.
// Reference counting ensures the connection is torn down
// only when the last consumer disconnects.

interface ConnectionState {
  ws: Ref<WebSocket | null>
  connected: Ref<boolean>
  stateUpdate: Ref<StateUpdate | null>
  handlers: Map<string, (msg: WSMessage) => void>
  reconnectTimer: ReturnType<typeof setTimeout> | null
  heartbeatTimer: ReturnType<typeof setInterval> | null
  refCount: number
}

const connections = new Map<string, ConnectionState>()

function createConnectionState(): ConnectionState {
  return {
    ws: ref(null),
    connected: ref(false),
    stateUpdate: ref(null),
    handlers: new Map(),
    reconnectTimer: null,
    heartbeatTimer: null,
    refCount: 0,
  }
}

// ─── Heartbeat ────────────────────────────────────────
function startHeartbeat(state: ConnectionState) {
  stopHeartbeat(state)
  state.heartbeatTimer = window.setInterval(() => {
    if (state.ws.value && state.ws.value.readyState === WebSocket.OPEN) {
      state.ws.value.send(JSON.stringify({ type: 'ping' }))
    }
  }, 5000)
}

function stopHeartbeat(state: ConnectionState) {
  if (state.heartbeatTimer !== null) {
    clearInterval(state.heartbeatTimer)
    state.heartbeatTimer = null
  }
}

// ─── Message Dispatch ────────────────────────────────
function dispatchMessage(state: ConnectionState, msg: WSMessage) {
  // Update stateUpdate
  if (msg.type === 'state_update') {
    state.stateUpdate.value = msg.payload as StateUpdate
  }

  // Exact match: `${type}:${action}`
  const exactKey = `${msg.type}:${msg.action}`
  const exactHandler = state.handlers.get(exactKey)
  if (exactHandler) {
    exactHandler(msg)
  }

  // Type-level wildcard: `${type}:*`
  const wildcardTypeKey = `${msg.type}:*`
  if (wildcardTypeKey !== exactKey) {
    const wildcardTypeHandler = state.handlers.get(wildcardTypeKey)
    if (wildcardTypeHandler) {
      wildcardTypeHandler(msg)
    }
  }

  // Global wildcard: `*`
  const globalHandler = state.handlers.get('*')
  if (globalHandler) {
    globalHandler(msg)
  }
}

// ─── Internal: create or reuse a WebSocket for a type ─
// This does NOT manage refCount; it's called both from
// the public `connect()` (which increments refCount) and
// from the auto-reconnect handler (which must not).
function ensureConnected(type: string, state: ConnectionState) {
  // Avoid duplicate connection attempts
  if (state.ws.value && (state.ws.value.readyState === WebSocket.OPEN || state.ws.value.readyState === WebSocket.CONNECTING)) {
    return
  }

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  const url = `${protocol}//${host}/ws?type=${type}`

  log(`[WS] Connecting as ${type}...`)
  const wsInstance = new WebSocket(url)
  state.ws.value = wsInstance

  wsInstance.onopen = () => {
    log('[WS] Connected')
    state.connected.value = true
    if (state.reconnectTimer) {
      clearTimeout(state.reconnectTimer)
      state.reconnectTimer = null
    }
    startHeartbeat(state)
  }

  wsInstance.onclose = () => {
    log('[WS] Disconnected')
    state.connected.value = false
    stopHeartbeat(state)
    // Auto-reconnect after 3 seconds (without incrementing refCount)
    state.reconnectTimer = window.setTimeout(() => {
      log('[WS] Reconnecting...')
      ensureConnected(type, state)
    }, 3000)
  }

  wsInstance.onerror = (err) => {
    logerror('[WS] Error:', err)
  }

  wsInstance.onmessage = (event) => {
    try {
      const msg: WSMessage = JSON.parse(event.data)
      // Respond to server ping
      if (msg.type === 'ping') {
        wsInstance.send(JSON.stringify({ type: 'pong' }))
        return
      }
      dispatchMessage(state, msg)
    } catch (e) {
      logerror('[WS] Failed to parse message:', e)
    }
  }
}

// ─── Composable ──────────────────────────────────────
export function useWebSocket() {
  const instanceType = ref<string | null>(null)
  // Per-instance queue for handlers registered before connect()
  const pendingHandlers: Array<{
    type: string
    action: string
    handler: (msg: WSMessage) => void
  }> = []

  // ── Reactive refs that always point to the shared connection state ──
  const ws = computed(() => {
    const t = instanceType.value
    if (!t) return null
    const state = connections.get(t)
    return state ? state.ws.value : null
  })

  const connected = computed(() => {
    const t = instanceType.value
    if (!t) return false
    const state = connections.get(t)
    return state ? state.connected.value : false
  })

  const stateUpdate = computed<StateUpdate | null>(() => {
    const t = instanceType.value
    if (!t) return null
    const state = connections.get(t)
    return state ? state.stateUpdate.value : null
  })

  // ── Lifecycle: clean up when this component unmounts ──
  onUnmounted(() => {
    if (instanceType.value) {
      releaseConnection(instanceType.value)
      instanceType.value = null
    }
  })

  // ── Connection Management ──
  function connect(type: string) {
    // If already bound to this type, just flush any pending handlers
    if (instanceType.value === type) {
      const state = connections.get(type)
      if (state) {
        flushPendingHandlers(state)
      }
      return
    }

    // Release previous type if switching
    if (instanceType.value) {
      releaseConnection(instanceType.value)
    }

    instanceType.value = type
    let state = connections.get(type)
    if (!state) {
      state = createConnectionState()
      connections.set(type, state)
    }
    state.refCount++

    // Flush any handlers registered before connect()
    flushPendingHandlers(state)

    // Establish the actual WebSocket connection
    ensureConnected(type, state)
  }

  function flushPendingHandlers(state: ConnectionState) {
    for (const pending of pendingHandlers) {
      state.handlers.set(`${pending.type}:${pending.action}`, pending.handler)
    }
    pendingHandlers.length = 0
  }

  function releaseConnection(type: string) {
    const state = connections.get(type)
    if (!state) return

    state.refCount--
    if (state.refCount <= 0) {
      log(`[WS] Last consumer released, closing ${type} connection`)
      stopHeartbeat(state)
      if (state.reconnectTimer) {
        clearTimeout(state.reconnectTimer)
        state.reconnectTimer = null
      }
      if (state.ws.value) {
        state.ws.value.close()
        state.ws.value = null
      }
      state.connected.value = false
      state.handlers.clear()
      connections.delete(type)
    }
  }

  function disconnect() {
    if (instanceType.value) {
      releaseConnection(instanceType.value)
      instanceType.value = null
    }
  }

  // ── Messaging ──
  function send(msg: WSMessage) {
    const t = instanceType.value
    if (!t) {
      console.warn('[WS] Cannot send, not connected to any type')
      return
    }
    const state = connections.get(t)
    if (!state || !state.ws.value || state.ws.value.readyState !== WebSocket.OPEN) {
      console.warn('[WS] Cannot send, not connected')
      return
    }
    state.ws.value.send(JSON.stringify(msg))
  }

  function onMessage(type: string, action: string, handler: (msg: WSMessage) => void) {
    const t = instanceType.value
    if (!t) {
      // Queue handler — will be flushed when connect() is called
      pendingHandlers.push({ type, action, handler })
      return
    }
    const state = connections.get(t)
    if (state) {
      state.handlers.set(`${type}:${action}`, handler)
    }
  }

  return {
    ws,
    connected,
    stateUpdate,
    connect,
    send,
    onMessage,
    disconnect,
    log,
    logerror,
  }
}
