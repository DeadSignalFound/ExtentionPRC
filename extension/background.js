const DEFAULT_ADDR = "ws://127.0.0.1:6473/rpc";
const DEFAULT_CLIENT_ID = "1493453061987893318";
const RECONNECT_MS = 5000;
const IDLE_SECS = 120;
const HEARTBEAT_MIN = 0.5;

let ws = null;
let connected = false;
let cfg = { clientId: "", addr: DEFAULT_ADDR, enabled: true };
let lastActivity = null;
let startTs = Math.floor(Date.now() / 1000);
let idleState = "active";
let reconnectTimer = null;
let lastErr = "";

async function loadCfg() {
  const s = await browser.storage.local.get(["clientId", "addr", "enabled", "buttons", "useAssets", "activityType"]);
  cfg.clientId = s.clientId || DEFAULT_CLIENT_ID;
  cfg.addr = s.addr || DEFAULT_ADDR;
  cfg.enabled = s.enabled !== false;
  cfg.buttons = s.buttons || [];
  cfg.useAssets = s.useAssets !== false;
  cfg.activityType = typeof s.activityType === "number" ? s.activityType : 3;
}

function setStatus(msg, ok) {
  browser.storage.local.set({ status: { msg, ok, t: Date.now() } });
}

function connect() {
  if (!cfg.enabled || !cfg.clientId) {
    setStatus("disabled or missing client id", false);
    return;
  }
  console.log("[rpc] connecting to", cfg.addr);
  try {
    ws = new WebSocket(cfg.addr);
  } catch (e) {
    lastErr = e.message || String(e);
    console.warn("[rpc] ws ctor failed:", lastErr);
    setStatus("ws ctor: " + lastErr, false);
    scheduleReconnect();
    return;
  }
  ws.onopen = () => {
    console.log("[rpc] ws open");
    lastErr = "";
    setStatus("connecting to discord...", true);
    ws.send(JSON.stringify({ type: "connect", client_id: cfg.clientId }));
  };
  ws.onmessage = (ev) => {
    let m;
    try { m = JSON.parse(ev.data); } catch { return; }
    console.log("[rpc] msg", m);
    if (m.status === "ok" && m.msg === "connected") {
      connected = true;
      setStatus("connected to discord", true);
      pushActivity(true);
    } else if (m.status === "ok" && (m.msg === "activity set" || m.msg === "activity sent")) {
      setStatus("activity pushed", true);
    } else if (m.status === "error") {
      setStatus(m.msg, false);
    }
  };
  ws.onclose = (ev) => {
    connected = false;
    const reason = lastErr || (ev && ev.reason) || "";
    const code = ev ? ev.code : 0;
    const msg = reason
      ? `disconnected (${code}): ${reason}`
      : code === 1006
        ? "disconnected (1006): bridge unreachable — is it running and is the address correct?"
        : `disconnected (${code})`;
    console.warn("[rpc]", msg);
    setStatus(msg, false);
    scheduleReconnect();
  };
  ws.onerror = (ev) => {
    lastErr = (ev && ev.message) ? ev.message : "connection failed";
    console.warn("[rpc] ws error", ev);
    setStatus("ws error: " + lastErr, false);
  };
}

function scheduleReconnect() {
  if (reconnectTimer) return;
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    connect();
  }, RECONNECT_MS);
}

function disconnect() {
  if (ws) {
    try { ws.send(JSON.stringify({ type: "clear" })); } catch {}
    try { ws.close(); } catch {}
    ws = null;
  }
  connected = false;
}

const niceNames = [
  [/(^|\.)youtube\.com$/, "YouTube"],
  [/(^|\.)github\.com$/, "GitHub"],
  [/(^|\.)twitch\.tv$/, "Twitch"],
  [/(^|\.)reddit\.com$/, "Reddit"],
  [/(^|\.)stackoverflow\.com$/, "Stack Overflow"],
  [/(^|\.)(twitter\.com|x\.com)$/, "Twitter / X"],
  [/(^|\.)wikipedia\.org$/, "Wikipedia"],
  [/(^|\.)google\.[a-z.]+$/, "Google"],
  [/(^|\.)discord\.com$/, "Discord"],
];

function siteName(host) {
  for (const [rx, n] of niceNames) if (rx.test(host)) return n;
  return host.replace(/^www\./, "");
}

