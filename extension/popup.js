const $ = (id) => document.getElementById(id);
const DEFAULT_CLIENT_ID = "1493453061987893318";

async function load() {
  const s = await browser.storage.local.get(["clientId", "addr", "enabled", "buttons", "status", "useAssets", "activityType"]);
  $("clientId").value = s.clientId || DEFAULT_CLIENT_ID;
  $("addr").value = s.addr || "ws://127.0.0.1:6473/rpc";
  $("enabled").checked = s.enabled !== false;
  $("useAssets").checked = s.useAssets !== false;
  $("activityType").value = String(typeof s.activityType === "number" ? s.activityType : 3);
  const btns = s.buttons || [];
  if (btns[0]) { $("b1label").value = btns[0].label || ""; $("b1url").value = btns[0].url || ""; }
  if (btns[1]) { $("b2label").value = btns[1].label || ""; $("b2url").value = btns[1].url || ""; }
  renderStatus(s.status);
}

function renderStatus(st) {
  const el = $("status");
  if (!st) { el.innerHTML = '<span class="dot bad"></span>no status yet'; return; }
  const dot = st.ok ? "ok" : "bad";
  el.innerHTML = `<span class="dot ${dot}"></span>${st.msg}`;
}

$("save").addEventListener("click", async () => {
  const buttons = [];
  const b1l = $("b1label").value.trim(), b1u = $("b1url").value.trim();
  const b2l = $("b2label").value.trim(), b2u = $("b2url").value.trim();
  if (b1l && b1u) buttons.push({ label: b1l, url: b1u });
  if (b2l && b2u) buttons.push({ label: b2l, url: b2u });
  await browser.storage.local.set({
    clientId: $("clientId").value.trim(),
    addr: $("addr").value.trim(),
    enabled: $("enabled").checked,
    useAssets: $("useAssets").checked,
    activityType: parseInt($("activityType").value, 10) || 0,
    buttons,
  });
  await browser.runtime.sendMessage("reconnect");
});

$("reconnect").addEventListener("click", async () => {
  await browser.runtime.sendMessage("reconnect");
});

browser.storage.onChanged.addListener((ch) => {
  if (ch.status) renderStatus(ch.status.newValue);
});

load();
