// app.js — Prompt Studio v2 Frontend Controller (Studio & Unified Library)
// Zero inline styles; strictly toggles BEM & semantic state classes.

(function () {
  const VAR_REGEX = /\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}/g;
  const USER_SEP = "===USER===";

  const state = {
    view: window.location.pathname === "/library" ? "library" : "studio", // "studio" | "library"
    settings: null,
    aiConnected: null, // null = unknown (settings failed to load) → do not block
    starters: [],
    showAllStarters: false,
    selectedStarterId: "",

    // Studio active session
    session: null,
    versions: [],
    activeVersionIndex: -1,
    streaming: false,
    streamText: "",
    abortController: null,
    openAssumptionIndex: -1,
    targetManualOverride: false,
    suggestedTarget: "",

    // History & Undo delete
    historySessions: [],
    historyHasMore: false,
    pendingDelete: null, // { id, timerId }

    // Library & Collections
    collections: [],
    totalSaved: 0,
    activeCollectionId: "all",
    librarySessions: [],
    libraryHasMore: false,
    activeLibraryDetail: null,
    activeLibraryVersionId: "",
    libRenaming: false,
    duplicateSessionId: "",
    pasteTitleTouched: false,

    // Per-session variable values shared between Studio and Library (D-P010-1)
    sessionVars: {},
    studioVarsOpen: false,
    libVarsOpen: false,

    // Inline Version Diff (D-P010-2)
    diffOpen: false,
    diffBaseVersionId: "",

    // Last generate/refine call, re-run by the error banner's "Thử lại".
    retryAction: null,
  };

  const el = (id) => document.getElementById(id);

  function show(node, visible) {
    if (!node) return;
    node.classList.toggle("is-hidden", !visible);
  }

  function escapeHTML(str) {
    return String(str || "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function extractVariables(text) {
    const out = [];
    const seen = new Set();
    let m;
    VAR_REGEX.lastIndex = 0;
    while ((m = VAR_REGEX.exec(text || "")) !== null) {
      if (!seen.has(m[1])) {
        seen.add(m[1]);
        out.push(m[1]);
      }
    }
    return out;
  }

  function getSessionVarMap(sessionId) {
    if (!sessionId) return {};
    if (!state.sessionVars[sessionId]) {
      state.sessionVars[sessionId] = {};
    }
    return state.sessionVars[sessionId];
  }

  function fillPromptVariables(text, varMap) {
    const map = varMap || {};
    return String(text || "").replace(/\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}/g, (full, name) => {
      const val = map[name];
      return typeof val === "string" && val.trim() ? val.trim() : full;
    });
  }

  function highlightInlineVars(escapedHtml) {
    return escapedHtml.replace(/\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}/g, (_, name) => {
      return `<span class="var-pill">{{${name}}}</span>`;
    });
  }

  function formatInlineMarkdown(rawLine) {
    const codeSpans = [];
    let s = escapeHTML(rawLine).replace(/`([^`]+)`/g, (_, code) => {
      const token = `\u0000CODE${codeSpans.length}\u0000`;
      codeSpans.push(`<code>${highlightInlineVars(code)}</code>`);
      return token;
    });
    s = s.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
    s = s.replace(/(^|[^*])\*([^*\n]+)\*(?!\*)/g, "$1<em>$2</em>");
    s = highlightInlineVars(s);
    return s.replace(/\u0000CODE(\d+)\u0000/g, (_, idx) => codeSpans[Number(idx)] || "");
  }

  function splitTableRow(rawRow) {
    let trimmed = rawRow.trim();
    if (trimmed.startsWith("|")) trimmed = trimmed.slice(1);
    if (trimmed.endsWith("|")) trimmed = trimmed.slice(0, -1);
    return trimmed.split("|").map((c) => c.trim());
  }

  function isTableSeparatorRow(rawRow) {
    return /^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)+\|?\s*$/.test(rawRow || "");
  }

  function renderMarkdownToHTML(markdown, withCaret) {
    const raw = String(markdown || "");
    if (!raw.trim()) {
      return withCaret ? `<p><span class="ai-caret" aria-hidden="true"></span></p>` : "";
    }
    const lines = raw.split("\n");
    const blocks = [];
    let inCode = false;
    let codeLines = [];
    let listTag = "";
    let listItems = [];
    let quoteLines = [];

    const flushList = () => {
      if (listItems.length > 0) {
        blocks.push(`<${listTag}>${listItems.join("")}</${listTag}>`);
        listItems = [];
        listTag = "";
      }
    };

    const flushQuote = () => {
      if (quoteLines.length > 0) {
        blocks.push(`<blockquote><p>${quoteLines.map(formatInlineMarkdown).join("\n")}</p></blockquote>`);
        quoteLines = [];
      }
    };

    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      const trimmed = line.trim();
      if (trimmed.startsWith("```")) {
        flushList();
        flushQuote();
        if (inCode) {
          blocks.push(`<pre>${highlightInlineVars(escapeHTML(codeLines.join("\n")))}</pre>`);
          codeLines = [];
          inCode = false;
        } else {
          inCode = true;
        }
        continue;
      }
      if (inCode) {
        codeLines.push(line);
        continue;
      }
      if (!trimmed) {
        flushList();
        flushQuote();
        continue;
      }
      if (/^(?:---+|\*\*\*+|___+)$/.test(trimmed)) {
        flushList();
        flushQuote();
        blocks.push("<hr>");
        continue;
      }
      const quoteMatch = trimmed.match(/^>\s?(.*)$/);
      if (quoteMatch) {
        flushList();
        quoteLines.push(quoteMatch[1]);
        continue;
      }
      flushQuote();

      // GFM pipe table: header row with "|" followed by separator row "| --- | --- |"
      if (trimmed.includes("|") && i + 1 < lines.length && isTableSeparatorRow(lines[i + 1])) {
        flushList();
        const headers = splitTableRow(trimmed);
        i += 2;
        const bodyRows = [];
        while (i < lines.length && lines[i].trim() && lines[i].includes("|")) {
          bodyRows.push(splitTableRow(lines[i]));
          i++;
        }
        i--; // step back so outer loop increments naturally
        const ths = headers.map((h) => `<th>${formatInlineMarkdown(h)}</th>`).join("");
        const trs = bodyRows
          .map((row) => {
            const cells = headers
              .map((_, colIdx) => `<td>${formatInlineMarkdown(row[colIdx] || "")}</td>`)
              .join("");
            return `<tr>${cells}</tr>`;
          })
          .join("");
        blocks.push(
          `<div class="table-wrap"><table class="table table--compact"><thead><tr>${ths}</tr></thead><tbody>${trs}</tbody></table></div>`
        );
        continue;
      }

      const headingMatch = trimmed.match(/^(#{1,4})\s+(.+)$/);
      if (headingMatch) {
        flushList();
        const level = Math.min(4, Math.max(1, headingMatch[1].length));
        blocks.push(`<h${level}>${formatInlineMarkdown(headingMatch[2])}</h${level}>`);
        continue;
      }
      const ulMatch = trimmed.match(/^[-*]\s+(.+)$/);
      if (ulMatch) {
        if (listTag && listTag !== "ul") flushList();
        listTag = "ul";
        listItems.push(`<li>${formatInlineMarkdown(ulMatch[1])}</li>`);
        continue;
      }
      const olMatch = trimmed.match(/^\d+\.\s+(.+)$/);
      if (olMatch) {
        if (listTag && listTag !== "ol") flushList();
        listTag = "ol";
        listItems.push(`<li>${formatInlineMarkdown(olMatch[1])}</li>`);
        continue;
      }
      flushList();
      blocks.push(`<p>${formatInlineMarkdown(line)}</p>`);
    }
    flushList();
    flushQuote();
    if (inCode && codeLines.length > 0) {
      blocks.push(`<pre>${highlightInlineVars(escapeHTML(codeLines.join("\n")))}</pre>`);
    }

    if (withCaret) {
      if (blocks.length > 0 && blocks[blocks.length - 1].endsWith("</p>")) {
        blocks[blocks.length - 1] =
          blocks[blocks.length - 1].slice(0, -4) + `<span class="ai-caret" aria-hidden="true"></span></p>`;
      } else {
        blocks.push(`<p><span class="ai-caret" aria-hidden="true"></span></p>`);
      }
    }
    return blocks.join("");
  }

  function splitAPIPrompt(content) {
    const idx = String(content || "").indexOf(USER_SEP);
    if (idx < 0) {
      return { hasSplit: false, system: String(content || "").trim(), user: "" };
    }
    return {
      hasSplit: true,
      system: content.slice(0, idx).trim(),
      user: content.slice(idx + USER_SEP.length).trim(),
    };
  }

  // Renders either two-part API blocks (system + user) or single Markdown into `container`.
  function renderPromptContent(container, content, apiSplit, singleClass = "prompt-markdown") {
    if (!container) return;
    if (apiSplit && apiSplit.hasSplit) {
      container.className = "api-blocks";
      container.innerHTML = `
        <div class="api-block">
          <div class="api-block__header">
            <span class="api-block__role api-block__role--system">system</span>
            <span class="api-block__hint">Cố định, đặt một lần trong code</span>
            <button type="button" class="btn btn--secondary btn--sm" data-copy-part="system">Copy</button>
          </div>
          <div class="api-block__body prompt-markdown">${renderMarkdownToHTML(apiSplit.system, false)}</div>
        </div>
        <div class="api-block">
          <div class="api-block__header">
            <span class="api-block__role api-block__role--user">user</span>
            <span class="api-block__hint">Mẫu, điền biến ở mỗi lần gọi</span>
            <button type="button" class="btn btn--secondary btn--sm" data-copy-part="user">Copy</button>
          </div>
          <div class="api-block__body api-block__body--mono">${highlightInlineVars(escapeHTML(apiSplit.user))}</div>
        </div>
      `;
      container.querySelectorAll("[data-copy-part]").forEach((btn) => {
        btn.addEventListener("click", () => {
          const part = btn.getAttribute("data-copy-part");
          copyTextWithFeedback(btn, part === "system" ? apiSplit.system : apiSplit.user);
        });
      });
    } else {
      container.className = singleClass;
      container.innerHTML = renderMarkdownToHTML(content, false);
    }
  }

  // Computes line-by-line LCS diff between oldText and newText (with O(N*M) cap for huge pastes).
  function computeLineDiff(oldText, newText) {
    const a = String(oldText || "").split("\n");
    const b = String(newText || "").split("\n");
    const n = a.length;
    const m = b.length;

    if (n * m > 250000) {
      let pre = 0;
      while (pre < n && pre < m && a[pre] === b[pre]) pre++;
      let suf = 0;
      while (suf < n - pre && suf < m - pre && a[n - 1 - suf] === b[m - 1 - suf]) suf++;
      const rows = [];
      for (let i = 0; i < pre; i++) rows.push({ type: "ctx", text: a[i] });
      let removed = 0;
      let added = 0;
      for (let i = pre; i < n - suf; i++) {
        rows.push({ type: "del", text: a[i] });
        removed++;
      }
      for (let j = pre; j < m - suf; j++) {
        rows.push({ type: "add", text: b[j] });
        added++;
      }
      for (let i = n - suf; i < n; i++) rows.push({ type: "ctx", text: a[i] });
      return { rows, added, removed };
    }

    const dp = Array.from({ length: n + 1 }, () => new Int32Array(m + 1));
    for (let i = n - 1; i >= 0; i--) {
      for (let j = m - 1; j >= 0; j--) {
        if (a[i] === b[j]) {
          dp[i][j] = dp[i + 1][j + 1] + 1;
        } else {
          dp[i][j] = dp[i + 1][j] >= dp[i][j + 1] ? dp[i + 1][j] : dp[i][j + 1];
        }
      }
    }

    const rows = [];
    let added = 0;
    let removed = 0;
    let i = 0;
    let j = 0;
    while (i < n && j < m) {
      if (a[i] === b[j]) {
        rows.push({ type: "ctx", text: a[i] });
        i++;
        j++;
      } else if (dp[i + 1][j] >= dp[i][j + 1]) {
        rows.push({ type: "del", text: a[i] });
        removed++;
        i++;
      } else {
        rows.push({ type: "add", text: b[j] });
        added++;
        j++;
      }
    }
    while (i < n) {
      rows.push({ type: "del", text: a[i++] });
      removed++;
    }
    while (j < m) {
      rows.push({ type: "add", text: b[j++] });
      added++;
    }
    return { rows, added, removed };
  }

  function formatRelativeTime(tsMs) {
    if (!tsMs) return "";
    const now = Date.now();
    const diffSec = Math.floor((now - tsMs) / 1000);
    if (diffSec < 60) return "Vừa xong";
    const diffMin = Math.floor(diffSec / 60);
    if (diffMin < 60) return `${diffMin} phút trước`;
    const diffHr = Math.floor(diffMin / 60);
    if (diffHr < 24) return "Hôm nay";
    const diffDay = Math.floor(diffHr / 24);
    if (diffDay === 1) return "Hôm qua";
    if (diffDay < 7) return `${diffDay} ngày trước`;
    return new Date(tsMs).toLocaleDateString("vi-VN");
  }

  function targetLabel(target) {
    if (target === "api") return "System prompt API";
    if (target === "agent") return "Agent lập trình";
    return "Chat";
  }

  function sourceLabel(ver, versions = []) {
    if (!ver) return "Tạo mới";
    let base = "Tạo từ yêu cầu";
    switch (ver.source) {
      case "regenerate":
        base = "Tạo lại";
        break;
      case "improve":
        if (ver.instruction) {
          const short = ver.instruction.length > 42 ? ver.instruction.slice(0, 42) + "…" : ver.instruction;
          base = `Tinh chỉnh: ${short}`;
        } else {
          base = "Tinh chỉnh";
        }
        break;
      case "assumption":
        if (ver.instruction) {
          const short = ver.instruction.length > 42 ? ver.instruction.slice(0, 42) + "…" : ver.instruction;
          base = short;
        } else {
          base = "Đổi giả định";
        }
        break;
      case "manual":
        base = "Sửa tay";
        break;
      case "pasted":
        base = "Đã dán";
        break;
    }
    if (ver.parentId && Array.isArray(versions) && versions.length > 0) {
      const parent = versions.find((v) => v.id === ver.parentId);
      if (parent) {
        base += ` · từ v${parent.seq}`;
      }
    }
    return base;
  }

  function deriveFirstLineTitle(content) {
    const lines = String(content || "").split("\n");
    for (const raw of lines) {
      const clean = raw.trim().replace(/^[#*\-\s]+/, "").trim();
      if (clean && clean !== USER_SEP) {
        return clean.length > 68 ? clean.slice(0, 68) + "…" : clean;
      }
    }
    return "";
  }

  async function copyTextWithFeedback(btn, text, copiedLabel = "Đã copy") {
    try {
      await navigator.clipboard.writeText(text);
    } catch (_) {
      const ta = document.createElement("textarea");
      ta.value = text;
      document.body.appendChild(ta);
      ta.select();
      document.execCommand("copy");
      document.body.removeChild(ta);
    }
    UI.announce(copiedLabel);
    // Text-only buttons swap their label briefly; menu items rely on the announcement.
    if (btn && btn.classList.contains("btn") && !btn.querySelector("svg")) {
      const prev = btn.textContent;
      btn.textContent = copiedLabel;
      setTimeout(() => {
        btn.textContent = prev;
      }, 1500);
    }
  }

  // --- Initialization & Routing ---
  async function init() {
    bindEvents();
    await Promise.all([loadSettings(), loadStarters(), loadCollections()]);

    const urlParams = new URLSearchParams(window.location.search);
    const openSessionId = urlParams.get("session");
    if (openSessionId) {
      await openSessionInStudio(openSessionId);
    } else if (state.view === "library") {
      await switchView("library", false);
    } else {
      await switchView("studio", false);
    }
    await loadHistorySessions();
  }

  async function loadSettings() {
    try {
      const s = await API.request("GET", "/api/settings");
      state.settings = s;
      applyThemePreference(s.theme || "dark", s.accent || "crimson");
      if (s.defaultTarget && !state.session) {
        el("targetSelect").value = s.defaultTarget;
      }
      renderConnectionStatus();
    } catch (_) {}
  }

  function renderConnectionStatus() {
    const s = state.settings;
    if (!s) return;
    const connected = Boolean(s.mockMode || s.hasKey || s.provider === "openai-compatible" || s.provider === "mock");
    state.aiConnected = connected;
    el("generateBtn").setAttribute("aria-disabled", connected ? "false" : "true");
    const dot = el("sidebarAiDot");
    dot.className = "status-dot " + (connected ? "status-dot--success" : "status-dot--warning");
    el("sidebarAiTitle").textContent = connected ? "Đã kết nối AI" : "Chưa kết nối AI";
    const provLabel = s.mockMode
      ? "Mock (--mock)"
      : s.provider === "anthropic"
        ? "Anthropic"
        : s.provider === "openai-compatible"
          ? "OpenAI-compatible"
          : s.provider === "mock"
            ? "Mock"
            : "OpenAI";
    const modelLabel = s.mockMode ? "mock-prompt-engineer-v2" : s.model;
    el("sidebarAiSub").textContent = connected ? `${provLabel} · ${modelLabel}` : "Bấm để thiết lập API key";
    show(el("unconfiguredBanner"), !connected);
  }

  async function loadStarters() {
    try {
      const data = await API.request("GET", "/api/starters");
      state.starters = data.starters || [];
      renderStarters();
    } catch (_) {}
  }

  function renderStarters() {
    const container = el("starterChipsContainer");
    if (!container) return;
    const hasVersions = state.versions && state.versions.length > 0;
    show(el("startersSection"), !hasVersions && !state.streaming);
    // The intro only helps before the first result; afterwards it pushes the prompt down.
    show(el("studioHero"), !hasVersions && !state.streaming);
    if (hasVersions || state.streaming) return;

    container.innerHTML = "";
    const visible = state.showAllStarters ? state.starters : state.starters.slice(0, 6);
    for (const item of visible) {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "starter-chip" + (state.selectedStarterId === item.id ? " is-selected" : "");
      btn.setAttribute("aria-pressed", state.selectedStarterId === item.id ? "true" : "false");
      btn.textContent = item.title;
      btn.addEventListener("click", () => applyStarter(item));
      container.appendChild(btn);
    }

    if (state.starters.length > 6 && !state.showAllStarters) {
      const moreBtn = document.createElement("button");
      moreBtn.type = "button";
      moreBtn.className = "starter-chip starter-chip--more";
      moreBtn.textContent = "Xem thêm";
      moreBtn.addEventListener("click", () => {
        state.showAllStarters = true;
        renderStarters();
      });
      container.appendChild(moreBtn);
    }
  }

  function applyStarter(starter) {
    state.selectedStarterId = starter.id;
    state.targetManualOverride = false;
    const input = el("requestInput");
    input.value = starter.request;

    if (starter.preset && starter.preset.target) {
      el("targetSelect").value = starter.preset.target;
      const badge = el("starterPresetBadge");
      badge.textContent = `Dùng cho: ${targetLabel(starter.preset.target)} (đặt sẵn theo mẫu)`;
      show(badge, true);
    }
    updateRequestCounter();
    renderStarters();

    input.focus();
    if (starter.select) {
      const idx = starter.request.indexOf(starter.select);
      if (idx >= 0) {
        input.setSelectionRange(idx, idx + starter.select.length);
      }
    }
  }

  // --- View Switching ---
  async function switchView(nextView, pushHistory = true) {
    state.view = nextView;

    const isLibrary = nextView === "library";
    el("navStudioBtn").classList.toggle("is-active", !isLibrary);
    el("navLibraryBtn").classList.toggle("is-active", isLibrary);
    for (const [btnId, on] of [["navStudioBtn", !isLibrary], ["navLibraryBtn", isLibrary]]) {
      if (on) el(btnId).setAttribute("aria-current", "page");
      else el(btnId).removeAttribute("aria-current");
    }

    show(el("sidebarStudioControls"), !isLibrary);
    show(el("sidebarStudioScroll"), !isLibrary);
    show(el("sidebarLibraryControls"), isLibrary);
    show(el("sidebarLibraryScroll"), isLibrary);

    show(el("studioView"), nextView === "studio");
    show(el("libraryView"), nextView === "library");

    closeMobileSidebar();

    if (pushHistory) {
      const targetPath = isLibrary ? "/library" : "/";
      if (window.location.pathname !== targetPath) {
        window.history.pushState({}, "", targetPath);
      }
    }

    if (isLibrary) {
      await Promise.all([loadCollections(), loadLibraryList()]);
    } else if (nextView === "studio") {
      await loadHistorySessions();
      renderStudioState();
    }
  }

  // --- History Sidebar & 5-Second Undo Delete ---
  const PAGE_SIZE = 50;

  // append=false reloads the first page; append=true fetches the next page.
  async function loadHistorySessions(append = false) {
    try {
      const q = el("historySearchInput").value.trim();
      const params = new URLSearchParams();
      if (q) params.set("q", q);
      params.set("limit", String(PAGE_SIZE));
      params.set("offset", String(append ? state.historySessions.length : 0));
      const data = await API.request("GET", `/api/sessions?${params.toString()}`);
      const hiddenId = state.pendingDelete ? state.pendingDelete.id : "";
      const page = (data.sessions || []).filter((s) => s.id !== hiddenId);
      if (append) {
        const known = new Set(state.historySessions.map((s) => s.id));
        state.historySessions = state.historySessions.concat(page.filter((s) => !known.has(s.id)));
      } else {
        state.historySessions = page;
      }
      state.historyHasMore = Boolean(data.hasMore);
      renderHistoryList();
    } catch (_) {}
  }

  function loadMoreButton(onClick) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "btn btn--secondary btn--sm";
    btn.textContent = "Tải thêm";
    btn.addEventListener("click", async () => {
      btn.disabled = true;
      btn.textContent = "Đang tải…";
      await onClick();
    });
    return btn;
  }

  function renderHistoryList() {
    const container = el("historyList");
    container.innerHTML = "";
    if (!state.historySessions.length) {
      const empty = document.createElement("div");
      empty.className = "text-xs text-muted";
      empty.textContent = "Chưa có phiên nào.";
      container.appendChild(empty);
      return;
    }

    const now = Date.now();
    const dayMs = 24 * 60 * 60 * 1000;
    const groups = [
      { label: "Hôm nay", items: [] },
      { label: "7 ngày qua", items: [] },
      { label: "Cũ hơn", items: [] },
    ];

    for (const s of state.historySessions) {
      const age = now - (s.openedAt || s.createdAt || now);
      if (age < dayMs) groups[0].items.push(s);
      else if (age < 7 * dayMs) groups[1].items.push(s);
      else groups[2].items.push(s);
    }

    for (const g of groups) {
      if (!g.items.length) continue;
      const lbl = document.createElement("div");
      lbl.className = "nav-group-label";
      lbl.textContent = g.label;
      container.appendChild(lbl);

      for (const s of g.items) {
        const isActive = Boolean(state.session && state.session.id === s.id);
        const row = document.createElement("div");
        row.className = "nav-item nav-item--split" + (isActive ? " is-active" : "");

        const link = document.createElement("button");
        link.type = "button";
        link.className = "nav-item__link";
        if (isActive) link.setAttribute("aria-current", "true");
        link.addEventListener("click", () => openSessionInStudio(s.id));
        const titleSpan = document.createElement("span");
        titleSpan.className = "nav-item__title";
        titleSpan.textContent = s.title || "Phiên mới";
        link.appendChild(titleSpan);
        row.appendChild(link);

        const actions = document.createElement("span");
        actions.className = "nav-item__actions";
        const delBtn = document.createElement("button");
        delBtn.type = "button";
        delBtn.className = "btn btn--ghost btn--icon";
        delBtn.setAttribute("aria-label", `Xóa phiên “${s.title || "Phiên mới"}”`);
        delBtn.innerHTML = UI.icons.close;
        delBtn.addEventListener("click", () => requestDeleteSession(s));
        actions.appendChild(delBtn);
        row.appendChild(actions);

        container.appendChild(row);
      }
    }

    if (state.historyHasMore) {
      container.appendChild(loadMoreButton(() => loadHistorySessions(true)));
    }
  }

  async function requestDeleteSession(sess) {
    // Saved sessions require explicit confirmation; unsaved history sessions get 5-second Undo
    if (sess.savedAt) {
      const ok = await UI.confirm({
        title: "Xóa vĩnh viễn prompt đã lưu?",
        message: `“${sess.title}” sẽ bị xóa khỏi cả Thư viện và Lịch sử, kèm mọi phiên bản. Không thể hoàn tác.`,
        confirmLabel: "Xóa vĩnh viễn",
        danger: true,
      });
      if (!ok) return;
      await commitDeleteSession(sess.id);
      return;
    }

    // Flush any previous pending delete first
    if (state.pendingDelete) {
      clearTimeout(state.pendingDelete.timerId);
      state.pendingDelete.toast.close();
      await commitDeleteSession(state.pendingDelete.id);
      state.pendingDelete = null;
    }

    if (state.session && state.session.id === sess.id) {
      resetStudioToEmpty();
    }

    const timerId = setTimeout(async () => {
      const targetId = state.pendingDelete ? state.pendingDelete.id : "";
      state.pendingDelete = null;
      if (targetId) {
        await commitDeleteSession(targetId);
      }
    }, 5000);

    const toast = UI.toast(`Đã xóa “${sess.title || "Phiên mới"}”.`, {
      actionLabel: "Hoàn tác",
      onAction: undoPendingDelete,
      duration: 5000,
    });
    state.pendingDelete = { id: sess.id, timerId, toast };
    await loadHistorySessions();
  }

  async function undoPendingDelete() {
    if (!state.pendingDelete) return;
    clearTimeout(state.pendingDelete.timerId);
    state.pendingDelete.toast.close();
    const restoredId = state.pendingDelete.id;
    state.pendingDelete = null;
    await loadHistorySessions();
    await openSessionInStudio(restoredId);
  }

  async function commitDeleteSession(id) {
    try {
      await API.request("DELETE", `/api/sessions/${id}`);
      if (state.session && state.session.id === id) {
        resetStudioToEmpty();
      }
      if (state.activeLibraryDetail && state.activeLibraryDetail.session.id === id) {
        state.activeLibraryDetail = null;
      }
      await Promise.all([loadHistorySessions(), loadCollections()]);
      if (state.view === "library") {
        await loadLibraryList();
      }
    } catch (err) {
      UI.toast(`Không xóa được phiên: ${err.message}`, { variant: "error" });
      await loadHistorySessions();
    }
  }

  // --- Studio Session & Streaming ---
  function resetStudioToEmpty() {
    if (state.abortController) {
      state.abortController.abort();
      state.abortController = null;
    }
    state.session = null;
    state.versions = [];
    state.activeVersionIndex = -1;
    state.streaming = false;
    state.streamText = "";
    state.selectedStarterId = "";
    state.openAssumptionIndex = -1;
    state.targetManualOverride = false;
    state.suggestedTarget = "";
    state.studioVarsOpen = false;
    state.diffOpen = false;
    state.diffBaseVersionId = "";

    el("requestInput").value = "";
    show(el("starterPresetBadge"), false);
    show(el("targetHintBtn"), false);
    show(el("studioErrorBanner"), false);
    show(el("saveBarEditor"), false);
    updateRequestCounter();
    renderStudioState();
    renderHistoryList();
  }

  // Opens a session at versionId, else at its saved (pinned) version, else the latest.
  async function openSessionInStudio(sessionId, versionId = "") {
    try {
      const detail = await API.request("GET", `/api/sessions/${sessionId}`);
      state.session = detail.session;
      state.versions = detail.versions || [];
      const wanted = versionId || state.session.pinnedVersionId || "";
      const idx = wanted ? state.versions.findIndex((v) => v.id === wanted) : -1;
      state.activeVersionIndex = idx >= 0 ? idx : state.versions.length - 1;
      state.openAssumptionIndex = -1;
      state.targetManualOverride = false;
      state.suggestedTarget = "";
      state.studioVarsOpen = false;
      state.diffOpen = false;
      state.diffBaseVersionId = "";

      el("requestInput").value = state.session.request || "";
      syncTargetSelectToVersion();
      show(el("starterPresetBadge"), false);
      show(el("targetHintBtn"), false);
      show(el("saveBarEditor"), false);
      updateRequestCounter();

      await switchView("studio", true);
      renderStudioState();
      renderHistoryList();
    } catch (err) {
      showStudioError(err.message);
    }
  }

  // The "Dùng cho" dropdown follows the version being viewed, so refining continues
  // with that version's target unless the user picks another one.
  function syncTargetSelectToVersion() {
    const ver = activeVersion();
    el("targetSelect").value = (ver && ver.target) || (state.session && state.session.target) || "chat";
  }

  function activeVersion() {
    if (state.activeVersionIndex < 0 || state.activeVersionIndex >= state.versions.length) {
      return null;
    }
    return state.versions[state.activeVersionIndex];
  }

  const TRUNCATED_MSG =
    "Đầu ra chạm giới hạn Max output tokens nên bị cắt. Tăng giới hạn trong Settings rồi bấm Tạo lại.";

  // After Stop, the server saves the partial version (or deletes a session that has no
  // output) only once it notices the disconnect. Retry briefly instead of racing it.
  // After Stop the server persists the partial version once the aborted stream unwinds;
  // poll with backoff (~3 s in total) instead of a fixed ~0.9 s window.
  const STOP_RELOAD_DELAYS_MS = [150, 250, 400, 600, 800, 1000];

  async function reloadSessionAfterStop(sessionId, minVersions) {
    let detail = null;
    for (let attempt = 0; attempt <= STOP_RELOAD_DELAYS_MS.length; attempt++) {
      try {
        detail = await API.request("GET", `/api/sessions/${sessionId}`);
        if ((detail.versions || []).length >= minVersions) return detail;
      } catch (err) {
        if (err.status === 404) return null;
      }
      if (attempt < STOP_RELOAD_DELAYS_MS.length) {
        await new Promise((resolve) => setTimeout(resolve, STOP_RELOAD_DELAYS_MS[attempt]));
      }
    }
    return detail;
  }

  async function startGenerate() {
    if (state.aiConnected === false) {
      // Locked until an AI provider is configured (PLAN.md §3); point at the fix.
      el("unconfiguredSettingsLink").focus();
      return;
    }
    const reqText = el("requestInput").value.trim();
    if (!reqText || state.streaming) return;
    if (reqText.length > 8000) {
      showStudioError("Yêu cầu vượt quá giới hạn 8.000 ký tự.");
      return;
    }

    show(el("studioErrorBanner"), false);
    show(el("starterPresetBadge"), false);
    show(el("targetHintBtn"), false);
    show(el("saveBarEditor"), false);
    // A new generation starts a new session: never keep showing the previous one's
    // versions, or a failed run would pair the new session with stale prompts.
    state.session = null;
    state.versions = [];
    state.activeVersionIndex = -1;
    state.openAssumptionIndex = -1;
    state.studioVarsOpen = false;
    state.diffOpen = false;
    state.diffBaseVersionId = "";
    state.streaming = true;
    state.streamText = "";
    let receivedFirstDelta = false;
    let gotVersion = false;
    let truncated = false;

    setAIStreamPhase("analyzing", "Đang phân tích yêu cầu…");
    renderStudioState();

    const target = el("targetSelect").value;
    state.retryAction = () => startGenerate();
    const ctrl = new AbortController();
    state.abortController = ctrl;

    try {
      await API.streamSSE(
        "/api/sessions",
        { request: reqText, target },
        {
          session(payload) {
            state.session = payload.session;
          },
          delta(payload) {
            if (!receivedFirstDelta) {
              receivedFirstDelta = true;
              setAIStreamPhase("streaming", "Đang viết prompt…");
            }
            state.streamText += payload.delta || "";
            renderLiveStreamPrompt();
          },
          done(payload) {
            state.session = payload.session || state.session;
            if (payload.version) {
              gotVersion = true;
              state.versions = [payload.version];
              state.activeVersionIndex = 0;
            }
            truncated = Boolean(payload.truncated);
          },
          error(payload) {
            showStudioError(payload.message || "Lỗi khi tạo prompt.", payload.code);
          },
        },
        ctrl.signal
      );
    } catch (err) {
      if (err.name === "AbortError") {
        const detail = state.session ? await reloadSessionAfterStop(state.session.id, 1) : null;
        if (detail && (detail.versions || []).length > 0) {
          state.session = detail.session;
          state.versions = detail.versions;
          state.activeVersionIndex = state.versions.length - 1;
          gotVersion = true;
        }
      } else {
        showStudioError(err.message, errorCode(err));
      }
    } finally {
      // No version means the server discarded the session; keep the typed request so
      // the user can simply retry.
      if (!gotVersion) {
        state.session = null;
        state.versions = [];
        state.activeVersionIndex = -1;
      }
      if (truncated) showStudioError(TRUNCATED_MSG);
      state.streaming = false;
      state.abortController = null;
      setAIStreamPhase("idle", "");
      renderStudioState();
      await loadHistorySessions();
    }
  }

  async function startRefine(instruction, source = "improve") {
    if (!state.session || state.streaming) return;
    const baseVer = activeVersion();
    if (!baseVer) return;

    show(el("studioErrorBanner"), false);
    state.openAssumptionIndex = -1;
    state.diffOpen = false;
    state.diffBaseVersionId = "";
    state.streaming = true;
    state.streamText = "";
    let receivedFirstDelta = false;
    let truncated = false;
    const sessionId = state.session.id;
    const versionCountBefore = state.versions.length;

    const statusMsg =
      source === "assumption"
        ? "Đang cập nhật giả định và viết lại prompt…"
        : source === "regenerate"
          ? "Đang tạo lại phiên bản khác…"
          : "Đang tinh chỉnh prompt…";
    setAIStreamPhase("analyzing", statusMsg);

    state.retryAction = () => startRefine(instruction, source);
    let failed = false;
    const ctrl = new AbortController();
    state.abortController = ctrl;

    try {
      await API.streamSSE(
        `/api/sessions/${state.session.id}/refine`,
        {
          fromVersionId: baseVer.id,
          instruction: instruction || "",
          source,
          target: el("targetSelect").value,
        },
        {
          delta(payload) {
            if (!receivedFirstDelta) {
              receivedFirstDelta = true;
              setAIStreamPhase("streaming", "Đang viết phiên bản mới…");
            }
            state.streamText += payload.delta || "";
            renderLiveStreamPrompt();
          },
          done(payload) {
            if (payload.session) state.session = payload.session;
            if (payload.version) {
              state.versions.push(payload.version);
              state.activeVersionIndex = state.versions.length - 1;
            }
            truncated = Boolean(payload.truncated);
          },
          error(payload) {
            failed = true;
            showStudioError(payload.message || "Lỗi khi tinh chỉnh prompt.", payload.code);
          },
        },
        ctrl.signal
      );
      // Keep the instruction on failure so "Thử lại" (or editing it) is one step away.
      if (!failed) {
        el("refineInput").value = "";
        el("submitRefineBtn").disabled = true;
      }
    } catch (err) {
      if (err.name === "AbortError") {
        const detail = await reloadSessionAfterStop(sessionId, versionCountBefore + 1);
        if (detail && state.session && state.session.id === sessionId) {
          state.session = detail.session;
          state.versions = detail.versions || [];
          state.activeVersionIndex = state.versions.length - 1;
        }
      } else {
        showStudioError(err.message, errorCode(err));
      }
    } finally {
      if (truncated) showStudioError(TRUNCATED_MSG);
      state.streaming = false;
      state.abortController = null;
      setAIStreamPhase("idle", "");
      renderStudioState();
      await loadHistorySessions();
    }
  }

  function setAIStreamPhase(phase, statusLabel) {
    const reqFrame = el("requestFrame");
    const resCard = el("resultCard");
    if (phase === "analyzing") {
      reqFrame.classList.add("is-busy");
      resCard.classList.remove("is-streaming");
      show(el("requestIdleFooter"), false);
      show(el("requestBusyFooter"), true);
      el("aiBusyStatusText").textContent = statusLabel;
    } else if (phase === "streaming") {
      reqFrame.classList.remove("is-busy");
      show(resCard, true);
      resCard.classList.add("is-streaming");
      show(el("requestIdleFooter"), false);
      show(el("requestBusyFooter"), true);
      el("aiBusyStatusText").textContent = statusLabel;
    } else {
      reqFrame.classList.remove("is-busy");
      resCard.classList.remove("is-streaming");
      show(el("requestIdleFooter"), true);
      show(el("requestBusyFooter"), false);
    }
  }

  function renderLiveStreamPrompt() {
    show(el("resultCard"), true);
    show(el("manualEditContainer"), false);
    show(el("diffBar"), false);
    show(el("studioVarsDrawer"), false);
    show(el("promptDisplayContainer"), true);
    show(el("assumptionsSection"), false);
    const box = el("promptDisplayContainer");
    box.className = "prompt-markdown";
    box.innerHTML = renderMarkdownToHTML(state.streamText, true);
  }

  // Populates variable inputs inside `container` bound to `sessionVars[sessionId]`.
  function renderBoundVarInputs(container, vars, sessionId, onInputExtra) {
    if (!container) return;
    container.innerHTML = "";
    const varMap = getSessionVarMap(sessionId);
    for (const name of vars) {
      const field = document.createElement("label");
      field.className = "field";
      const lbl = document.createElement("span");
      lbl.className = "field__label text-mono";
      lbl.textContent = `{{${name}}}`;
      const inp = document.createElement("input");
      inp.type = "text";
      inp.className = "input input--sm";
      inp.setAttribute("data-var-name", name);
      inp.placeholder = `Nhập giá trị cho ${name}…`;
      inp.value = varMap[name] || "";
      inp.addEventListener("input", () => {
        varMap[name] = inp.value;
        if (typeof onInputExtra === "function") onInputExtra();
      });
      field.appendChild(lbl);
      field.appendChild(inp);
      container.appendChild(field);
    }
  }

  function firstEmptyVarInput(container) {
    if (!container) return null;
    return [...container.querySelectorAll("[data-var-name]")].find((inp) => !inp.value.trim()) || null;
  }

  function copyFilledFromContainer(btn, content, sessionId, inputsContainer, asApiJson = false) {
    const empty = firstEmptyVarInput(inputsContainer);
    if (empty) {
      const name = empty.getAttribute("data-var-name") || "biến";
      empty.focus();
      UI.toast(`Hãy điền giá trị cho {{${name}}} trước khi copy bản đã điền.`, { variant: "error" });
      return;
    }
    const filled = fillPromptVariables(content, getSessionVarMap(sessionId));
    if (asApiJson) {
      const parts = splitAPIPrompt(filled);
      const msgs = parts.hasSplit
        ? [
            { role: "system", content: parts.system },
            { role: "user", content: parts.user },
          ]
        : [{ role: "user", content: filled }];
      copyTextWithFeedback(btn, JSON.stringify(msgs, null, 2), "Đã copy JSON");
      return;
    }
    copyTextWithFeedback(btn, filled, "Đã copy bản điền");
  }

  function resolveDiffBaseVersion(ver) {
    if (!ver || state.versions.length < 2) return null;
    const others = state.versions.filter((v) => v.id !== ver.id);
    if (!others.length) return null;
    if (state.diffBaseVersionId) {
      const chosen = others.find((v) => v.id === state.diffBaseVersionId);
      if (chosen) return chosen;
    }
    if (ver.parentId) {
      const parent = others.find((v) => v.id === ver.parentId);
      if (parent) return parent;
    }
    const prevIdx = state.activeVersionIndex > 0 ? state.activeVersionIndex - 1 : 0;
    return state.versions[prevIdx] && state.versions[prevIdx].id !== ver.id
      ? state.versions[prevIdx]
      : others[0];
  }

  function renderStudioDiff(ver, displayBox) {
    const baseVer = resolveDiffBaseVersion(ver);
    if (!baseVer) {
      state.diffOpen = false;
      show(el("diffBar"), false);
      return false;
    }
    state.diffBaseVersionId = baseVer.id;
    const sel = el("diffBaseSelect");
    sel.innerHTML = "";
    for (const cand of state.versions) {
      if (cand.id === ver.id) continue;
      const opt = document.createElement("option");
      opt.value = cand.id;
      const roleNote =
        ver.parentId === cand.id ? " (bản cha)" : cand.seq === 1 ? " (bản gốc)" : "";
      opt.textContent = `v${cand.seq}${roleNote} — ${sourceLabel(cand)}`;
      sel.appendChild(opt);
    }
    sel.value = baseVer.id;

    const diff = computeLineDiff(baseVer.content, ver.content);
    el("diffStatsLabel").textContent =
      diff.added === 0 && diff.removed === 0
        ? "Không có dòng thay đổi"
        : `+${diff.added} / −${diff.removed} dòng`;

    displayBox.className = "diff-view";
    if (diff.added === 0 && diff.removed === 0) {
      displayBox.innerHTML = `<div class="diff-line diff-line--ctx"><span class="diff-line__sign">=</span><span>Nội dung hai phiên bản giống hệt nhau.</span></div>`;
      return true;
    }
    displayBox.innerHTML = diff.rows
      .map((r) => {
        const cls = r.type === "add" ? "diff-line--add" : r.type === "del" ? "diff-line--del" : "diff-line--ctx";
        const sign = r.type === "add" ? "+" : r.type === "del" ? "−" : " ";
        return `<div class="diff-line ${cls}"><span class="diff-line__sign">${sign}</span><span>${highlightInlineVars(escapeHTML(r.text))}</span></div>`;
      })
      .join("");
    return true;
  }

  function renderStudioState() {
    renderStarters();
    const ver = activeVersion();
    const hasResult = Boolean(ver);

    show(el("resultCard"), hasResult);
    show(el("refineSection"), hasResult);

    if (state.session && state.session.origin === "pasted") {
      el("requestInput").placeholder = "Phiên này được dán trực tiếp vào Thư viện (không có yêu cầu gốc).";
    } else {
      el("requestInput").placeholder =
        "Tôi cần prompt để viết kịch bản video giới thiệu sản phẩm 30 giây cho TikTok";
    }

    if (!hasResult) return;

    // Stepper & badges (with parent lineage)
    el("versionIndicator").textContent = `v${ver.seq} / ${state.versions.length}`;
    el("prevVersionBtn").disabled = state.activeVersionIndex <= 0;
    el("nextVersionBtn").disabled = state.activeVersionIndex >= state.versions.length - 1;
    const srcBadge = el("versionSourceBadge");
    srcBadge.textContent = sourceLabel(ver, state.versions);
    srcBadge.title = ver.instruction || "";
    show(el("incompleteBadge"), Boolean(ver.incomplete));

    // Version Diff toggle (D-P010-2)
    const canDiff = state.versions.length > 1;
    if (!canDiff) state.diffOpen = false;
    const diffBtn = el("toggleDiffBtn");
    show(diffBtn, canDiff);
    diffBtn.setAttribute("aria-pressed", state.diffOpen ? "true" : "false");
    show(el("diffBar"), canDiff && state.diffOpen);

    // Save button & Save Bar. Saving pins the version being viewed; viewing another
    // version offers to move the saved copy to it.
    const isSaved = Boolean(state.session && state.session.savedAt);
    const pinnedVer = isSaved ? savedVersion() : null;
    const viewingSaved = Boolean(pinnedVer && pinnedVer.id === ver.id);
    const saveBtn = el("saveSessionBtn");
    saveBtn.textContent = !isSaved ? "Lưu" : viewingSaved ? "Đã lưu" : `Cập nhật bản lưu (v${ver.seq})`;
    saveBtn.setAttribute("aria-pressed", viewingSaved ? "true" : "false");

    show(el("saveBar"), isSaved);
    if (isSaved) {
      const colName = state.session.collectionName || "Chưa phân loại";
      const seqNote = pinnedVer ? ` · bản v${pinnedVer.seq}` : "";
      el("saveBarSummary").innerHTML = `Đã lưu vào <strong>${escapeHTML(colName)}</strong> với tên “${escapeHTML(state.session.title)}”${seqNote}`;
      el("saveTitleInput").value = state.session.title || "";
      populateCollectionSelect(el("saveCollectionSelect"), state.session.collectionId, false);
    }

    // Prompt content display (Diff vs Two-part API vs Single Markdown)
    show(el("manualEditContainer"), false);
    const displayBox = el("promptDisplayContainer");
    show(displayBox, true);

    const target = ver.target || (state.session && state.session.target) || el("targetSelect").value;
    const apiSplit = target === "api" ? splitAPIPrompt(ver.content) : { hasSplit: false };
    const vars = extractVariables(ver.content);

    // Inline Variable Fill drawer (D-P010-1)
    if (vars.length === 0) state.studioVarsOpen = false;
    const varsBtn = el("toggleStudioVarsBtn");
    show(varsBtn, vars.length > 0);
    varsBtn.textContent = vars.length > 0 ? `Điền biến (${vars.length})` : "Điền biến";
    varsBtn.setAttribute("aria-expanded", state.studioVarsOpen ? "true" : "false");
    varsBtn.setAttribute("aria-pressed", state.studioVarsOpen ? "true" : "false");
    show(el("studioVarsDrawer"), vars.length > 0 && state.studioVarsOpen);
    show(el("copyFilledApiJsonBtn"), apiSplit.hasSplit);
    if (vars.length > 0 && state.studioVarsOpen && state.session) {
      renderBoundVarInputs(el("studioVarsInputs"), vars, state.session.id);
    }

    show(el("copyApiJsonBtn"), apiSplit.hasSplit);
    el("copyPromptBtn").textContent = apiSplit.hasSplit ? "Copy tất cả" : "Copy";

    if (state.diffOpen && renderStudioDiff(ver, displayBox)) {
      // Diff rendered into displayBox
    } else {
      renderPromptContent(displayBox, ver.content, apiSplit, "prompt-markdown");
    }

    // Editable Assumptions
    renderAssumptions(ver.assumptions || []);

    // Metadata footer
    const metaParts = [];
    if (ver.model) metaParts.push(ver.model);
    if (ver.inputTokens || ver.outputTokens) {
      metaParts.push(`${ver.inputTokens || 0} → ${ver.outputTokens || 0} token`);
    }
    if (ver.latencyMs) {
      metaParts.push(`${(ver.latencyMs / 1000).toFixed(1).replace(".", ",")} giây`);
    }
    if (vars.length > 0) {
      metaParts.push(`${vars.length} biến: ${vars.join(", ")}`);
    }
    if (ver.templateVersion) {
      metaParts.push(ver.templateVersion);
    }
    el("resultMetaBar").innerHTML = metaParts.map((m) => `<span>${escapeHTML(m)}</span>`).join("");
  }

  function renderAssumptions(assumptions) {
    const sec = el("assumptionsSection");
    const list = el("assumptionsList");
    const titleMain = el("assumptionsTitleMain");
    const titleHint = el("assumptionsTitleHint");
    list.innerHTML = "";
    if (!assumptions || !assumptions.length) {
      sec.classList.remove("assumptions-section--needs-review");
      show(sec, false);
      return;
    }
    show(sec, true);

    const highCount = assumptions.filter((a) => a && a.impact === "high").length;
    sec.classList.toggle("assumptions-section--needs-review", highCount > 0);
    if (titleMain && titleHint) {
      if (highCount > 0) {
        titleMain.textContent = `Cần xác nhận ${highCount} điểm quan trọng`;
        titleMain.classList.add("assumptions-section__title--warning");
        titleHint.textContent = "· AI đã chọn mặc định an toàn, bấm để chốt hoặc dùng ngay";
      } else {
        titleMain.textContent = "AI đã giả định";
        titleMain.classList.remove("assumptions-section__title--warning");
        titleHint.textContent = "· bấm để đổi, prompt sẽ được viết lại";
      }
    }

    assumptions.forEach((item, idx) => {
      const isHigh = item.impact === "high";
      const wrap = document.createElement("div");
      wrap.className = "assumption-wrap";

      const chip = document.createElement("button");
      chip.type = "button";
      chip.className = "assumption-chip" + (isHigh ? " assumption-chip--high" : "");
      chip.setAttribute("aria-expanded", state.openAssumptionIndex === idx ? "true" : "false");
      if (isHigh) {
        chip.innerHTML = `${UI.icons.alertCircle}<span class="badge badge--warning">Cần rõ</span><span class="assumption-chip__label">${escapeHTML(item.label)}:</span> <span>${escapeHTML(item.value)}</span>${UI.icons.chevronDown}`;
      } else {
        chip.innerHTML = `<span class="assumption-chip__label">${escapeHTML(item.label)}:</span> <span>${escapeHTML(item.value)}</span>${UI.icons.chevronDown}`;
      }
      chip.addEventListener("click", (e) => {
        e.stopPropagation();
        state.openAssumptionIndex = state.openAssumptionIndex === idx ? -1 : idx;
        renderAssumptions(assumptions);
      });
      wrap.appendChild(chip);

      if (state.openAssumptionIndex === idx) {
        const pop = document.createElement("div");
        pop.className = "assumption-popover" + (isHigh ? " assumption-popover--high" : "");
        pop.addEventListener("click", (e) => e.stopPropagation());

        if (isHigh || item.question) {
          const qNode = document.createElement("div");
          qNode.className = "assumption-popover__question";
          qNode.textContent = item.question || `Xác nhận “${item.label}”`;
          pop.appendChild(qNode);

          const curNode = document.createElement("div");
          curNode.className = "assumption-popover__current";
          curNode.textContent = `Đang áp dụng: ${item.value}`;
          pop.appendChild(curNode);
        } else {
          const title = document.createElement("div");
          title.className = "text-xs text-secondary font-semibold";
          title.textContent = `Đổi giả định “${item.label}”`;
          pop.appendChild(title);
        }

        for (const alt of item.alternatives || []) {
          const optBtn = document.createElement("button");
          optBtn.type = "button";
          optBtn.className = "assumption-popover__option";
          optBtn.textContent = alt;
          optBtn.addEventListener("click", () => {
            state.openAssumptionIndex = -1;
            startRefine(`Đổi giả định "${item.label}" thành: ${alt}`, "assumption");
          });
          pop.appendChild(optBtn);
        }

        const customForm = document.createElement("form");
        customForm.className = "cluster cluster--xs";
        const customInput = document.createElement("input");
        customInput.type = "text";
        customInput.className = "input input--sm toolbar__spacer";
        customInput.placeholder = "Hoặc nhập giá trị khác…";
        const applyBtn = document.createElement("button");
        applyBtn.type = "submit";
        applyBtn.className = "btn btn--primary btn--sm";
        applyBtn.textContent = "Đổi";
        customForm.appendChild(customInput);
        customForm.appendChild(applyBtn);
        customForm.addEventListener("submit", (e) => {
          e.preventDefault();
          const val = customInput.value.trim();
          if (!val) return;
          state.openAssumptionIndex = -1;
          startRefine(`Đổi giả định "${item.label}" thành: ${val}`, "assumption");
        });
        pop.appendChild(customForm);

        wrap.appendChild(pop);
      }

      list.appendChild(wrap);
    });
  }

  // --- One-Click Save & Manual Edit ---
  // The version the Library shows: the pinned one, or the latest for legacy saves.
  function savedVersion() {
    if (!state.session || !state.session.savedAt || !state.versions.length) return null;
    const pinned = state.session.pinnedVersionId;
    return state.versions.find((v) => v.id === pinned) || state.versions[state.versions.length - 1];
  }

  async function handleOneClickSave() {
    if (!state.session) return;
    const ver = activeVersion();
    if (!ver) return;
    const pinnedVer = savedVersion();
    if (state.session.savedAt && pinnedVer && pinnedVer.id === ver.id) {
      // Already saved at this version: toggle the inline editor (title / collection).
      const editor = el("saveBarEditor");
      show(editor, editor.classList.contains("is-hidden"));
      return;
    }
    try {
      const body = { versionId: ver.id };
      if (state.session.collectionId) body.collectionId = state.session.collectionId;
      const saved = await API.request("PUT", `/api/sessions/${state.session.id}/save`, body);
      state.session = saved;
      await Promise.all([loadCollections(), loadHistorySessions()]);
      renderStudioState();
    } catch (err) {
      showStudioError(err.message);
    }
  }

  async function applySaveEdit() {
    if (!state.session) return;
    const title = el("saveTitleInput").value.trim();
    const collectionId = el("saveCollectionSelect").value;
    try {
      const pinnedVer = savedVersion();
      const updated = await API.request("PUT", `/api/sessions/${state.session.id}/save`, {
        title,
        collectionId,
        versionId: pinnedVer ? pinnedVer.id : "",
      });
      state.session = updated;
      show(el("saveBarEditor"), false);
      await Promise.all([loadCollections(), loadHistorySessions()]);
      renderStudioState();
    } catch (err) {
      showStudioError(err.message);
    }
  }

  async function unsaveCurrentSession() {
    if (!state.session) return;
    try {
      await API.request("DELETE", `/api/sessions/${state.session.id}/save`);
      state.session.savedAt = null;
      state.session.pinnedVersionId = "";
      state.session.collectionId = "";
      state.session.collectionName = "";
      show(el("saveBarEditor"), false);
      await Promise.all([loadCollections(), loadHistorySessions()]);
      renderStudioState();
    } catch (err) {
      showStudioError(err.message);
    }
  }

  async function saveManualEdit() {
    if (!state.session) return;
    const ver = activeVersion();
    const content = el("manualEditTextarea").value.trim();
    if (!content) return;
    try {
      const res = await API.request("POST", `/api/sessions/${state.session.id}/versions`, {
        parentId: ver ? ver.id : "",
        content,
      });
      if (res && res.version) {
        state.versions.push(res.version);
        state.activeVersionIndex = state.versions.length - 1;
      }
      renderStudioState();
    } catch (err) {
      showStudioError(err.message);
    }
  }

  // --- Collections & Library ---
  async function loadCollections() {
    try {
      const data = await API.request("GET", "/api/collections");
      state.collections = data.collections || [];
      state.totalSaved = data.totalSaved || 0;
      renderCollectionsNav();
    } catch (_) {}
  }

  function populateCollectionSelect(selectEl, selectedId, includeCreateOption) {
    if (!selectEl) return;
    selectEl.innerHTML = "";
    const sorted = [...state.collections].sort((a, b) => Number(a.isSystem) - Number(b.isSystem));
    for (const c of sorted) {
      const opt = document.createElement("option");
      opt.value = c.id;
      opt.textContent = c.name;
      selectEl.appendChild(opt);
    }
    if (includeCreateOption) {
      const createOpt = document.createElement("option");
      createOpt.value = "__new__";
      createOpt.textContent = "+ Tạo collection mới…";
      selectEl.appendChild(createOpt);
    }
    if (selectedId && [...selectEl.options].some((o) => o.value === selectedId)) {
      selectEl.value = selectedId;
    } else if (state.settings && state.settings.lastCollectionId) {
      if ([...selectEl.options].some((o) => o.value === state.settings.lastCollectionId)) {
        selectEl.value = state.settings.lastCollectionId;
      }
    }
  }

  function renderCollectionsNav() {
    const nav = el("collectionsList");
    if (!nav) return;
    nav.innerHTML = "";

    const allRow = document.createElement("button");
    allRow.type = "button";
    allRow.className = "nav-item" + (state.activeCollectionId === "all" ? " is-active" : "");
    if (state.activeCollectionId === "all") allRow.setAttribute("aria-current", "true");
    allRow.innerHTML = `<span class="nav-item__title">Tất cả</span><span class="text-xs text-muted">${state.totalSaved}</span>`;
    allRow.addEventListener("click", () => {
      state.activeCollectionId = "all";
      renderCollectionsNav();
      loadLibraryList();
    });
    nav.appendChild(allRow);

    const userCols = state.collections.filter((c) => !c.isSystem);
    const sysCols = state.collections.filter((c) => c.isSystem);

    for (const c of [...userCols, ...sysCols]) {
      const row = document.createElement("button");
      row.type = "button";
      row.className = "nav-item" + (state.activeCollectionId === c.id ? " is-active" : "");
      if (state.activeCollectionId === c.id) row.setAttribute("aria-current", "true");
      row.innerHTML = `<span class="nav-item__title">${escapeHTML(c.name)}</span><span class="text-xs text-muted">${c.count || 0}</span>`;
      row.addEventListener("click", () => {
        state.activeCollectionId = c.id;
        renderCollectionsNav();
        loadLibraryList();
      });
      nav.appendChild(row);
    }
  }

  async function loadLibraryList(append = false) {
    try {
      const params = new URLSearchParams();
      params.set("limit", String(PAGE_SIZE));
      params.set("offset", String(append ? state.librarySessions.length : 0));
      params.set("saved", "1");
      if (state.activeCollectionId && state.activeCollectionId !== "all") {
        params.set("collection", state.activeCollectionId);
      }
      const q = el("librarySearchInput").value.trim();
      if (q) params.set("q", q);
      params.set("sort", el("librarySortSelect").value || "recent");

      const data = await API.request("GET", `/api/sessions?${params.toString()}`);
      const page = data.sessions || [];
      if (append) {
        const known = new Set(state.librarySessions.map((s) => s.id));
        state.librarySessions = state.librarySessions.concat(page.filter((s) => !known.has(s.id)));
      } else {
        state.librarySessions = page;
      }
      state.libraryHasMore = Boolean(data.hasMore);
      renderLibraryHeaderAndList();
      if (append) return;

      if (state.librarySessions.length > 0) {
        const stillExists =
          state.activeLibraryDetail &&
          state.librarySessions.some((s) => s.id === state.activeLibraryDetail.session.id);
        if (!stillExists) {
          await selectLibrarySession(state.librarySessions[0].id);
        } else {
          await selectLibrarySession(state.activeLibraryDetail.session.id);
        }
      } else {
        state.activeLibraryDetail = null;
        renderLibraryDetail();
      }
    } catch (err) {
      UI.toast(`Không tải được thư viện: ${err.message}`, { variant: "error" });
    }
  }

  function renderLibraryHeaderAndList() {
    const activeCol = state.collections.find((c) => c.id === state.activeCollectionId);
    el("libraryCollectionHeading").textContent = activeCol ? activeCol.name : "Tất cả";
    el("libraryCountLabel").textContent = `${state.librarySessions.length}${state.libraryHasMore ? "+" : ""} prompt`;

    const isEditableCol = Boolean(activeCol && !activeCol.isSystem);
    show(el("renameCollectionBtn"), isEditableCol);
    show(el("deleteCollectionBtn"), isEditableCol);

    const listEl = el("libraryItemsList");
    listEl.innerHTML = "";
    if (!state.librarySessions.length) {
      listEl.innerHTML = `
        <div class="empty-state">
          <div class="empty-state__title">Chưa có prompt nào trong mục này</div>
          <p class="empty-state__desc">Bấm “Lưu” trên kết quả Studio hoặc bấm “+ Thêm prompt” để dán prompt có sẵn vào thư viện.</p>
        </div>
      `;
      return;
    }

    for (const item of state.librarySessions) {
      const isSelected = Boolean(state.activeLibraryDetail && state.activeLibraryDetail.session.id === item.id);
      const card = document.createElement("div");
      card.className = "library-item" + (isSelected ? " is-active" : "");

      const mainBtn = document.createElement("button");
      mainBtn.type = "button";
      mainBtn.className = "library-item__main";
      if (isSelected) mainBtn.setAttribute("aria-current", "true");
      mainBtn.addEventListener("click", () => selectLibrarySession(item.id));

      const originTag =
        item.origin === "pasted"
          ? `<span>Đã dán</span>`
          : `<span class="library-item__origin--studio">Từ Studio</span>`;
      const varsTag =
        item.variablesCount > 0
          ? `<span class="library-item__vars">${item.variablesCount} biến</span>`
          : "";

      mainBtn.innerHTML = `
        <span class="library-item__title">${escapeHTML(item.title)}</span>
        <span class="library-item__preview">${escapeHTML(item.preview || "")}</span>
        <span class="library-item__meta">
          ${originTag}
          ${varsTag}
          <span>${escapeHTML(formatRelativeTime(item.openedAt || item.savedAt))}</span>
        </span>
      `;
      card.appendChild(mainBtn);

      const actions = document.createElement("span");
      actions.className = "library-item__actions";
      const renameBtn = document.createElement("button");
      renameBtn.type = "button";
      renameBtn.className = "btn btn--ghost btn--icon";
      renameBtn.setAttribute("aria-label", `Đổi tên “${item.title || "Prompt"}”`);
      renameBtn.setAttribute("data-tooltip", "Đổi tên");
      renameBtn.innerHTML = UI.icons.edit;
      renameBtn.addEventListener("click", async () => {
        if (!state.activeLibraryDetail || state.activeLibraryDetail.session.id !== item.id) {
          await selectLibrarySession(item.id);
        }
        openLibraryInlineRename();
      });
      actions.appendChild(renameBtn);
      card.appendChild(actions);

      listEl.appendChild(card);
    }

    if (state.libraryHasMore) {
      listEl.appendChild(loadMoreButton(() => loadLibraryList(true)));
    }
  }

  async function selectLibrarySession(sessionId) {
    try {
      const prevId = state.activeLibraryDetail ? state.activeLibraryDetail.session.id : "";
      const detail = await API.request("GET", `/api/sessions/${sessionId}`);
      state.activeLibraryDetail = detail;
      if (prevId !== sessionId) {
        state.libVarsOpen = false;
        state.libRenaming = false;
      }
      const pinned = detail.session.pinnedVersionId;
      const versions = detail.versions || [];
      const foundPinned = pinned && versions.find((v) => v.id === pinned);
      state.activeLibraryVersionId = foundPinned
        ? foundPinned.id
        : versions.length
          ? versions[versions.length - 1].id
          : "";
      renderLibraryHeaderAndList();
      renderLibraryDetail();
    } catch (err) {
      UI.toast(`Không mở được prompt: ${err.message}`, { variant: "error" });
    }
  }

  function activeLibraryVersion() {
    if (!state.activeLibraryDetail) return null;
    const versions = state.activeLibraryDetail.versions || [];
    return (
      versions.find((v) => v.id === state.activeLibraryVersionId) ||
      versions[versions.length - 1] ||
      null
    );
  }

  function renderLibraryDetail() {
    const detail = state.activeLibraryDetail;
    show(el("libraryEmptyDetail"), !detail);
    show(el("libraryActiveDetail"), Boolean(detail));
    if (!detail) return;

    const sess = detail.session;
    const allAsc = detail.versions || [];
    const versions = [...allAsc].reverse(); // newest first
    const currentVer = activeLibraryVersion();

    const originText = sess.origin === "pasted" ? "Đã dán" : "Từ Studio";
    el("libDetailCrumb").textContent = `${sess.collectionName || "Chưa phân loại"} · ${originText}`;
    el("libDetailTitle").textContent = sess.title;
    show(el("libTitleRow"), !state.libRenaming);
    show(el("libInlineRenameForm"), state.libRenaming);

    const descEl = el("libDetailDesc");
    descEl.textContent = sess.description || "";
    show(descEl, Boolean(sess.description));

    populateCollectionSelect(el("libMoveCollectionSelect"), sess.collectionId, false);

    const target = (currentVer && currentVer.target) || sess.target || "chat";
    const apiSplit =
      currentVer && target === "api" ? splitAPIPrompt(currentVer.content) : { hasSplit: false };
    show(el("libCopyApiJsonBtn"), apiSplit.hasSplit);
    el("libCopyBtn").textContent = apiSplit.hasSplit ? "Copy tất cả" : "Copy";

    const vars = currentVer ? extractVariables(currentVer.content) : [];
    if (vars.length === 0) state.libVarsOpen = false;
    const libVarsBtn = el("toggleLibVarsBtn");
    show(libVarsBtn, vars.length > 0);
    libVarsBtn.textContent = vars.length > 0 ? `Điền biến (${vars.length})` : "Điền biến";
    libVarsBtn.setAttribute("aria-expanded", state.libVarsOpen ? "true" : "false");
    libVarsBtn.setAttribute("aria-pressed", state.libVarsOpen ? "true" : "false");
    show(el("libVarsDrawer"), vars.length > 0 && state.libVarsOpen);
    show(el("copyFilledLibApiJsonBtn"), apiSplit.hasSplit);
    if (vars.length > 0 && state.libVarsOpen) {
      renderBoundVarInputs(el("libVarsInputs"), vars, sess.id);
    }

    const contentBox = el("libDetailContent");
    if (currentVer) {
      renderPromptContent(contentBox, currentVer.content, apiSplit, "api-block__body prompt-markdown");
    } else {
      contentBox.className = "";
      contentBox.innerHTML = "";
    }

    const vList = el("libVersionsList");
    vList.innerHTML = "";
    for (const v of versions) {
      const li = document.createElement("li");
      const btn = document.createElement("button");
      btn.type = "button";
      const isCurrent = Boolean(currentVer && v.id === currentVer.id);
      btn.className = "version-history-item" + (isCurrent ? " is-active" : "");
      if (isCurrent) btn.setAttribute("aria-current", "true");
      const pinNote =
        sess.pinnedVersionId === v.id || (!sess.pinnedVersionId && v.id === versions[0].id)
          ? " · bản đã lưu"
          : "";
      btn.innerHTML = `
        <span class="version-history-item__seq">v${v.seq}</span>
        <span class="toolbar__spacer">${escapeHTML(sourceLabel(v, allAsc))}${pinNote}</span>
        <span class="text-xs text-muted">${escapeHTML(formatRelativeTime(v.createdAt))}</span>
      `;
      btn.addEventListener("click", () => {
        state.activeLibraryVersionId = v.id;
        renderLibraryDetail();
      });
      li.appendChild(btn);
      vList.appendChild(li);
    }

    el("libDetailTimestamps").textContent = `Lưu ${formatRelativeTime(sess.savedAt)} · Mở lần cuối: ${formatRelativeTime(sess.openedAt)}`;
  }

  function openLibraryInlineRename() {
    if (!state.activeLibraryDetail) return;
    state.libRenaming = true;
    renderLibraryDetail();
    const inp = el("libInlineRenameInput");
    if (inp) {
      inp.value = state.activeLibraryDetail.session.title || "";
      inp.focus();
      inp.select();
    }
  }

  function cancelLibraryInlineRename(restoreFocus = true) {
    if (!state.libRenaming) return;
    state.libRenaming = false;
    renderLibraryDetail();
    if (restoreFocus && el("libInlineRenameBtn")) {
      el("libInlineRenameBtn").focus();
    }
  }

  async function submitLibraryInlineRename() {
    if (!state.activeLibraryDetail) return;
    const sessId = state.activeLibraryDetail.session.id;
    const inp = el("libInlineRenameInput");
    const next = inp.value.trim();
    if (!next) {
      UI.toast("Tên prompt không được để trống.", { variant: "error" });
      inp.focus();
      return;
    }
    if (next === state.activeLibraryDetail.session.title) {
      cancelLibraryInlineRename(true);
      return;
    }
    const saveBtn = el("libInlineRenameSaveBtn");
    if (saveBtn) saveBtn.disabled = true;
    try {
      const updated = await API.request("PATCH", `/api/sessions/${sessId}`, { title: next });
      state.libRenaming = false;
      if (state.session && state.session.id === sessId && updated && updated.title) {
        state.session.title = updated.title;
      }
      await Promise.all([loadLibraryList(), loadHistorySessions(), selectLibrarySession(sessId)]);
      UI.toast("Đã đổi tên prompt.");
    } catch (err) {
      UI.toast(`Không đổi được tên: ${err.message}`, { variant: "error" });
      inp.focus();
    } finally {
      if (saveBtn) saveBtn.disabled = false;
    }
  }

  // --- Paste Modal ---
  function openPasteModal() {
    state.duplicateSessionId = "";
    state.pasteTitleTouched = false;
    el("pasteContentInput").value = "";
    el("pasteTitleInput").value = "";
    el("pasteDescInput").value = "";
    el("pasteNewCollectionInput").value = "";
    show(el("pasteNewCollectionField"), false);
    show(el("pasteDuplicateAlert"), false);
    show(el("pasteErrorAlert"), false);
    show(el("pasteVarsBadge"), false);
    el("pasteCharCounter").textContent = "0 / 32.000";

    const preferredCol = state.activeCollectionId !== "all" ? state.activeCollectionId : "";
    populateCollectionSelect(el("pasteCollectionSelect"), preferredCol, true);
    el("pasteModal").classList.add("is-open");
    el("pasteContentInput").focus();
  }

  function closePasteModal() {
    el("pasteModal").classList.remove("is-open");
  }

  async function submitPasteModal() {
    const content = el("pasteContentInput").value.trim();
    if (!content) {
      const errEl = el("pasteErrorAlert");
      errEl.textContent = "Vui lòng dán nội dung prompt trước khi lưu.";
      show(errEl, true);
      return;
    }

    show(el("pasteDuplicateAlert"), false);
    show(el("pasteErrorAlert"), false);

    let collectionId = el("pasteCollectionSelect").value;
    try {
      if (collectionId === "__new__") {
        const newName = el("pasteNewCollectionInput").value.trim();
        if (!newName) {
          const errEl = el("pasteErrorAlert");
          errEl.textContent = "Vui lòng nhập tên collection mới.";
          show(errEl, true);
          return;
        }
        const createdCol = await API.request("POST", "/api/collections", { name: newName });
        collectionId = createdCol.id;
        await loadCollections();
      }

      const res = await API.request("POST", "/api/sessions/paste", {
        title: el("pasteTitleInput").value.trim(),
        description: el("pasteDescInput").value.trim(),
        content,
        collectionId,
      });
      closePasteModal();
      await Promise.all([loadCollections(), loadLibraryList(), loadHistorySessions()]);
      if (res && res.session) {
        await selectLibrarySession(res.session.id);
      }
    } catch (err) {
      if (err.status === 409 && err.data && err.data.existingSessionId) {
        state.duplicateSessionId = err.data.existingSessionId;
        show(el("pasteDuplicateAlert"), true);
        return;
      }
      const errEl = el("pasteErrorAlert");
      errEl.textContent = err.message;
      show(errEl, true);
    }
  }

  // --- Helpers & Event Bindings ---
  // Detects strong coding-agent or system-prompt-API cues so Studio can offer a 1-click
  // target switch without opening a blocking clarification modal (D-P011-3).
  function detectSuggestedTarget(text, currentTarget) {
    const trimmed = String(text || "").trim();
    if (trimmed.length < 16) return "";
    const lower = trimmed.toLowerCase();

    // Unicode-safe word boundary helpers (\b in JS only treats ASCII [A-Za-z0-9_] as word chars).
    const hasPhrase = (re) => re.test(lower);

    // Coding-agent cues take priority over generic "API" mentions (e.g. "để agent sửa lỗi API thanh toán").
    const explicitAgent =
      hasPhrase(/(?:^|[^\p{L}\p{N}_])(coding agent|ide agent|claude code|cursor|codex|antigravity)(?=$|[^\p{L}\p{N}_])/iu) ||
      hasPhrase(/(?:^|[^\p{L}\p{N}_])(?:để|cho|yêu cầu|nhờ)\s+agent(?=$|[^\p{L}\p{N}_])/iu) ||
      hasPhrase(/(?:^|[^\p{L}\p{N}_])agent\s+(?:lập trình|review|kiểm tra|chẩn đoán|sửa|viết|refactor|tối ưu|khảo sát|thêm|vá)(?=$|[^\p{L}\p{N}_])/iu) ||
      hasPhrase(/panic:\s*runtime error/iu);
    const repoWorkflow =
      hasPhrase(/(?:^|[^\p{L}\p{N}_])(pull request|regression test|unit test|codebase|repository|workspace|git diff|goroutine)(?=$|[^\p{L}\p{N}_])/iu) &&
      hasPhrase(/(?:^|[^\p{L}\p{N}_])(review|sửa|chẩn đoán|refactor|fix|vá|kiểm thử|tối ưu|thêm)(?=$|[^\p{L}\p{N}_])/iu);
    if (explicitAgent || repoWorkflow) {
      return currentTarget !== "agent" ? "agent" : "";
    }

    // System-prompt API cues (building an LLM messages endpoint, not debugging a business API).
    const explicitApi =
      hasPhrase(/(?:^|[^\p{L}\p{N}_])system\s+prompt\s+(?:cho\s+)?api(?=$|[^\p{L}\p{N}_])/iu) ||
      hasPhrase(/(?:^|[^\p{L}\p{N}_])(?:tạo|viết|thiết kế)\s+(?:system\s+prompt|prompt)\s+cho\s+api(?=$|[^\p{L}\p{N}_])/iu) ||
      hasPhrase(/(?:^|[^\p{L}\p{N}_])api\s+(?:trích xuất|phân loại|tóm tắt|chuyển\s+câu\s+hỏi|chuyển\s+đổi)(?=$|[^\p{L}\p{N}_])/iu) ||
      hasPhrase(/(?:^|[^\p{L}\p{N}_])(?:openai|anthropic)\s+messages\s+api(?=$|[^\p{L}\p{N}_])/iu) ||
      lower.includes("===user===");
    if (explicitApi) {
      return currentTarget !== "api" ? "api" : "";
    }

    return "";
  }

  function updateTargetHint() {
    const btn = el("targetHintBtn");
    if (!btn) return;
    const text = el("requestInput").value;
    if (!text.trim()) {
      state.targetManualOverride = false;
    }
    if (state.streaming || state.targetManualOverride || (state.session && state.session.request === text.trim())) {
      state.suggestedTarget = "";
      show(btn, false);
      return;
    }
    const suggested = detectSuggestedTarget(text, el("targetSelect").value);
    state.suggestedTarget = suggested;
    if (!suggested) {
      show(btn, false);
      return;
    }
    btn.textContent = `Gợi ý: Đổi sang ${targetLabel(suggested)}`;
    show(btn, true);
  }

  function updateRequestCounter() {
    const len = el("requestInput").value.length;
    const counter = el("requestCharCounter");
    counter.textContent = `${len.toLocaleString("vi-VN")} / 8.000`;
    counter.classList.toggle("is-over", len > 8000);
    updateTargetHint();
  }

  // Error codes that the user fixes in Settings, and transient ones worth a retry (PLAN.md §3).
  const SETTINGS_ERROR_CODES = new Set(["unconfigured", "auth_error", "model_required"]);
  const RETRY_ERROR_CODES = new Set(["rate_limit", "timeout", "bad_response", "network", "internal_error"]);

  // errorCode normalizes thrown fetch errors: no HTTP status means the request never got an answer.
  function errorCode(err) {
    if (err && err.code) return err.code;
    return err && err.status ? "http_error" : "network";
  }

  // showStudioError shows msg under the request box; `code` adds "Mở Cài đặt" / "Thử lại".
  // The typed request and refine text are never cleared on error.
  function showStudioError(msg, code = "") {
    el("studioErrorText").textContent =
      code === "network" ? "Không kết nối được tới Prompt Studio hoặc nhà cung cấp AI. Kiểm tra mạng rồi thử lại." : msg;
    show(el("studioErrorSettingsLink"), SETTINGS_ERROR_CODES.has(code));
    show(el("studioErrorRetryBtn"), RETRY_ERROR_CODES.has(code) && typeof state.retryAction === "function");
    show(el("studioErrorBanner"), true);
  }

  function openMobileSidebar() {
    el("appSidebar").classList.add("is-open");
    el("sidebarBackdrop").classList.add("is-open");
    show(el("closeSidebarBtn"), true);
    el("openSidebarBtn").setAttribute("aria-expanded", "true");
    el("closeSidebarBtn").focus();
  }

  function closeMobileSidebar() {
    el("appSidebar").classList.remove("is-open");
    el("sidebarBackdrop").classList.remove("is-open");
    show(el("closeSidebarBtn"), false);
    el("openSidebarBtn").setAttribute("aria-expanded", "false");
  }

  function bindEvents() {
    // Close assumption popover when clicking outside
    document.addEventListener("click", () => {
      if (state.openAssumptionIndex !== -1) {
        state.openAssumptionIndex = -1;
        const ver = activeVersion();
        if (ver) renderAssumptions(ver.assumptions || []);
      }
    });

    // Global Esc shortcut to stop streaming, close modal, or dismiss assumption popover
    document.addEventListener("keydown", (e) => {
      if (e.key === "Escape") {
        if (state.abortController) {
          state.abortController.abort();
        } else if (el("pasteModal").classList.contains("is-open")) {
          closePasteModal();
        } else if (state.libRenaming) {
          cancelLibraryInlineRename(true);
        } else if (state.openAssumptionIndex !== -1) {
          const prevIdx = state.openAssumptionIndex;
          state.openAssumptionIndex = -1;
          const ver = activeVersion();
          if (ver) renderAssumptions(ver.assumptions || []);
          const chips = el("assumptionsList").querySelectorAll(".assumption-chip");
          if (chips[prevIdx]) chips[prevIdx].focus();
        }
      }
    });

    // Mobile drawer
    el("openSidebarBtn").addEventListener("click", openMobileSidebar);
    el("closeSidebarBtn").addEventListener("click", closeMobileSidebar);
    el("sidebarBackdrop").addEventListener("click", closeMobileSidebar);

    // Navigation
    el("brandHomeLink").addEventListener("click", (e) => {
      e.preventDefault();
      switchView("studio", true);
    });
    el("navStudioBtn").addEventListener("click", () => switchView("studio", true));
    el("navLibraryBtn").addEventListener("click", () => switchView("library", true));
    el("newSessionBtn").addEventListener("click", () => {
      resetStudioToEmpty();
      switchView("studio", true);
      el("requestInput").focus();
    });

    // History search & Undo toast
    let histTimer = null;
    el("historySearchInput").addEventListener("input", () => {
      clearTimeout(histTimer);
      histTimer = setTimeout(loadHistorySessions, 180);
    });
    el("dismissErrorBtn").addEventListener("click", () => show(el("studioErrorBanner"), false));
    el("studioErrorRetryBtn").addEventListener("click", () => {
      show(el("studioErrorBanner"), false);
      if (typeof state.retryAction === "function" && !state.streaming) state.retryAction();
    });

    // Request input, Target Hint & Generate
    el("requestInput").addEventListener("input", updateRequestCounter);
    el("targetSelect").addEventListener("change", () => {
      state.targetManualOverride = true;
      show(el("starterPresetBadge"), false);
      updateTargetHint();
    });
    el("targetHintBtn").addEventListener("click", () => {
      if (!state.suggestedTarget) return;
      const next = state.suggestedTarget;
      el("targetSelect").value = next;
      state.targetManualOverride = true;
      show(el("starterPresetBadge"), false);
      updateTargetHint();
      UI.announce(`Đã chuyển nơi dùng sang ${targetLabel(next)}`);
      el("generateBtn").focus();
    });
    el("requestInput").addEventListener("keydown", (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
        e.preventDefault();
        startGenerate();
      }
    });
    el("generateBtn").addEventListener("click", startGenerate);
    el("stopStreamBtn").addEventListener("click", () => {
      if (state.abortController) state.abortController.abort();
    });

    // Result actions
    el("prevVersionBtn").addEventListener("click", () => {
      if (state.activeVersionIndex > 0) {
        state.activeVersionIndex--;
        state.diffBaseVersionId = "";
        syncTargetSelectToVersion();
        renderStudioState();
      }
    });
    el("nextVersionBtn").addEventListener("click", () => {
      if (state.activeVersionIndex < state.versions.length - 1) {
        state.activeVersionIndex++;
        state.diffBaseVersionId = "";
        syncTargetSelectToVersion();
        renderStudioState();
      }
    });
    UI.bindMenu(el("resultMoreBtn"), el("resultMoreMenu"));
    UI.bindMenu(el("libMoreBtn"), el("libMoreMenu"));

    // Version Diff events (D-P010-2)
    el("toggleDiffBtn").addEventListener("click", () => {
      state.diffOpen = !state.diffOpen;
      if (state.diffOpen) state.diffBaseVersionId = "";
      renderStudioState();
    });
    el("diffBaseSelect").addEventListener("change", (e) => {
      state.diffBaseVersionId = e.target.value;
      renderStudioState();
    });
    el("closeDiffBtn").addEventListener("click", () => {
      state.diffOpen = false;
      renderStudioState();
    });

    // Studio Inline Variable Fill events (D-P010-1)
    el("toggleStudioVarsBtn").addEventListener("click", () => {
      state.studioVarsOpen = !state.studioVarsOpen;
      renderStudioState();
      if (state.studioVarsOpen) {
        const first = el("studioVarsInputs").querySelector("input");
        if (first) first.focus();
      }
    });
    el("clearStudioVarsBtn").addEventListener("click", () => {
      if (state.session) state.sessionVars[state.session.id] = {};
      renderStudioState();
    });
    el("copyFilledStudioBtn").addEventListener("click", () => {
      const ver = activeVersion();
      if (!ver || !state.session) return;
      copyFilledFromContainer(
        el("copyFilledStudioBtn"),
        ver.content,
        state.session.id,
        el("studioVarsInputs"),
        false
      );
    });
    el("copyFilledApiJsonBtn").addEventListener("click", () => {
      const ver = activeVersion();
      if (!ver || !state.session) return;
      copyFilledFromContainer(
        el("copyFilledApiJsonBtn"),
        ver.content,
        state.session.id,
        el("studioVarsInputs"),
        true
      );
    });

    el("regenerateBtn").addEventListener("click", () => startRefine("", "regenerate"));
    el("saveSessionBtn").addEventListener("click", handleOneClickSave);
    el("toggleSaveEditBtn").addEventListener("click", () => {
      const editor = el("saveBarEditor");
      show(editor, editor.classList.contains("is-hidden"));
    });
    el("applySaveEditBtn").addEventListener("click", applySaveEdit);
    el("unsaveSessionBtn").addEventListener("click", unsaveCurrentSession);

    el("manualEditBtn").addEventListener("click", () => {
      const ver = activeVersion();
      if (!ver) return;
      state.diffOpen = false;
      show(el("diffBar"), false);
      el("toggleDiffBtn").setAttribute("aria-pressed", "false");
      el("manualEditTextarea").value = ver.content;
      show(el("promptDisplayContainer"), false);
      show(el("manualEditContainer"), true);
      el("manualEditTextarea").focus();
    });
    el("cancelManualEditBtn").addEventListener("click", () => {
      show(el("manualEditContainer"), false);
      show(el("promptDisplayContainer"), true);
    });
    el("saveManualEditBtn").addEventListener("click", saveManualEdit);

    el("downloadMdBtn").addEventListener("click", () => {
      const ver = activeVersion();
      if (!ver) return;
      const blob = new Blob([ver.content], { type: "text/markdown;charset=utf-8" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${(state.session && state.session.title) || "prompt"}-v${ver.seq}.md`;
      a.click();
      URL.revokeObjectURL(url);
    });

    el("copyPromptBtn").addEventListener("click", () => {
      const ver = activeVersion();
      if (!ver) return;
      copyTextWithFeedback(el("copyPromptBtn"), ver.content);
    });

    el("copyApiJsonBtn").addEventListener("click", () => {
      const ver = activeVersion();
      if (!ver) return;
      const parts = splitAPIPrompt(ver.content);
      const msgs = parts.hasSplit
        ? [
            { role: "system", content: parts.system },
            { role: "user", content: parts.user },
          ]
        : [{ role: "user", content: ver.content }];
      copyTextWithFeedback(el("copyApiJsonBtn"), JSON.stringify(msgs, null, 2));
    });

    // Refine chips & form
    el("quickRefineChips")
      .querySelectorAll("[data-instruction]")
      .forEach((chip) => {
        chip.addEventListener("click", () => {
          startRefine(chip.getAttribute("data-instruction"), "improve");
        });
      });

    // Empty refine box → dimmed send button (PLAN.md §3.5)
    el("refineInput").addEventListener("input", () => {
      el("submitRefineBtn").disabled = !el("refineInput").value.trim();
    });

    el("refineForm").addEventListener("submit", (e) => {
      e.preventDefault();
      const instr = el("refineInput").value.trim();
      if (!instr) return;
      startRefine(instr, "improve");
    });

    // Collections & Library controls
    el("newCollectionBtn").addEventListener("click", () => {
      const f = el("newCollectionForm");
      const opening = f.classList.contains("is-hidden");
      show(f, opening);
      el("newCollectionBtn").setAttribute("aria-expanded", opening ? "true" : "false");
      if (opening) el("newCollectionInput").focus();
    });
    el("newCollectionForm").addEventListener("submit", async (e) => {
      e.preventDefault();
      const name = el("newCollectionInput").value.trim();
      if (!name) return;
      try {
        const col = await API.request("POST", "/api/collections", { name });
        el("newCollectionInput").value = "";
        show(el("newCollectionForm"), false);
        el("newCollectionBtn").setAttribute("aria-expanded", "false");
        state.activeCollectionId = col.id;
        await Promise.all([loadCollections(), loadLibraryList()]);
      } catch (err) {
        UI.toast(`Không tạo được collection: ${err.message}`, { variant: "error" });
      }
    });

    el("renameCollectionBtn").addEventListener("click", async () => {
      const col = state.collections.find((c) => c.id === state.activeCollectionId);
      if (!col || col.isSystem) return;
      const nextName = await UI.prompt({
        title: "Đổi tên collection",
        value: col.name,
        confirmLabel: "Đổi tên",
      });
      if (!nextName) return;
      try {
        await API.request("PATCH", `/api/collections/${col.id}`, { name: nextName });
        await Promise.all([loadCollections(), loadLibraryList()]);
      } catch (err) {
        UI.toast(`Không đổi được tên: ${err.message}`, { variant: "error" });
      }
    });

    el("deleteCollectionBtn").addEventListener("click", async () => {
      const col = state.collections.find((c) => c.id === state.activeCollectionId);
      if (!col || col.isSystem) return;
      const ok = await UI.confirm({
        title: `Xóa collection “${col.name}”?`,
        message: "Các prompt bên trong không bị xóa mà được chuyển về “Chưa phân loại”.",
        confirmLabel: "Xóa collection",
        danger: true,
      });
      if (!ok) return;
      try {
        await API.request("DELETE", `/api/collections/${col.id}`);
        state.activeCollectionId = "all";
        await Promise.all([loadCollections(), loadLibraryList()]);
      } catch (err) {
        UI.toast(`Không xóa được collection: ${err.message}`, { variant: "error" });
      }
    });

    let libSearchTimer = null;
    el("librarySearchInput").addEventListener("input", () => {
      clearTimeout(libSearchTimer);
      libSearchTimer = setTimeout(loadLibraryList, 180);
    });
    el("librarySortSelect").addEventListener("change", loadLibraryList);

    el("libCopyBtn").addEventListener("click", () => {
      const v = activeLibraryVersion();
      if (v) copyTextWithFeedback(el("libCopyBtn"), v.content);
    });

    el("libCopyApiJsonBtn").addEventListener("click", () => {
      const v = activeLibraryVersion();
      if (!v) return;
      const parts = splitAPIPrompt(v.content);
      const msgs = parts.hasSplit
        ? [
            { role: "system", content: parts.system },
            { role: "user", content: parts.user },
          ]
        : [{ role: "user", content: v.content }];
      copyTextWithFeedback(el("libCopyApiJsonBtn"), JSON.stringify(msgs, null, 2));
    });

    el("toggleLibVarsBtn").addEventListener("click", () => {
      state.libVarsOpen = !state.libVarsOpen;
      renderLibraryDetail();
      if (state.libVarsOpen) {
        const first = el("libVarsInputs").querySelector("input");
        if (first) first.focus();
      }
    });

    el("clearLibVarsBtn").addEventListener("click", () => {
      if (state.activeLibraryDetail) {
        state.sessionVars[state.activeLibraryDetail.session.id] = {};
      }
      renderLibraryDetail();
    });

    el("copyFilledLibBtn").addEventListener("click", () => {
      const v = activeLibraryVersion();
      if (!v || !state.activeLibraryDetail) return;
      copyFilledFromContainer(
        el("copyFilledLibBtn"),
        v.content,
        state.activeLibraryDetail.session.id,
        el("libVarsInputs"),
        false
      );
    });

    el("copyFilledLibApiJsonBtn").addEventListener("click", () => {
      const v = activeLibraryVersion();
      if (!v || !state.activeLibraryDetail) return;
      copyFilledFromContainer(
        el("copyFilledLibApiJsonBtn"),
        v.content,
        state.activeLibraryDetail.session.id,
        el("libVarsInputs"),
        true
      );
    });

    el("libOpenInStudioBtn").addEventListener("click", () => {
      if (!state.activeLibraryDetail) return;
      // Open exactly the version being viewed (the saved one by default).
      openSessionInStudio(state.activeLibraryDetail.session.id, state.activeLibraryVersionId);
    });

    el("libMoveCollectionSelect").addEventListener("change", async (e) => {
      if (!state.activeLibraryDetail) return;
      const colId = e.target.value;
      const sessId = state.activeLibraryDetail.session.id;
      try {
        await API.request("PATCH", `/api/sessions/${sessId}`, { collectionId: colId });
        await Promise.all([loadCollections(), loadLibraryList(), selectLibrarySession(sessId)]);
      } catch (err) {
        // Revert the select to the real collection so the UI never lies.
        e.target.value = state.activeLibraryDetail.session.collectionId || "";
        UI.toast(`Không chuyển được collection: ${err.message}`, { variant: "error" });
      }
    });

    el("libInlineRenameBtn").addEventListener("click", openLibraryInlineRename);
    el("libRenamePromptBtn").addEventListener("click", openLibraryInlineRename);
    el("libInlineRenameCancelBtn").addEventListener("click", () => cancelLibraryInlineRename(true));
    el("libInlineRenameForm").addEventListener("submit", (e) => {
      e.preventDefault();
      submitLibraryInlineRename();
    });

    el("libUnsaveBtn").addEventListener("click", async () => {
      if (!state.activeLibraryDetail) return;
      const sess = state.activeLibraryDetail.session;
      const restore = { title: sess.title, collectionId: sess.collectionId || "" };
      const refresh = () =>
        Promise.all([loadCollections(), loadLibraryList(), loadHistorySessions()]);
      try {
        await API.request("DELETE", `/api/sessions/${sess.id}/save`);
        state.activeLibraryDetail = null;
        renderLibraryDetail();
        await refresh();
        UI.toast(`Đã bỏ “${sess.title}” khỏi thư viện. Phiên vẫn còn trong Lịch sử.`, {
          actionLabel: "Hoàn tác",
          onAction: async () => {
            try {
              await API.request("PUT", `/api/sessions/${sess.id}/save`, restore);
              await refresh();
              await selectLibrarySession(sess.id);
            } catch (err) {
              UI.toast(`Không hoàn tác được: ${err.message}`, { variant: "error" });
            }
          },
        });
      } catch (err) {
        UI.toast(`Không bỏ lưu được: ${err.message}`, { variant: "error" });
      }
    });

    el("libDeletePromptBtn").addEventListener("click", async () => {
      if (!state.activeLibraryDetail) return;
      await requestDeleteSession(state.activeLibraryDetail.session);
    });

    // Paste Modal events
    el("openPasteModalBtn").addEventListener("click", openPasteModal);
    el("closePasteModalBtn").addEventListener("click", closePasteModal);
    el("cancelPasteModalBtn").addEventListener("click", closePasteModal);
    el("pasteModalBackdrop").addEventListener("click", closePasteModal);

    el("pasteTitleInput").addEventListener("input", () => {
      state.pasteTitleTouched = Boolean(el("pasteTitleInput").value.trim());
    });

    el("pasteContentInput").addEventListener("input", () => {
      const val = el("pasteContentInput").value;
      const counter = el("pasteCharCounter");
      counter.textContent = `${val.length.toLocaleString("vi-VN")} / 32.000`;
      counter.classList.toggle("is-over", val.length > 32000);

      const vars = extractVariables(val);
      const badge = el("pasteVarsBadge");
      if (vars.length > 0) {
        badge.textContent = `${vars.length} biến: ${vars.join(", ")}`;
        show(badge, true);
      } else {
        show(badge, false);
      }

      if (!state.pasteTitleTouched) {
        el("pasteTitleInput").value = deriveFirstLineTitle(val);
      }
    });

    el("pasteCollectionSelect").addEventListener("change", (e) => {
      show(el("pasteNewCollectionField"), e.target.value === "__new__");
    });

    el("submitPasteBtn").addEventListener("click", submitPasteModal);
    el("openExistingDuplicateBtn").addEventListener("click", async () => {
      const id = state.duplicateSessionId;
      closePasteModal();
      if (id) {
        await switchView("library", true);
        await selectLibrarySession(id);
      }
    });

    window.addEventListener("popstate", () => {
      const want = window.location.pathname === "/library" ? "library" : "studio";
      switchView(want, false);
    });
  }

  document.addEventListener("DOMContentLoaded", init);
})();