function faviconFor(tab, host) {
  const f = tab && tab.favIconUrl;
  const good =
    f &&
    /^https:\/\//i.test(f) &&
    f.length <= 256 &&
    !/\.svg(\?|#|$)/i.test(f) &&
    !/[?&](type|format)=svg\b/i.test(f) &&
    !/^https:\/\/[^/]*(mozilla|firefox)/i.test(f);
  if (good) return f;
  return `https://icons.duckduckgo.com/ip3/${encodeURIComponent(host)}.ico`;
}

function describe(tab) {
  if (!tab || !tab.url) return null;
  let u;
  try { u = new URL(tab.url); } catch { return null; }
  if (!/^https?:$/.test(u.protocol)) return null;
  const host = u.hostname;
  const name = siteName(host);
  return {
    details: trim(tab.title || host, 128),
    detailsURL: tab.url,
    state: trim(name, 128),
    stateURL: tab.url,
    largeImage: faviconFor(tab, host),
    largeText: trim(name, 128),
    largeURL: tab.url,
    smallText: "Firefox",
  };
}

function trim(s, n) {
  s = (s || "").trim();
  if (s.length <= n) return s;
  return s.slice(0, n - 1) + "\u2026";
}

async function currentTab() {
  const tabs = await browser.tabs.query({ active: true, lastFocusedWindow: true });
  return tabs[0] || null;
}

const mediaByTab = new Map();
const MEDIA_STALE_MS = 10000;

function buildMedia(m) {
  const now = Date.now();
  const a = {
    name: trim(m.title, 128),
    type: 2,
    status_display_type: 1,
    details: trim(m.title, 128),
    details_url: m.url,
    state: trim(m.artist || m.app, 128),
    state_url: m.url,
  };
  if (!m.paused && m.duration > 0 && m.currentTime >= 0) {
    a.timestamps = {
      start: Math.floor(now - m.currentTime * 1000),
      end: Math.floor(now + Math.max(0, m.duration - m.currentTime) * 1000),
    };
  }
  if (cfg.useAssets) {
    const thumbOk = m.thumb && m.thumb.length <= 256;
    a.assets = {
      large_image: thumbOk ? m.thumb : "",
      large_text: trim(m.album || m.app, 128),
      large_url: m.url,
      small_text: m.paused ? "Paused" : m.app,
    };
  }
  return a;
}

function buildBrowse(d) {
  const a = {
    name: d.state,
    details: d.details,
    details_url: d.detailsURL,
    state: d.state,
    state_url: d.stateURL,
    timestamps: { start: startTs },
    type: cfg.activityType | 0,
    status_display_type: 1,
  };
  if (cfg.useAssets) {
    a.assets = {
      large_image: d.largeImage,
      large_text: d.largeText,
      large_url: d.largeURL,
      small_text: d.smallText,
    };
  }
  return a;
}

async function pushActivity(force) {
  if (!connected || !ws) return;
  if (idleState !== "active") {
    send({ type: "clear" });
    lastActivity = null;
    return;
  }
  const t = await currentTab();
  if (!t) return;

  const m = mediaByTab.get(t.id);
  const mediaFresh = m && Date.now() - m.t < MEDIA_STALE_MS;

  let activity, key;
  if (mediaFresh) {
    activity = buildMedia(m);
    const bucket = Math.floor((m.currentTime || 0) / 5);
    key = `m|${m.paused ? 1 : 0}|${m.title}|${m.artist}|${bucket}|${m.thumb}`;
  } else {
    const d = describe(t);
    if (!d) {
      send({ type: "clear" });
      lastActivity = null;
      return;
    }
    activity = buildBrowse(d);
    key = `b|${d.details}|${d.state}|${d.largeImage}|${cfg.activityType}`;
  }

  if (!force && key === lastActivity) return;
  lastActivity = key;

  if (Array.isArray(cfg.buttons) && cfg.buttons.length) {
    activity.buttons = cfg.buttons.slice(0, 2);
  }
  send({ type: "activity", activity });
}

function send(obj) {
  if (!ws || ws.readyState !== 1) return;
  try { ws.send(JSON.stringify(obj)); } catch {}
}

function heartbeat() {
  if (!cfg.enabled) return;
  if (!ws || ws.readyState !== 1) {
    if (!reconnectTimer) connect();
    return;
  }
  send({ type: "ping" });
  pushActivity();
}

try {
  browser.alarms.create("rpc-heartbeat", { periodInMinutes: HEARTBEAT_MIN });
  browser.alarms.onAlarm.addListener((a) => {
    if (a.name === "rpc-heartbeat") heartbeat();
  });
} catch (e) {
  console.warn("[rpc] alarms unavailable:", e);
}

browser.tabs.onActivated.addListener(() => pushActivity());
browser.tabs.onUpdated.addListener((id, info) => {
  if (info.url) mediaByTab.delete(id);
  if (info.url || info.title) pushActivity();
});
browser.tabs.onRemoved.addListener((id) => mediaByTab.delete(id));
browser.windows.onFocusChanged.addListener(() => pushActivity());

browser.idle.setDetectionInterval(IDLE_SECS);
browser.idle.onStateChanged.addListener((st) => {
  idleState = st;
  pushActivity(true);
});

browser.storage.onChanged.addListener(async (changes) => {
  if (changes.clientId || changes.addr || changes.enabled || changes.buttons || changes.useAssets || changes.activityType) {
    const prevEnabled = cfg.enabled;
    const prevAddr = cfg.addr;
    const prevClient = cfg.clientId;
    await loadCfg();
    if (cfg.addr !== prevAddr || cfg.clientId !== prevClient || cfg.enabled !== prevEnabled) {
      disconnect();
      startTs = Math.floor(Date.now() / 1000);
      if (cfg.enabled) connect();
    } else {
      lastActivity = null;
      pushActivity(true);
    }
  }
});

browser.runtime.onMessage.addListener(async (msg, sender) => {
  if (msg && msg.kind === "media" && sender && sender.tab) {
    const d = msg.data || {};
    mediaByTab.set(sender.tab.id, { ...d, t: Date.now() });
    const cur = await currentTab();
    if (cur && cur.id === sender.tab.id) pushActivity();
    return;
  }
  if (msg === "reconnect") {
    disconnect();
    connect();
    return { ok: true };
  }
  if (msg === "status") {
    return { connected, cfg };
  }
});

(async () => {
  await loadCfg();
  if (cfg.enabled) connect();
})();
