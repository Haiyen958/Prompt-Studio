// settings.js — Prompt Studio v2 Settings & Backup Controller
// Zero inline styles; strictly toggles semantic BEM classes.

(function () {
  const el = (id) => document.getElementById(id);

  let hasStoredKey = false;
  let replacingKey = false;
  let keyTestedOk = false;

  function show(node, visible) {
    if (!node) return;
    node.classList.toggle("is-hidden", !visible);
  }

  function showFeedback(kind, htmlMessage) {
    const banner = el("settingsFeedbackBanner");
    banner.className = `alert alert--${kind}`;
    banner.innerHTML = htmlMessage;
    show(banner, true);
  }

  function escapeHTML(str) {
    return String(str || "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  // numberOr parses a numeric input, using fallback only for empty/non-numeric values.
  function numberOr(raw, fallback) {
    const s = String(raw ?? "").trim();
    const n = Number(s);
    return s !== "" && Number.isFinite(n) ? n : fallback;
  }

  function syncProviderUI() {
    const prov = el("providerSelect").value;
    const isCompat = prov === "openai-compatible";
    el("baseUrlInput").disabled = !isCompat;
    if (!isCompat && prov !== "openai-compatible") {
      el("baseUrlInput").placeholder =
        prov === "anthropic" ? "https://api.anthropic.com/v1" : "https://api.openai.com/v1";
    } else {
      el("baseUrlInput").placeholder = "http://127.0.0.1:11434/v1";
    }
  }

  function syncKeyDisplay(keyHint) {
    if (hasStoredKey && !replacingKey) {
      el("keyHintText").textContent = keyHint || "••••••••••";
      show(el("storedKeyBar"), true);
      show(el("keyInputGroup"), false);
      show(el("cancelReplaceKeyBtn"), false);
    } else {
      show(el("storedKeyBar"), false);
      show(el("keyInputGroup"), true);
      show(el("cancelReplaceKeyBtn"), hasStoredKey && replacingKey);
    }
  }

  async function loadSettings() {
    try {
      const s = await API.request("GET", "/api/settings");
      applyThemePreference(s.theme || "dark");

      el("providerSelect").value = s.provider || "openai";
      el("baseUrlInput").value = s.baseUrl || "";
      el("modelInput").value = s.model || "gpt-4o-mini";
      el("temperatureInput").value = s.temperature ?? 0.4;
      el("maxTokensInput").value = s.maxTokens ?? 2000;
      el("timeoutInput").value = s.timeoutSec ?? 60;
      el("promptLanguageSelect").value = s.promptLanguage || "match";
      el("themeSelect").value = s.theme || "dark";

      hasStoredKey = Boolean(s.hasKey);
      replacingKey = false;
      keyTestedOk = false;
      el("apiKeyInput").value = "";

      syncProviderUI();
      syncKeyDisplay(s.keyHint);
    } catch (err) {
      showFeedback("error", `<span>Không thể tải cấu hình: ${escapeHTML(err.message)}</span>`);
    }
  }

  async function handleTestConnection() {
    const btn = el("testConnectionBtn");
    btn.disabled = true;
    btn.textContent = "Đang kiểm tra…";

    try {
      const res = await API.request("POST", "/api/settings/test", {
        provider: el("providerSelect").value,
        baseUrl: el("baseUrlInput").value.trim(),
        model: el("modelInput").value.trim(),
        apiKey: el("apiKeyInput").value.trim(),
        timeoutSec: Number(el("timeoutInput").value) || 15,
      });

      if (res.ok) {
        keyTestedOk = true;
        showFeedback(
          "success",
          `<span><strong>Kết nối thành công.</strong> ${res.latencyMs ?? 0} ms.</span>`
        );
      } else {
        keyTestedOk = false;
        showFeedback(
          "warning",
          `<span><strong>Kiểm tra kết nối thất bại:</strong> ${escapeHTML(res.error || "Lỗi không xác định")}. Bạn vẫn có thể bấm Lưu nếu muốn cấu hình trước.</span>`
        );
      }
    } catch (err) {
      keyTestedOk = false;
      showFeedback("error", `<span>${escapeHTML(err.message)}</span>`);
    } finally {
      btn.disabled = false;
      btn.textContent = "Test Connection";
    }
  }

  async function saveAllSettings(e) {
    if (e) e.preventDefault();
    const payload = {
      provider: el("providerSelect").value,
      baseUrl: el("baseUrlInput").value.trim(),
      model: el("modelInput").value.trim(),
      // 0 is a valid temperature: only fall back when the field is empty or not a number.
      temperature: numberOr(el("temperatureInput").value, 0.4),
      maxTokens: Number(el("maxTokensInput").value) || 2000,
      timeoutSec: Number(el("timeoutInput").value) || 60,
      promptLanguage: el("promptLanguageSelect").value,
      theme: el("themeSelect").value,
      tested: keyTestedOk,
    };

    const typedKey = el("apiKeyInput").value.trim();
    if (typedKey) {
      payload.apiKey = typedKey;
    }

    try {
      const updated = await API.request("PUT", "/api/settings", payload);
      applyThemePreference(updated.theme || "dark");
      hasStoredKey = Boolean(updated.hasKey);
      replacingKey = false;
      el("apiKeyInput").value = "";
      syncKeyDisplay(updated.keyHint);

      if (updated.warning) {
        showFeedback("warning", `<span>${escapeHTML(updated.warning)}</span>`);
      } else {
        showFeedback("success", `<span><strong>Đã lưu cấu hình thành công.</strong></span>`);
      }
    } catch (err) {
      showFeedback("error", `<span>Lỗi khi lưu: ${escapeHTML(err.message)}</span>`);
    }
  }

  async function handleDeleteKey() {
    const ok = await UI.confirm({
      title: "Xóa API key?",
      message: "Key đã lưu trên máy này sẽ bị xóa. Studio sẽ không tạo được prompt cho tới khi bạn nhập key mới.",
      confirmLabel: "Xóa key",
      danger: true,
    });
    if (!ok) return;
    try {
      await API.request("DELETE", "/api/settings/key");
      hasStoredKey = false;
      replacingKey = false;
      keyTestedOk = false;
      el("apiKeyInput").value = "";
      syncKeyDisplay("");
      showFeedback("info", `<span>Đã xóa API key khỏi máy.</span>`);
    } catch (err) {
      showFeedback("error", `<span>${escapeHTML(err.message)}</span>`);
    }
  }

  function handleExportData() {
    window.location.href = "/api/export";
  }

  async function handleImportFile(file) {
    if (!file) return;
    try {
      const raw = await file.text();
      const parsed = JSON.parse(raw);
      const res = await API.request("POST", "/api/import", parsed);
      await loadSettings();
      showFeedback(
        "success",
        `<span><strong>Khôi phục dữ liệu thành công:</strong> ${res.sessions || 0} phiên và ${res.collections || 0} collection.</span>`
      );
    } catch (err) {
      showFeedback("error", `<span>Không thể nhập dữ liệu: ${escapeHTML(err.message)}</span>`);
    } finally {
      el("importFileInput").value = "";
    }
  }

  document.addEventListener("DOMContentLoaded", () => {
    loadSettings();

    el("providerSelect").addEventListener("change", () => {
      keyTestedOk = false;
      const prov = el("providerSelect").value;
      if (prov === "anthropic" && el("modelInput").value === "gpt-4o-mini") {
        el("modelInput").value = "claude-3-5-sonnet-latest";
      } else if (prov === "openai" && el("modelInput").value.startsWith("claude")) {
        el("modelInput").value = "gpt-4o-mini";
      } else if (prov === "openai-compatible" && el("modelInput").value === "gpt-4o-mini") {
        el("modelInput").value = "llama3.1";
      }
      syncProviderUI();
    });

    el("apiKeyInput").addEventListener("input", () => {
      keyTestedOk = false;
    });

    el("replaceKeyBtn").addEventListener("click", () => {
      replacingKey = true;
      keyTestedOk = false;
      syncKeyDisplay("");
      el("apiKeyInput").focus();
    });

    el("cancelReplaceKeyBtn").addEventListener("click", () => {
      replacingKey = false;
      el("apiKeyInput").value = "";
      loadSettings();
    });

    el("deleteKeyBtn").addEventListener("click", handleDeleteKey);
    el("testConnectionBtn").addEventListener("click", handleTestConnection);
    el("aiSettingsForm").addEventListener("submit", saveAllSettings);

    // Auto-save theme & promptLanguage immediately when changed
    el("themeSelect").addEventListener("change", () => {
      applyThemePreference(el("themeSelect").value);
      saveAllSettings();
    });
    el("promptLanguageSelect").addEventListener("change", () => {
      saveAllSettings();
    });

    el("exportDataBtn").addEventListener("click", handleExportData);
    el("importDataBtn").addEventListener("click", () => el("importFileInput").click());
    el("importFileInput").addEventListener("change", (e) => {
      const f = e.target.files && e.target.files[0];
      if (f) handleImportFile(f);
    });
  });
})();
