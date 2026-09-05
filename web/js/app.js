(() => {
  const REFRESH_WS_PATH = "/api/v.1/refresh/websocket";

  const state = {
    view: "welcome",
    members: [],
    models: [],
    filter: "",
    nodesFilter: "",
    error: null,
    adminEnabled: false,
    invites: [],
    adminNodes: [],
    createdInvite: null,
    meshSize: { w: 0, h: 0 },
    selectedPeer: null,
    expandedPeer: null,
    expandedModel: null,
    chat: {
      model: "",
      messages: [],
      busy: false,
      abort: null,
    },
  };

  const el = {
    status: document.getElementById("conn-status"),
    statusLabel: document.getElementById("conn-status-label"),
    meshCount: document.getElementById("mesh-count"),
    nodesCount: document.getElementById("nodes-count"),
    modelsCount: document.getElementById("models-count"),
    meshSubtitle: document.getElementById("mesh-subtitle"),
    meshCanvas: document.getElementById("mesh-canvas"),
    meshGraph: document.getElementById("mesh-graph"),
    nodeDetail: document.getElementById("node-detail"),
    nodesBody: document.getElementById("nodes-body"),
    nodesSearch: document.getElementById("nodes-search"),
    modelsBody: document.getElementById("models-body"),
    search: document.getElementById("models-search"),
    chatModel: document.getElementById("chat-model"),
    chatThread: document.getElementById("chat-thread"),
    chatForm: document.getElementById("chat-form"),
    chatInput: document.getElementById("chat-input"),
    chatSend: document.getElementById("chat-send"),
    btnNewChat: document.getElementById("btn-new-chat"),
    chatPrivacy: document.getElementById("chat-privacy"),
    settingsYaml: document.getElementById("settings-yaml"),
    settingsSubtitle: document.getElementById("settings-subtitle"),
    welcomeOpenai: document.getElementById("welcome-openai-url"),
    welcomeModels: document.getElementById("welcome-models-url"),
    welcomeChat: document.getElementById("welcome-chat-url"),
    welcomeCurl: document.getElementById("welcome-curl"),
    welcomeLocalNote: document.getElementById("welcome-local-note"),
    navAdmin: document.getElementById("nav-admin"),
    adminCount: document.getElementById("admin-count"),
    adminOpen: document.getElementById("admin-invite-open"),
    adminModal: document.getElementById("admin-invite-modal"),
    adminForm: document.getElementById("admin-invite-form"),
    adminName: document.getElementById("admin-invite-name"),
    adminLifetime: document.getElementById("admin-invite-lifetime"),
    adminOnce: document.getElementById("admin-invite-once"),
    adminError: document.getElementById("admin-invite-error"),
    adminNodesError: document.getElementById("admin-nodes-error"),
    adminModalError: document.getElementById("admin-invite-modal-error"),
    adminCreated: document.getElementById("admin-invite-created"),
    adminCreatedLink: document.getElementById("admin-invite-link"),
    adminCopy: document.getElementById("admin-invite-copy"),
    adminInvitesBody: document.getElementById("admin-invites-body"),
    adminNodesBody: document.getElementById("admin-nodes-body"),
  };

  function setStatus(kind, label) {
    el.status.classList.remove("online", "offline", "loading");
    el.status.classList.add(kind);
    el.statusLabel.textContent = label;
  }

  async function getJSON(path) {
    const res = await fetch(path, { headers: { Accept: "application/json" } });
    if (!res.ok) {
      throw new Error(`${path}: ${res.status} ${res.statusText}`);
    }
    return res.json();
  }

  async function sendJSON(path, method, body) {
    const res = await fetch(path, {
      method,
      headers: { Accept: "application/json", "Content-Type": "application/json" },
      body: body == null ? undefined : JSON.stringify(body),
    });
    if (!res.ok) {
      const text = (await res.text()).trim();
      throw new Error(text || `${path}: ${res.status} ${res.statusText}`);
    }
    const ct = res.headers.get("content-type") || "";
    if (!ct.includes("application/json")) return null;
    return res.json();
  }

  function escapeHTML(s) {
    return String(s ?? "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function shortID(id) {
    if (!id) return "";
    if (id.length <= 14) return id;
    return id.slice(0, 6) + "…" + id.slice(-4);
  }

  function modelName(m) {
    return (m && (m.name || m.model)) || "";
  }

  function formatContext(n) {
    const v = Number(n) || 0;
    if (v <= 0) return "N/A";
    return v.toLocaleString();
  }

  function formatModified(m) {
    const t = m && m.modified_at;
    if (!t || t === "0001-01-01T00:00:00Z") return "";
    const d = new Date(t);
    return Number.isNaN(d.getTime()) ? "" : d.toLocaleString();
  }

  function visibilityBadge(m) {
    return m && m.private
      ? `<span class="badge badge-private">private</span>`
      : `<span class="badge badge-shared">shared</span>`;
  }

  function memberName(m) {
    return m.Name || shortID(m.PeerID);
  }

  function sortedMembers() {
    return [...state.members].sort((a, b) => {
      if ((a.Type === "self") !== (b.Type === "self")) return a.Type === "self" ? -1 : 1;
      return memberName(a).localeCompare(memberName(b));
    });
  }

  function kindRank(kind) {
    if (kind === "direct") return 3;
    if (kind === "relay/unlimited") return 2;
    if (kind && kind.startsWith("relay")) return 1;
    if (kind === "limited") return 1;
    return 0;
  }

  function buildEdges(members) {
    const ids = new Set(members.map((m) => m.PeerID));
    const edges = new Map();
    for (const m of members) {
      const conns = (m.Mesh && m.Mesh.Connections) || [];
      for (const c of conns) {
        if (!c.PeerID || c.PeerID === m.PeerID || !ids.has(c.PeerID)) continue;
        const key = [m.PeerID, c.PeerID].sort().join("|");
        const prev = edges.get(key);
        if (!prev || kindRank(c.Kind) > kindRank(prev.kind)) {
          edges.set(key, { a: m.PeerID, b: c.PeerID, kind: c.Kind || "unknown" });
        }
      }
    }
    return [...edges.values()];
  }

  function layoutCircle(nodes, cx, cy, r) {
    const n = nodes.length;
    if (n === 0) return [];
    if (n === 1) return [{ ...nodes[0], x: cx, y: cy }];
    return nodes.map((node, i) => {
      const a = -Math.PI / 2 + (2 * Math.PI * i) / n;
      return { ...node, x: cx + r * Math.cos(a), y: cy + r * Math.sin(a) };
    });
  }

  function lineClass(kind) {
    if (kind === "direct") return "mesh-link mesh-link-direct";
    if (kind && kind.startsWith("relay")) return "mesh-link mesh-link-relay";
    return "mesh-link mesh-link-limited";
  }

  function meshCanvasSize() {
    const rect = el.meshCanvas.getBoundingClientRect();
    return {
      w: Math.max(0, Math.round(rect.width)),
      h: Math.max(0, Math.round(rect.height)),
    };
  }

  function selfMember() {
    return state.members.find((m) => m.Type === "self") || null;
  }

  function providerID(p) {
    return (p && p.ID) || "";
  }

  function providerName(p) {
    if (!p) return "";
    return p.Name || shortID(providerID(p));
  }

  function providerIsSelf(p) {
    const self = selfMember();
    const id = providerID(p);
    return !!(self && id && self.PeerID === id);
  }

  function sortedProviders(list) {
    return [...(list || [])].sort((a, b) => {
      const as = providerIsSelf(a);
      const bs = providerIsSelf(b);
      if (as !== bs) return as ? -1 : 1;
      return providerName(a).localeCompare(providerName(b));
    });
  }

  function modelsForPeer(peerID) {
    return state.models.filter((m) =>
      (m.providers || []).some((p) => providerID(p) === peerID)
    );
  }

  function relatedPeerIds(peerID, edges) {
    const related = new Set([peerID]);
    for (const e of edges) {
      if (e.a === peerID) related.add(e.b);
      if (e.b === peerID) related.add(e.a);
    }
    return related;
  }

  function hideNodeDetail() {
    el.nodeDetail.classList.add("hidden");
    el.nodeDetail.hidden = true;
    el.nodeDetail.innerHTML = "";
  }

  function renderNodeDetail(member) {
    if (!member) {
      hideNodeDetail();
      return;
    }

    const models = modelsForPeer(member.PeerID);
    const conns = (member.Mesh && member.Mesh.Connections) || [];
    const modelRows = models.length
      ? models
          .map((m) => {
            const name = modelName(m);
            const meta = m.owner || formatContext(m.context_length);
            return `<div class="node-detail-model">
              <span class="node-detail-model-name" title="${escapeHTML(name)}">${escapeHTML(name)}</span>
              <span class="node-detail-model-meta">${escapeHTML(meta === "N/A" ? "" : meta)}</span>
              ${visibilityBadge(m)}
            </div>`;
          })
          .join("")
      : `<div class="empty">No models advertised on this node.</div>`;

    el.nodeDetail.hidden = false;
    el.nodeDetail.classList.remove("hidden");
    el.nodeDetail.innerHTML = `
      <div class="node-detail-header">
        <div>
          <h3 class="node-detail-title">${escapeHTML(memberName(member))}</h3>
          <p class="node-detail-id">${escapeHTML(member.PeerID)}</p>
        </div>
        <button type="button" class="btn btn-ghost btn-sm" data-close-detail aria-label="Close">✕</button>
      </div>
      <div class="node-detail-body">
        <div class="node-detail-meta">
          ${member.Type === "self" ? `<span class="badge badge-online">this node</span>` : `<span class="badge">peer</span>`}
          ${member.Reachable ? `<span class="badge badge-running">reachable</span>` : `<span class="badge badge-offline">unreachable</span>`}
          <span class="badge">${conns.length} link${conns.length === 1 ? "" : "s"}</span>
        </div>
        <p class="node-detail-section">Models</p>
        <div class="node-detail-models">${modelRows}</div>
      </div>`;
  }

  function renderMesh() {
    const members = sortedMembers();

    const reachable = members.filter((m) => m.Reachable).length;
    el.meshCount.textContent = String(members.length);
    el.meshSubtitle.textContent = members.length
      ? `${members.length} member${members.length === 1 ? "" : "s"} · ${reachable} reachable`
      : "Peer connectivity across the mesh";

    if (state.selectedPeer && !members.some((m) => m.PeerID === state.selectedPeer)) {
      state.selectedPeer = null;
    }

    if (!members.length) {
      el.meshGraph.innerHTML = `<div class="empty">No mesh members yet.</div>`;
      hideNodeDetail();
      return;
    }

    const { w, h } = meshCanvasSize();
    if (w < 32 || h < 32) return;
    state.meshSize = { w, h };

    const cx = w / 2;
    const cy = h / 2;
    const nodeR = Math.max(14, Math.min(32, Math.min(w, h) * 0.045));
    const labelSpace = nodeR + 40;
    const r = Math.max(nodeR + 8, Math.min(cx, cy) - labelSpace);
    const placed = layoutCircle(members, cx, cy, r);
    const byID = Object.fromEntries(placed.map((n) => [n.PeerID, n]));
    const edges = buildEdges(members);
    const selected = state.selectedPeer;
    const related = selected ? relatedPeerIds(selected, edges) : null;
    const labelY = nodeR + 16;
    const idY = nodeR + 32;
    const graphClass = selected ? "mesh-graph has-selection" : "mesh-graph";

    const links = edges
      .map((e) => {
        const a = byID[e.a];
        const b = byID[e.b];
        if (!a || !b) return "";
        const isRelated = selected && (e.a === selected || e.b === selected);
        const linkState = selected ? (isRelated ? " is-related" : " is-muted") : "";
        return `<line class="${lineClass(e.kind)}${linkState}" x1="${a.x}" y1="${a.y}" x2="${b.x}" y2="${b.y}">
          <title>${escapeHTML(memberName(a))} ↔ ${escapeHTML(memberName(b))} (${escapeHTML(e.kind)})</title>
        </line>`;
      })
      .join("");

    const nodes = placed
      .map((n) => {
        const isSelected = selected === n.PeerID;
        const isRelated = related && related.has(n.PeerID);
        const cls = [
          "mesh-node",
          n.Type === "self" ? "is-self" : "",
          n.Reachable ? "is-up" : "is-down",
          isSelected ? "is-selected" : "",
          selected && isRelated && !isSelected ? "is-related" : "",
          selected && !isRelated ? "is-muted" : "",
        ]
          .filter(Boolean)
          .join(" ");
        const label = escapeHTML(memberName(n));
        const sub = escapeHTML(shortID(n.PeerID));
        return `
          <g class="${cls}" data-peer-id="${escapeHTML(n.PeerID)}" transform="translate(${n.x} ${n.y})">
            <circle class="mesh-node-hit" r="${nodeR + 12}"></circle>
            <circle class="mesh-node-dot" r="${nodeR}"></circle>
            <text class="mesh-node-label" y="${labelY}">${label}</text>
            <text class="mesh-node-id" y="${idY}">${sub}</text>
            <title>${label}\n${escapeHTML(n.PeerID)}\n${n.Reachable ? "reachable" : "unreachable"}</title>
          </g>`;
      })
      .join("");

    el.meshGraph.className = graphClass;
    el.meshGraph.innerHTML = `
      <svg viewBox="0 0 ${w} ${h}" preserveAspectRatio="xMidYMid meet" role="img" aria-label="Mesh connectivity">
        <circle class="mesh-ring" cx="${cx}" cy="${cy}" r="${r}"></circle>
        <circle class="mesh-ring mesh-ring-inner" cx="${cx}" cy="${cy}" r="${r * 0.5}"></circle>
        <g class="mesh-links">${links}</g>
        <g class="mesh-nodes">${nodes}</g>
      </svg>`;

    renderNodeDetail(selected ? byID[selected] : null);
  }

  function nodeExpandHTML(member) {
    const models = modelsForPeer(member.PeerID);
    const mesh = member.Mesh || {};
    const addrs = mesh.AdvertisedAddresses || [];
    const conns = mesh.Connections || [];

    const modelList = models.length
      ? models
          .map((m) => {
            const name = modelName(m);
            const meta = m.owner || formatContext(m.context_length);
            return `<div class="node-detail-model">
              <span class="node-detail-model-name">${escapeHTML(name)}</span>
              <span class="node-detail-model-meta">${escapeHTML(meta === "N/A" ? "" : meta)}</span>
              ${visibilityBadge(m)}
            </div>`;
          })
          .join("")
      : `<div class="empty">No models advertised on this node.</div>`;

    const addrList = addrs.length
      ? `<table class="node-expand-addrs"><tbody>${addrs
          .map((a) => `<tr><td class="mono">${escapeHTML(a)}</td></tr>`)
          .join("")}</tbody></table>`
      : `<div class="empty">No advertised addresses.</div>`;

    const connList = conns.length
      ? `<table class="node-expand-conns">
          <thead><tr><th>Peer</th><th>Kind</th><th>Dir</th><th>Remote</th><th>Streams</th></tr></thead>
          <tbody>${conns
            .map((c) => `<tr>
              <td>${escapeHTML(c.PeerName || shortID(c.PeerID))}</td>
              <td><span class="badge">${escapeHTML(c.Kind || "—")}</span></td>
              <td class="mono">${escapeHTML(c.Direction || "—")}</td>
              <td class="mono">${escapeHTML(c.RemoteAddress || "—")}</td>
              <td class="mono">${c.StreamCount ?? 0}</td>
            </tr>`)
            .join("")}</tbody>
        </table>`
      : `<div class="empty">No active connections.</div>`;

    return `<div class="node-expand">
      <div class="node-expand-section">
        <h4>Identity</h4>
        <p class="node-expand-id">${escapeHTML(member.PeerID)}</p>
        <div class="node-detail-meta">
          ${member.Type === "self" ? `<span class="badge badge-online">this node</span>` : `<span class="badge">peer</span>`}
          ${member.Reachable ? `<span class="badge badge-running">reachable</span>` : `<span class="badge badge-offline">unreachable</span>`}
        </div>
      </div>
      <div class="node-expand-section">
        <h4>Models</h4>
        <div class="node-detail-models">${modelList}</div>
      </div>
      <div class="node-expand-section">
        <h4>Advertised addresses</h4>
        ${addrList}
      </div>
      <div class="node-expand-section node-expand-full">
        <h4>Connections</h4>
        ${connList}
      </div>
    </div>`;
  }

  function renderNodes() {
    const q = state.nodesFilter.trim().toLowerCase();
    const members = sortedMembers().filter((m) => {
      if (!q) return true;
      const models = modelsForPeer(m.PeerID)
        .map((model) => modelName(model))
        .join(" ");
      return `${memberName(m)} ${m.PeerID} ${m.Type || ""} ${models}`.toLowerCase().includes(q);
    });

    el.nodesCount.textContent = String(state.members.length);

    if (state.expandedPeer && !state.members.some((m) => m.PeerID === state.expandedPeer)) {
      state.expandedPeer = null;
    }

    if (!members.length) {
      const msg = state.members.length ? "No nodes match that filter." : "No mesh members yet.";
      el.nodesBody.innerHTML = `<tr><td colspan="6" class="empty">${msg}</td></tr>`;
      return;
    }

    el.nodesBody.innerHTML = members
      .map((m) => {
        const models = modelsForPeer(m.PeerID);
        const conns = (m.Mesh && m.Mesh.Connections) || [];
        const expanded = state.expandedPeer === m.PeerID;
        const role = m.Type === "self" ? `<span class="badge badge-online">this node</span>` : `<span class="badge">peer</span>`;
        const status = m.Reachable
          ? `<span class="badge badge-running">reachable</span>`
          : `<span class="badge badge-offline">unreachable</span>`;
        const modelChips = models.length
          ? models
              .map((model) => `<span class="node-chip">${escapeHTML(modelName(model))}</span>`)
              .join("")
          : "—";
        return `<tr class="clickable-row node-row${expanded ? " is-expanded" : ""}" data-peer-id="${escapeHTML(m.PeerID)}">
          <td>
            <div class="cell-name"><span class="node-chevron">▾</span> ${escapeHTML(memberName(m))}</div>
          </td>
          <td class="mono">${escapeHTML(shortID(m.PeerID))}</td>
          <td>${role}</td>
          <td>${status}</td>
          <td><div class="node-chip-row">${modelChips}</div></td>
          <td class="mono">${conns.length}</td>
        </tr>
        ${
          expanded
            ? `<tr class="node-expand-row"><td colspan="6">${nodeExpandHTML(m)}</td></tr>`
            : ""
        }`;
      })
      .join("");
  }

  function detailKV(label, value) {
    if (value === undefined || value === null || value === "" || value === 0) return "";
    return `<div class="profile-grid-row"><span class="label">${escapeHTML(label)}</span><span class="value">${value}</span></div>`;
  }

  function modelCapabilities(m) {
    const raw = (m && (m.capabilities || m.Capabilities)) || [];
    if (!Array.isArray(raw)) return [];
    const seen = new Set();
    const out = [];
    for (const item of raw) {
      const s = String(item || "").trim();
      if (!s) continue;
      const key = s.toLowerCase();
      if (seen.has(key)) continue;
      seen.add(key);
      out.push(s);
    }
    return out;
  }

  function formatCapability(c) {
    return String(c || "")
      .trim()
      .replace(/[_-]+/g, " ")
      .replace(/\b\w/g, (ch) => ch.toUpperCase());
  }

  function modelCapabilitiesHTML(caps) {
    if (!caps.length) {
      return `<div class="empty">No capabilities reported.</div>`;
    }
    return `<div class="node-chip-row model-cap-row">${caps
      .map((c) => `<span class="model-cap-chip">${escapeHTML(formatCapability(c))}</span>`)
      .join("")}</div>`;
  }

  function modelExpandHTML(m) {
    const providers = sortedProviders(m.providers)
      .map((p) => {
        return `<div class="node-detail-model">
          <span class="node-detail-model-name">${escapeHTML(providerName(p))}</span>
          <span class="node-detail-model-meta mono">${escapeHTML(providerID(p))}</span>
          ${providerIsSelf(p) ? `<span class="badge badge-online">this node</span>` : ""}
        </div>`;
      })
      .join("") || `<div class="empty">No providers.</div>`;

    const specs = [
      detailKV("Owner", escapeHTML(m.owner || "")),
      detailKV("Context", escapeHTML(formatContext(m.context_length))),
      detailKV("Modified", escapeHTML(formatModified(m))),
      detailKV("Visibility", m.private ? "Private" : "Shared"),
    ].join("");
    const caps = modelCapabilities(m);
    const alias =
      m.model && m.name && m.model !== m.name
        ? `<p class="node-expand-id">${escapeHTML(m.model)}</p>`
        : "";

    return `<div class="node-expand">
      <div class="node-expand-section">
        <h4>Identity</h4>
        <p class="node-expand-id">${escapeHTML(modelName(m))}</p>
        ${alias}
        <div class="node-detail-meta">
          ${visibilityBadge(m)}
        </div>
      </div>
      <div class="node-expand-section">
        <h4>Details</h4>
        <div class="model-spec-grid">${specs || `<div class="empty">No extra details.</div>`}</div>
      </div>
      <div class="node-expand-section node-expand-full">
        <h4>Capabilities</h4>
        ${modelCapabilitiesHTML(caps)}
      </div>
      <div class="node-expand-section node-expand-full">
        <h4>Provided by</h4>
        <div class="node-detail-models">${providers}</div>
      </div>
    </div>`;
  }

  function renderModels() {
    const q = state.filter.trim().toLowerCase();
    const rows = state.models.filter((m) => {
      if (!q) return true;
      const hay = [
        m.name,
        m.model,
        m.owner,
        m.private ? "private" : "shared",
        m.context_length,
        ...modelCapabilities(m),
        ...(m.providers || []).map((p) => `${providerName(p)} ${providerID(p)}`),
      ]
        .join(" ")
        .toLowerCase();
      return hay.includes(q);
    });

    el.modelsCount.textContent = String(state.models.length);

    if (state.expandedModel && !state.models.some((m) => modelName(m) === state.expandedModel)) {
      state.expandedModel = null;
    }

    if (!rows.length) {
      const msg = state.models.length ? "No models match that filter." : "No models advertised on the mesh.";
      el.modelsBody.innerHTML = `<tr><td colspan="6" class="empty">${msg}</td></tr>`;
      return;
    }

    el.modelsBody.innerHTML = rows
      .map((m) => {
        const key = modelName(m);
        const expanded = state.expandedModel === key;
        const providers = sortedProviders(m.providers)
          .map((p) => {
            const cls = providerIsSelf(p) ? "node-chip is-self" : "node-chip";
            const title = escapeHTML(providerID(p));
            return `<span class="${cls}" title="${title}">${escapeHTML(providerName(p))}</span>`;
          })
          .join("");
        const caps = modelCapabilities(m);
        const capCell = caps.length
          ? `<div class="node-chip-row">${caps
              .map((c) => `<span class="model-cap-chip">${escapeHTML(formatCapability(c))}</span>`)
              .join("")}</div>`
          : "—";
        const alias =
          m.model && m.name && m.model !== m.name
            ? `<div class="mono muted">${escapeHTML(m.model)}</div>`
            : "";
        return `<tr class="clickable-row node-row${expanded ? " is-expanded" : ""}" data-model-name="${escapeHTML(key)}">
          <td>
            <div class="cell-name"><span class="node-chevron">▾</span> ${escapeHTML(key)}</div>
            ${alias}
          </td>
          <td>${escapeHTML(m.owner || "—")}</td>
          <td class="mono">${escapeHTML(formatContext(m.context_length))}</td>
          <td>${visibilityBadge(m)}</td>
          <td><div class="node-chip-row">${providers || "—"}</div></td>
          <td>${capCell}</td>
        </tr>
        ${expanded ? `<tr class="node-expand-row"><td colspan="6">${modelExpandHTML(m)}</td></tr>` : ""}`;
      })
      .join("");
  }

  function modelNames() {
    return state.models
      .map((m) => modelName(m))
      .filter(Boolean)
      .sort((a, b) => a.localeCompare(b));
  }

  function renderMarkdown(text) {
    const src = String(text || "");
    const fences = [];
    let html = escapeHTML(src).replace(/```(?:\w+)?\n([\s\S]*?)```/g, (_, code) => {
      fences.push(`<pre><code>${code.replace(/\n$/, "")}</code></pre>`);
      return `\u0000FENCE${fences.length - 1}\u0000`;
    });
    html = html.replace(/`([^`]+)`/g, "<code>$1</code>");
    html = html.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
    html = html.replace(/(^|[^\*])\*([^*\n]+)\*/g, "$1<em>$2</em>");
    html = html
      .split(/\n{2,}/)
      .map((block) => {
        const lines = block.split("\n");
        if (lines.every((l) => /^[-*]\s+/.test(l))) {
          return `<ul>${lines.map((l) => `<li>${l.replace(/^[-*]\s+/, "")}</li>`).join("")}</ul>`;
        }
        return `<p>${block.replace(/\n/g, "<br>")}</p>`;
      })
      .join("");
    html = html.replace(/\u0000FENCE(\d+)\u0000/g, (_, i) => fences[Number(i)]);
    return html;
  }

  function resizeChatInput() {
    const box = el.chatInput;
    box.style.height = "auto";
    box.style.height = Math.min(box.scrollHeight, 160) + "px";
  }

  function scrollChat() {
    el.chatThread.scrollTop = el.chatThread.scrollHeight;
  }

  function updateChatControls() {
    const hasModel = Boolean(el.chatModel.value);
    const hasText = el.chatInput.value.trim().length > 0;
    el.chatSend.disabled = state.chat.busy ? false : !hasModel || !hasText;
    el.chatSend.classList.toggle("is-stop", state.chat.busy);
    el.chatSend.setAttribute("aria-label", state.chat.busy ? "Stop" : "Send");
    el.chatSend.innerHTML = state.chat.busy
      ? `<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="7" y="7" width="10" height="10" rx="1.5" fill="currentColor"/></svg>`
      : `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" aria-hidden="true"><path d="M12 19V5M6 11l6-6 6 6"/></svg>`;
    el.chatModel.disabled = state.chat.busy;
  }

  const MESH_MODEL_PREFIX = "mesh/";

  function bareModelName(value) {
    const n = String(value || "");
    return n.startsWith(MESH_MODEL_PREFIX) ? n.slice(MESH_MODEL_PREFIX.length) : n;
  }

  function findModel(name) {
    const bare = bareModelName(name);
    return state.models.find((m) => modelName(m) === bare);
  }

  function modelOnThisNode(name) {
    const m = findModel(name);
    return !!(m && (m.providers || []).some((p) => providerIsSelf(p)));
  }

  function chatModelValue(name) {
    return modelOnThisNode(name) ? name : MESH_MODEL_PREFIX + name;
  }

  function updateChatPrivacy() {
    const box = el.chatPrivacy;
    if (!box) return;
    const value = el.chatModel.value;
    const name = bareModelName(value);
    if (!name || modelOnThisNode(name)) {
      box.hidden = true;
      box.classList.add("hidden");
      box.textContent = "";
      return;
    }
    const m = findModel(name);
    const hosts = (m && m.providers ? m.providers : [])
      .map((p) => providerName(p))
      .filter(Boolean);
    const where = hosts.length ? hosts.join(", ") : "another mesh node";
    box.hidden = false;
    box.classList.remove("hidden");
    box.innerHTML = `<strong>Not on this node.</strong> ${escapeHTML(MESH_MODEL_PREFIX + name)} is served by ${escapeHTML(where)}. Prompts and replies travel over the mesh — this conversation is not private.`;
  }

  function syncChatModels() {
    const names = modelNames();
    const values = names.map(chatModelValue);
    const current = el.chatModel.value || state.chat.model;
    el.chatModel.innerHTML = names.length
      ? names
          .map((n) => {
            const value = chatModelValue(n);
            const m = findModel(n);
            const label = m && m.private ? `${value} (private)` : value;
            return `<option value="${escapeHTML(value)}">${escapeHTML(label)}</option>`;
          })
          .join("")
      : `<option value="">No models available</option>`;
    if (current && values.includes(current)) el.chatModel.value = current;
    else if (current && values.includes(chatModelValue(bareModelName(current)))) {
      el.chatModel.value = chatModelValue(bareModelName(current));
    } else if (values.length) el.chatModel.value = values[0];
    state.chat.model = el.chatModel.value;
    updateChatControls();
    updateChatPrivacy();
  }

  function formatDuration(ms) {
    if (ms == null || ms < 0) return "";
    if (ms < 1000) return `${Math.round(ms)}ms`;
    const s = ms / 1000;
    return s < 10 ? `${s.toFixed(1)}s` : `${Math.round(s)}s`;
  }

  function chatMetaHTML(msg) {
    const parts = [];
    if (msg.model) parts.push(escapeHTML(msg.model));
    if (msg.durationMs != null) parts.push(formatDuration(msg.durationMs));
    if (!parts.length) return "";
    return `<p class="chat-meta">${parts.join(" · ")}</p>`;
  }

  function renderChatThread() {
    if (!state.chat.messages.length) {
      el.chatThread.innerHTML = `
        <div class="chat-empty">
          <h2>What can I help with?</h2>
          <p class="muted">Ask a model on this mesh. This conversation stays in memory and is discarded when you start a new chat.</p>
        </div>`;
      return;
    }

    el.chatThread.innerHTML = state.chat.messages
      .map((msg, i) => {
        const last = i === state.chat.messages.length - 1;
        if (msg.role === "user") {
          return `<article class="chat-msg chat-msg-user"><div class="chat-bubble">${escapeHTML(msg.content)}</div></article>`;
        }
        const meta = chatMetaHTML(msg);
        if (msg.error) {
          return `<article class="chat-msg chat-msg-assistant"><p class="chat-error">${escapeHTML(msg.error)}</p>${meta}</article>`;
        }
        if (!msg.content && state.chat.busy && last) {
          return `<article class="chat-msg chat-msg-assistant"><div class="chat-pending" id="chat-stream"><i></i><i></i><i></i></div>${meta}</article>`;
        }
        const streamId = last && state.chat.busy ? ` id="chat-stream"` : "";
        return `<article class="chat-msg chat-msg-assistant"><div class="chat-md"${streamId}>${renderMarkdown(msg.content)}</div>${meta}</article>`;
      })
      .join("");
    scrollChat();
  }

  function patchStream(content) {
    let node = document.getElementById("chat-stream");
    if (!node || node.classList.contains("chat-pending")) {
      renderChatThread();
      node = document.getElementById("chat-stream");
    }
    if (node) {
      node.className = "chat-md";
      node.innerHTML = renderMarkdown(content);
      scrollChat();
    }
  }

  async function readChatResponse(res, onDelta) {
    const ctype = res.headers.get("content-type") || "";
    if (ctype.includes("application/json") && !ctype.includes("event-stream")) {
      const json = await res.json();
      const text = json.choices && json.choices[0] && json.choices[0].message && json.choices[0].message.content;
      if (text) onDelta(text);
      return;
    }
    if (!res.body) throw new Error("empty response");
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buf = "";
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      buf += decoder.decode(value, { stream: true });
      const lines = buf.split(/\r?\n/);
      buf = lines.pop();
      for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed.startsWith("data:")) continue;
        const data = trimmed.slice(5).trim();
        if (!data || data === "[DONE]") continue;
        try {
          const json = JSON.parse(data);
          const delta =
            (json.choices && json.choices[0] && json.choices[0].delta && json.choices[0].delta.content) ||
            (json.choices && json.choices[0] && json.choices[0].message && json.choices[0].message.content) ||
            "";
          if (delta) onDelta(delta);
        } catch (_) {
          /* ignore partial SSE frames */
        }
      }
    }
  }

  async function sendChat(fromComposer) {
    if (state.chat.busy) {
      if (fromComposer && state.chat.abort) state.chat.abort.abort();
      return;
    }
    const text = el.chatInput.value.trim();
    const model = bareModelName(el.chatModel.value);
    if (!text || !model) return;

    el.chatInput.value = "";
    resizeChatInput();
    state.chat.messages.push({ role: "user", content: text });
    state.chat.messages.push({
      role: "assistant",
      content: "",
      model: el.chatModel.value,
    });
    state.chat.busy = true;
    const assistant = state.chat.messages[state.chat.messages.length - 1];
    const started = performance.now();
    renderChatThread();
    updateChatControls();

    const apiMessages = state.chat.messages
      .slice(0, -1)
      .filter((m) => m.role === "user" || (m.role === "assistant" && m.content))
      .map((m) => ({ role: m.role, content: m.content }));

    const ac = new AbortController();
    state.chat.abort = ac;
    try {
      const res = await fetch("/v1/chat/completions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model,
          messages: apiMessages,
          stream: true,
        }),
        signal: ac.signal,
      });
      if (!res.ok) {
        const errText = await res.text();
        throw new Error(errText || res.statusText);
      }
      await readChatResponse(res, (delta) => {
        assistant.content += delta;
        patchStream(assistant.content);
      });
      if (!assistant.content) assistant.content = "(no response)";
      renderChatThread();
    } catch (err) {
      if (err.name === "AbortError") {
        if (!assistant.content) state.chat.messages.pop();
        renderChatThread();
      } else {
        assistant.error = err.message || String(err);
        assistant.content = assistant.content || "";
        renderChatThread();
      }
    } finally {
      assistant.durationMs = performance.now() - started;
      state.chat.busy = false;
      state.chat.abort = null;
      renderChatThread();
      updateChatControls();
      el.chatInput.focus();
    }
  }

  function newChat() {
    if (state.chat.abort) state.chat.abort.abort();
    state.chat.messages = [];
    state.chat.busy = false;
    state.chat.abort = null;
    renderChatThread();
    updateChatControls();
    el.chatInput.focus();
  }

  function proxyOrigin() {
    return window.location.origin;
  }

  function isLocalHost(host) {
    const h = String(host || "").toLowerCase();
    return h === "localhost" || h === "127.0.0.1" || h === "[::1]" || h === "::1";
  }

  function renderWelcome() {
    const origin = proxyOrigin();
    const base = origin + "/v1";
    if (el.welcomeOpenai) el.welcomeOpenai.textContent = base;
    if (el.welcomeModels) el.welcomeModels.textContent = origin + "/v1/models";
    if (el.welcomeChat) el.welcomeChat.textContent = origin + "/v1/chat/completions";
    if (el.welcomeCurl) {
      el.welcomeCurl.textContent = `curl ${origin}/v1/models`;
    }
    if (el.welcomeLocalNote) {
      const local = isLocalHost(window.location.hostname);
      el.welcomeLocalNote.hidden = !local;
      el.welcomeLocalNote.classList.toggle("hidden", !local);
    }
  }

  async function copyWelcomeURL(targetId, btn) {
    const node = document.getElementById(targetId);
    const text = node ? node.textContent.trim() : "";
    if (!text) return;
    const label = btn.textContent;
    try {
      await navigator.clipboard.writeText(text);
      btn.textContent = "Copied";
    } catch (_) {
      btn.textContent = "Copy failed";
    }
    setTimeout(() => {
      btn.textContent = label;
    }, 1200);
  }

  async function renderSettings() {
    if (!el.settingsYaml) return;
    try {
      const data = await getJSON("/api/mesh/config");
      if (data.path && el.settingsSubtitle) {
        el.settingsSubtitle.textContent = data.path;
      }
      el.settingsYaml.textContent = data.content || "";
    } catch (err) {
      el.settingsYaml.textContent = "Could not load config.yaml\n" + (err.message || err);
    }
  }

  function setAdminVisible(on) {
    state.adminEnabled = !!on;
    const nav = el.navAdmin || document.getElementById("nav-admin");
    if (nav) {
      nav.hidden = !on;
      nav.classList.toggle("hidden", !on);
      nav.setAttribute("aria-hidden", on ? "false" : "true");
    }
    if (!on && state.view === "admin") showView("welcome");
  }

  function formatInviteExpiry(unix) {
    const n = Number(unix) || 0;
    if (n <= 0) return "Never";
    const d = new Date(n * 1000);
    if (Number.isNaN(d.getTime())) return "Never";
    return d.toLocaleString();
  }

  function setErrorEl(node, msg) {
    if (!node) return;
    if (!msg) {
      node.hidden = true;
      node.classList.add("hidden");
      node.textContent = "";
      return;
    }
    node.hidden = false;
    node.classList.remove("hidden");
    node.textContent = msg;
  }

  function setAdminError(msg) {
    setErrorEl(el.adminError, msg);
  }

  function setInviteModalError(msg) {
    setErrorEl(el.adminModalError, msg);
  }

  function inviteModalOpen() {
    return el.adminModal && !el.adminModal.hidden && !el.adminModal.classList.contains("hidden");
  }

  function openInviteModal() {
    if (!state.adminEnabled) return;
    if (el.adminForm) el.adminForm.reset();
    setInviteModalError("");
    if (el.adminModal) {
      el.adminModal.hidden = false;
      el.adminModal.classList.remove("hidden");
    }
    if (el.adminName) el.adminName.focus();
  }

  function closeInviteModal() {
    if (el.adminModal) {
      el.adminModal.hidden = true;
      el.adminModal.classList.add("hidden");
    }
    setInviteModalError("");
  }

  function tailLink(link) {
    const s = String(link || "");
    if (s.length <= 16) return s;
    return "…" + s.slice(-16);
  }

  function copyIconSVG() {
    return `<svg class="copy-icon" viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>`;
  }

  function inviteLinkCell(link, copyTitle) {
    const full = String(link || "");
    const title = copyTitle || "Copy invite link";
    return `<span class="invite-link-cell"><code>${escapeHTML(tailLink(full))}</code><button type="button" class="btn btn-ghost btn-copy" data-copy-link="${escapeHTML(full)}" title="${escapeHTML(title)}" aria-label="${escapeHTML(title)}">${copyIconSVG()}</button></span>`;
  }

  function fallbackCopy(text) {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.top = "0";
    ta.style.left = "0";
    ta.style.width = "1px";
    ta.style.height = "1px";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    ta.setSelectionRange(0, ta.value.length);
    let ok = false;
    try {
      ok = document.execCommand("copy");
    } catch (_) {}
    document.body.removeChild(ta);
    return ok;
  }

  function markCopied(btn, ok) {
    if (!btn) return;
    const label = btn.getAttribute("data-copy-label") || btn.getAttribute("title") || "Copy";
    btn.setAttribute("data-copy-label", label);
    btn.setAttribute("title", ok ? "Copied" : "Copy failed");
    btn.setAttribute("aria-label", ok ? "Copied" : "Copy failed");
    btn.classList.toggle("is-copied", ok);
    setTimeout(() => {
      btn.setAttribute("title", label);
      btn.setAttribute("aria-label", label);
      btn.classList.remove("is-copied");
    }, 1200);
  }

  function copyToClipboard(text, btn) {
    const value = String(text || "");
    if (!value) {
      markCopied(btn, false);
      return;
    }
    const clip = navigator.clipboard;
    if (clip && typeof clip.writeText === "function") {
      clip.writeText(value).then(() => markCopied(btn, true)).catch(() => {
        markCopied(btn, fallbackCopy(value));
      });
      return;
    }
    markCopied(btn, fallbackCopy(value));
  }

  function showCreatedInvite(link) {
    if (!el.adminCreated) return;
    if (!link) {
      el.adminCreated.hidden = true;
      el.adminCreated.classList.add("hidden");
      return;
    }
    el.adminCreated.hidden = false;
    el.adminCreated.classList.remove("hidden");
    if (el.adminCreatedLink) el.adminCreatedLink.textContent = tailLink(link);
    if (el.adminCopy) el.adminCopy.setAttribute("data-copy-link", link);
  }

  function nodeID(n) {
    return n.ID || n.Id || n.id || "";
  }

  function nodeName(n) {
    return n.Name || n.name || "—";
  }

  function formatNodeUpdated(n) {
    const raw = n.AddedAt || n.addedAt || n.LastUpdate || n.lastUpdate || n.last_update || "";
    if (!raw) return "—";
    const d = new Date(raw);
    if (Number.isNaN(d.getTime()) || d.getTime() === 0) return "—";
    return d.toLocaleString();
  }

  function nodeMesh(n) {
    return n.MeshId || n.meshId || n.mesh_id || "—";
  }

  function nodeInvitedAs(n) {
    return n.InvitedAs || n.invitedAs || n.invited_as || "—";
  }

  function renderAdminNodes() {
    if (el.adminCount) el.adminCount.textContent = String(state.adminNodes.length);
    if (!el.adminNodesBody) return;
    if (!state.adminNodes.length) {
      el.adminNodesBody.innerHTML = `<tr><td colspan="6" class="empty">No nodes registered on the mesh.</td></tr>`;
      return;
    }
    el.adminNodesBody.innerHTML = state.adminNodes.map((n) => {
      const id = nodeID(n);
      return `<tr>
        <td>${escapeHTML(nodeName(n))}</td>
        <td>${inviteLinkCell(id, "Copy peer ID")}</td>
        <td>${escapeHTML(nodeMesh(n))}</td>
        <td>${escapeHTML(nodeInvitedAs(n))}</td>
        <td>${escapeHTML(formatNodeUpdated(n))}</td>
        <td><button type="button" class="btn btn-danger btn-sm" data-kick-node="${escapeHTML(id)}">Kick</button></td>
      </tr>`;
    }).join("");
  }

  function renderAdminInvites() {
    if (!el.adminInvitesBody) return;
    if (!state.invites.length) {
      el.adminInvitesBody.innerHTML = `<tr><td colspan="5" class="empty">No invites. Create one to add a node to the mesh.</td></tr>`;
      return;
    }
    el.adminInvitesBody.innerHTML = state.invites.map((inv) => {
      const id = escapeHTML(inv.InviteId || inv.inviteId || "");
      const link = inv.InviteLink || inv.inviteLink || "";
      const name = escapeHTML(inv.Name || inv.name || "—");
      const once = inv.OneTime || inv.oneTime ? "One-time" : "Reusable";
      const expires = escapeHTML(formatInviteExpiry(inv.Expires || inv.expires));
      return `<tr>
        <td>${name}</td>
        <td>${inviteLinkCell(link)}</td>
        <td>${expires}</td>
        <td>${once}</td>
        <td><button type="button" class="btn btn-danger btn-sm" data-revoke-invite="${id}">Revoke</button></td>
      </tr>`;
    }).join("");
  }

  function renderAdmin() {
    renderAdminNodes();
    renderAdminInvites();
  }

  async function loadInvites() {
    if (!state.adminEnabled) return;
    const data = await getJSON("/api/admin/invite");
    state.invites = data.Invites || data.invites || [];
    if (state.view === "admin") renderAdminInvites();
  }

  async function loadAdminNodes() {
    if (!state.adminEnabled) return;
    const data = await getJSON("/api/admin/node");
    state.adminNodes = data.Nodes || data.nodes || [];
    if (el.adminCount) el.adminCount.textContent = String(state.adminNodes.length);
    if (state.view === "admin") renderAdminNodes();
  }

  async function checkAdmin() {
    try {
      const data = await getJSON("/api/admin/enabled");
      const on = !!(data && (data.enabled || data.Enabled));
      setAdminVisible(on);
    } catch (_) {
      setAdminVisible(false);
      return;
    }
    if (!state.adminEnabled) return;
    const tasks = [
      loadInvites().catch((err) => {
        state.invites = [];
        setAdminError(err.message || String(err));
      }),
      loadAdminNodes().catch((err) => {
        state.adminNodes = [];
        setErrorEl(el.adminNodesError, err.message || String(err));
      }),
    ];
    setAdminError("");
    setErrorEl(el.adminNodesError, "");
    await Promise.all(tasks);
    if (state.view === "admin") renderAdmin();
  }

  async function createInvite(e) {
    e.preventDefault();
    if (!state.adminEnabled) return;
    setInviteModalError("");
    const lifetime = Number(el.adminLifetime && el.adminLifetime.value) || 0;
    const submit = document.getElementById("admin-invite-create");
    if (submit) submit.disabled = true;
    try {
      const created = await sendJSON("/api/admin/invite", "POST", {
        Name: (el.adminName && el.adminName.value.trim()) || "",
        LifetimeSec: lifetime,
        OneTime: !!(el.adminOnce && el.adminOnce.checked),
        MeshId: "default",
      });
      state.createdInvite = created;
      showCreatedInvite(created.InviteLink || created.inviteLink || "");
      closeInviteModal();
      await loadInvites();
    } catch (err) {
      setInviteModalError(err.message || String(err));
    } finally {
      if (submit) submit.disabled = false;
    }
  }

  async function revokeInvite(id) {
    if (!id || !state.adminEnabled) return;
    if (!window.confirm("Revoke this invite? It will no longer work.")) return;
    setAdminError("");
    try {
      await sendJSON("/api/admin/invite/" + encodeURIComponent(id), "DELETE");
      if (state.createdInvite && (state.createdInvite.InviteId === id || state.createdInvite.inviteId === id)) {
        state.createdInvite = null;
        showCreatedInvite("");
      }
      await loadInvites();
    } catch (err) {
      setAdminError(err.message || String(err));
    }
  }

  async function kickNode(id) {
    if (!id || !state.adminEnabled) return;
    if (!window.confirm("Kick this node from the mesh? It will need a new invite to rejoin.")) return;
    setErrorEl(el.adminNodesError, "");
    try {
      await sendJSON("/api/admin/node/" + encodeURIComponent(id), "DELETE");
      await loadAdminNodes();
    } catch (err) {
      setErrorEl(el.adminNodesError, err.message || String(err));
    }
  }

  function render() {
    if (state.view === "welcome") renderWelcome();
    else if (state.view === "mesh") renderMesh();
    else if (state.view === "nodes") renderNodes();
    else if (state.view === "models") renderModels();
    else if (state.view === "chat") syncChatModels();
    else if (state.view === "admin") renderAdmin();
    else if (state.view === "settings") renderSettings();
  }

  function showView(name) {
    state.view = name;
    document.querySelectorAll(".sidebar-item").forEach((btn) => {
      btn.classList.toggle("is-active", btn.dataset.view === name);
    });
    document.querySelectorAll(".view").forEach((view) => {
      view.classList.toggle("active", view.dataset.view === name);
    });
    render();
    if (name === "chat") {
      renderChatThread();
      el.chatInput.focus();
    }
  }

  async function refresh() {
    try {
      const [members, models] = await Promise.all([
        getJSON("/api/mesh/members"),
        getJSON("/api/mesh/models"),
      ]);
      state.members = members.Nodes || members.nodes || [];
      state.models = models.models || models.Models || [];
      state.error = null;
      el.meshCount.textContent = String(state.members.length);
      el.nodesCount.textContent = String(state.members.length);
      el.modelsCount.textContent = String(state.models.length);
      const up = state.members.filter((m) => m.Reachable).length;
      setStatus("online", `${up}/${state.members.length} reachable`);
      syncChatModels();
    } catch (err) {
      state.error = err;
      setStatus("offline", "API unavailable");
      console.error(err);
    }
    await checkAdmin();
    render();
  }

  let refreshTimer = null;
  function scheduleRefresh() {
    if (refreshTimer) return;
    refreshTimer = setTimeout(() => {
      refreshTimer = null;
      refresh();
    }, 50);
  }

  function refreshSocketURL() {
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    return proto + "//" + window.location.host + REFRESH_WS_PATH;
  }

  function connectRefreshSocket() {
    let delay = 1000;
    function connect() {
      let socket;
      try {
        socket = new WebSocket(refreshSocketURL());
      } catch (err) {
        console.error(err);
        setTimeout(connect, delay);
        delay = Math.min(delay * 2, 15000);
        return;
      }
      socket.addEventListener("open", () => {
        delay = 1000;
      });
      socket.addEventListener("message", () => {
        scheduleRefresh();
      });
      socket.addEventListener("close", () => {
        setStatus("loading", "Reconnecting");
        setTimeout(connect, delay);
        delay = Math.min(delay * 2, 15000);
      });
    }
    connect();
  }

  document.querySelectorAll(".sidebar-item").forEach((btn) => {
    btn.addEventListener("click", () => showView(btn.dataset.view));
  });
  document.getElementById("view-welcome").addEventListener("click", (e) => {
    const btn = e.target.closest(".welcome-copy");
    if (!btn) return;
    copyWelcomeURL(btn.getAttribute("data-copy-target"), btn);
  });
  document.getElementById("btn-refresh").addEventListener("click", () => {
    setStatus("loading", "Refreshing");
    refresh();
  });
  if (el.adminOpen) {
    el.adminOpen.addEventListener("click", () => openInviteModal());
  }
  if (el.adminForm) {
    el.adminForm.addEventListener("submit", createInvite);
  }
  if (el.adminModal) {
    el.adminModal.addEventListener("click", (e) => {
      if (e.target.closest("[data-close-invite-modal]")) closeInviteModal();
    });
  }
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && inviteModalOpen()) closeInviteModal();
  });
  if (el.adminCopy) {
    el.adminCopy.addEventListener("click", () => {
      copyToClipboard(el.adminCopy.getAttribute("data-copy-link"), el.adminCopy);
    });
  }
  if (el.adminInvitesBody) {
    el.adminInvitesBody.addEventListener("click", (e) => {
      const copyBtn = e.target.closest("[data-copy-link]");
      if (copyBtn) {
        copyToClipboard(copyBtn.getAttribute("data-copy-link"), copyBtn);
        return;
      }
      const btn = e.target.closest("[data-revoke-invite]");
      if (!btn) return;
      revokeInvite(btn.getAttribute("data-revoke-invite"));
    });
  }
  if (el.adminNodesBody) {
    el.adminNodesBody.addEventListener("click", (e) => {
      const copyBtn = e.target.closest("[data-copy-link]");
      if (copyBtn) {
        copyToClipboard(copyBtn.getAttribute("data-copy-link"), copyBtn);
        return;
      }
      const btn = e.target.closest("[data-kick-node]");
      if (!btn) return;
      kickNode(btn.getAttribute("data-kick-node"));
    });
  }
  el.search.addEventListener("input", () => {
    state.filter = el.search.value;
    if (state.view === "models") renderModels();
  });
  el.modelsBody.addEventListener("click", (e) => {
    const row = e.target.closest("tr.node-row");
    if (!row) return;
    const name = row.getAttribute("data-model-name");
    state.expandedModel = state.expandedModel === name ? null : name;
    renderModels();
  });
  el.nodesSearch.addEventListener("input", () => {
    state.nodesFilter = el.nodesSearch.value;
    if (state.view === "nodes") renderNodes();
  });
  el.chatForm.addEventListener("submit", (e) => {
    e.preventDefault();
    sendChat(true);
  });
  el.chatInput.addEventListener("input", () => {
    resizeChatInput();
    updateChatControls();
  });
  el.chatInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      if (!state.chat.busy) sendChat(false);
    }
  });
  el.chatModel.addEventListener("change", () => {
    state.chat.model = el.chatModel.value;
    updateChatControls();
    updateChatPrivacy();
  });
  el.btnNewChat.addEventListener("click", () => newChat());
  el.nodesBody.addEventListener("click", (e) => {
    const row = e.target.closest("tr.node-row");
    if (!row) return;
    const id = row.getAttribute("data-peer-id");
    state.expandedPeer = state.expandedPeer === id ? null : id;
    renderNodes();
  });
  el.meshCanvas.addEventListener("click", (e) => {
    if (e.target.closest("[data-close-detail]")) {
      state.selectedPeer = null;
      renderMesh();
      return;
    }
    if (e.target.closest(".node-detail")) return;
    const node = e.target.closest("[data-peer-id]");
    if (node) {
      const id = node.getAttribute("data-peer-id");
      state.selectedPeer = state.selectedPeer === id ? null : id;
      renderMesh();
      return;
    }
    if (state.selectedPeer) {
      state.selectedPeer = null;
      renderMesh();
    }
  });

  if (window.ResizeObserver) {
    const ro = new ResizeObserver((entries) => {
      if (state.view !== "mesh") return;
      const box = entries[0] && entries[0].contentRect;
      if (!box) return;
      const w = Math.round(box.width);
      const h = Math.round(box.height);
      if (w === state.meshSize.w && h === state.meshSize.h) return;
      renderMesh();
    });
    ro.observe(el.meshCanvas);
  } else {
    window.addEventListener("resize", () => {
      if (state.view === "mesh") renderMesh();
    });
  }

  renderWelcome();
  renderChatThread();
  updateChatControls();
  refresh();
  connectRefreshSocket();
})();
