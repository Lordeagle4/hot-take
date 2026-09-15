const state = { sessionId: null, activeSkillId: null, pending: false, config: null };

const elements = {
  form: document.querySelector("#chat-form"),
  input: document.querySelector("#message"),
  messages: document.querySelector("#messages"),
  trace: document.querySelector("#trace"),
  traceEmpty: document.querySelector("#trace-empty"),
  traceCount: document.querySelector("#trace-count"),
  mode: document.querySelector("#mode"),
  model: document.querySelector("#model-label"),
  dialog: document.querySelector("#plugins-dialog"),
  pluginList: document.querySelector("#plugin-list"),
  pluginCount: document.querySelector("#plugin-count"),
  pluginForm: document.querySelector("#plugin-form"),
  pluginError: document.querySelector("#plugin-error"),
};

async function api(path, options = {}) {
  const response = await fetch(path, { ...options, headers: { "Content-Type": "application/json", ...(options.headers || {}) } });
  const payload = await response.json();
  if (!response.ok) throw new Error(payload.error || `Request failed (${response.status}).`);
  return payload;
}

function escapeHtml(value) {
  return value.replace(/[&<>'"]/g, (character) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", '"': "&quot;" })[character]);
}

function formatText(value) {
  return escapeHtml(value).replace(/\*\*(.+?)\*\*/g, "<strong>$1</strong>").replace(/\n/g, "<br>");
}

function addMessage(role, content, node = null) {
  const message = document.createElement("div");
  message.className = `message ${role}`;
  message.innerHTML = `<div class="avatar">${role === "user" ? "Y" : "H"}</div><div class="bubble"></div>`;
  const bubble = message.querySelector(".bubble");
  if (node) bubble.append(node); else bubble.innerHTML = `<p>${formatText(content)}</p>`;
  elements.messages.append(message);
  elements.messages.scrollTop = elements.messages.scrollHeight;
  return message;
}

function loading() {
  return addMessage("agent", "", Object.assign(document.createElement("p"), { className: "typing", textContent: "Routing intent" }));
}

function renderTrace(events) {
  elements.traceEmpty.hidden = true;
  for (const item of events) {
    const row = document.createElement("li");
    row.className = item.status;
    const time = new Date(item.at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
    row.innerHTML = `<span class="trace-time">${time}</span><p class="trace-title">${escapeHtml(item.title)}</p><p class="trace-detail">${escapeHtml(item.detail)}</p>`;
    elements.trace.append(row);
  }
  elements.traceCount.textContent = String(elements.trace.children.length).padStart(2, "0");
  elements.trace.scrollTop = elements.trace.scrollHeight;
}

function updateStack(reply) {
  document.querySelectorAll(".stack-item").forEach((item) => item.classList.add("active"));
  document.querySelector("#active-skill").textContent = reply.skill.name;
  const plugins = reply.trace.find((event) => event.type === "plugin");
  const tools = reply.trace.find((event) => event.type === "tools");
  document.querySelector("#active-plugin").textContent = plugins?.detail || "None";
  document.querySelector("#active-tools").textContent = tools?.detail || "None";
  document.querySelector("#active-approval").textContent = reply.approval ? "Waiting" : "Clear";
}

function approvalCard(reply) {
  const fragment = document.querySelector("#approval-template").content.cloneNode(true);
  const card = fragment.querySelector(".approval-card");
  card.querySelector("[data-tool]").textContent = reply.approval.tool;
  card.querySelector("[data-plugin]").textContent = reply.approval.plugin;
  try { card.querySelector("[data-arguments]").textContent = JSON.stringify(JSON.parse(reply.approval.arguments), null, 2); }
  catch { card.querySelector("[data-arguments]").textContent = reply.approval.arguments; }
  card.querySelector("[data-approve]").addEventListener("click", () => decide(reply, true, card));
  card.querySelector("[data-reject]").addEventListener("click", () => decide(reply, false, card));
  return fragment;
}

async function decide(reply, approve, card) {
  card.querySelectorAll("button").forEach((button) => button.disabled = true);
  try {
    const next = await api("/api/approvals", { method: "POST", body: JSON.stringify({ sessionId: reply.sessionId, approvalId: reply.approval.id, approve, skillId: reply.skill.id }) });
    card.replaceWith(Object.assign(document.createElement("p"), { className: "muted", textContent: approve ? "Approved once." : "Rejected." }));
    consume(next);
  } catch (error) {
    card.querySelectorAll("button").forEach((button) => button.disabled = false);
    addMessage("agent", error.message);
  }
}

function consume(reply) {
  state.sessionId = reply.sessionId;
  state.activeSkillId = reply.skill.id;
  renderTrace(reply.trace);
  updateStack(reply);
  if (reply.text) addMessage("agent", reply.text);
  if (reply.approval) addMessage("agent", "", approvalCard(reply));
}

async function send(message) {
  if (state.pending || !message.trim()) return;
  state.pending = true;
  elements.form.querySelector("button").disabled = true;
  addMessage("user", message.trim());
  const waiting = loading();
  try {
    const reply = await api("/api/chat", { method: "POST", body: JSON.stringify({ sessionId: state.sessionId, message: message.trim() }) });
    waiting.remove();
    consume(reply);
  } catch (error) {
    waiting.remove();
    addMessage("agent", `Run failed: ${error.message}`);
  } finally {
    state.pending = false;
    elements.form.querySelector("button").disabled = false;
    elements.input.focus();
  }
}

function pluginCard(plugin) {
  const card = document.createElement("div");
  card.className = "plugin-card";
  card.innerHTML = `<div><h3>${escapeHtml(plugin.label)}</h3><p>${escapeHtml(plugin.description)}</p><div class="plugin-tags"><span class="tag">${plugin.kind === "remote_mcp" ? "REMOTE MCP" : "CONNECTOR"}</span><span class="tag">${plugin.approval.toUpperCase()} APPROVAL</span>${plugin.authorizationEnv ? `<button class="tag connect-token" type="button">${plugin.connected ? "CONNECTED" : "CONNECT TOKEN"}</button>` : ""}${plugin.allowedTools.map((tool) => `<span class="tag">${escapeHtml(tool)}</span>`).join("")}</div></div><label class="switch" title="Enable ${escapeHtml(plugin.label)}"><input type="checkbox" ${plugin.enabled ? "checked" : ""}><span></span></label>`;
  card.querySelector("input").addEventListener("change", async (event) => {
    try { await api(`/api/plugins/${plugin.id}/toggle`, { method: "POST", body: JSON.stringify({ enabled: event.target.checked }) }); }
    catch (error) { event.target.checked = !event.target.checked; window.alert(error.message); }
  });
  card.querySelector(".connect-token")?.addEventListener("click", async () => {
    const token = window.prompt(`Paste the access token for ${plugin.label}. It stays only in this running process.`);
    if (!token) return;
    try { await api(`/api/plugins/${plugin.id}/connect`, { method: "POST", body: JSON.stringify({ token }) }); await loadState(); }
    catch (error) { window.alert(error.message); }
  });
  return card;
}

async function loadState() {
  state.config = await api("/api/state");
  elements.mode.classList.toggle("live", state.config.live);
  elements.mode.innerHTML = `<i></i> ${state.config.live ? "LIVE API" : "SIMULATION"}`;
  elements.model.textContent = `MODEL ${state.config.model}`;
  elements.pluginCount.textContent = state.config.plugins.length;
  elements.pluginList.replaceChildren(...state.config.plugins.map(pluginCard));
}

elements.form.addEventListener("submit", (event) => {
  event.preventDefault();
  const message = elements.input.value;
  elements.input.value = "";
  elements.input.style.height = "auto";
  send(message);
});

elements.input.addEventListener("keydown", (event) => {
  if (event.key === "Enter" && !event.shiftKey) { event.preventDefault(); elements.form.requestSubmit(); }
});

elements.input.addEventListener("input", () => {
  elements.input.style.height = "auto";
  elements.input.style.height = `${Math.min(elements.input.scrollHeight, 150)}px`;
});

document.addEventListener("click", (event) => {
  if (event.target.matches(".inline-prompt")) send(event.target.dataset.prompt);
});

document.querySelector("#clear-button").addEventListener("click", () => {
  state.sessionId = null;
  state.activeSkillId = null;
  elements.messages.querySelectorAll(".message:not(.intro-message)").forEach((message) => message.remove());
  elements.trace.replaceChildren();
  elements.traceEmpty.hidden = false;
  elements.traceCount.textContent = "00";
  document.querySelectorAll(".stack-item").forEach((item, index) => item.classList.toggle("active", index === 0));
});

document.querySelector("#plugins-button").addEventListener("click", () => elements.dialog.showModal());
document.querySelector("#close-plugins").addEventListener("click", () => elements.dialog.close());
document.querySelector("#plugin-kind").addEventListener("change", (event) => {
  const remote = event.target.value === "remote_mcp";
  document.querySelector("#endpoint-label").hidden = !remote;
  document.querySelector("#connector-label").hidden = remote;
});

elements.pluginForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  elements.pluginError.textContent = "";
  const values = Object.fromEntries(new FormData(elements.pluginForm));
  const plugin = {
    id: values.id,
    label: values.label,
    description: values.description,
    kind: values.kind,
    serverUrl: values.kind === "remote_mcp" ? values.serverUrl : undefined,
    connectorId: values.kind === "connector" ? values.connectorId : undefined,
    authorizationEnv: values.authorizationEnv || undefined,
    allowedTools: values.allowedTools ? values.allowedTools.split(",").map((item) => item.trim()).filter(Boolean) : [],
    approval: values.approval,
    enabled: true,
    trusted: values.trusted === "on",
  };
  try {
    await api("/api/plugins", { method: "POST", body: JSON.stringify(plugin) });
    elements.pluginForm.reset();
    await loadState();
  } catch (error) { elements.pluginError.textContent = error.message; }
});

loadState().catch((error) => addMessage("agent", `Could not load the runtime: ${error.message}`));
