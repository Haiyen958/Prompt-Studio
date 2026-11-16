// api.js — HTTP & Server-Sent Events (SSE) client for Prompt Studio

const API = {
  async request(method, path, body) {
    const opts = {
      method,
      headers: {},
      credentials: "same-origin",
    };
    if (body !== undefined) {
      opts.headers["Content-Type"] = "application/json";
      opts.body = JSON.stringify(body);
    }
    const res = await fetch(path, opts);
    if (res.status === 204) return null;
    let data = null;
    try {
      data = await res.json();
    } catch (_) {
      data = null;
    }
    if (!res.ok) {
      const err = new Error((data && data.error) || `Request failed (${res.status})`);
      err.status = res.status;
      err.code = data && data.code;
      err.data = data;
      throw err;
    }
    return data;
  },

  // streamSSE posts JSON to `path` and parses event/data frames until completion or abort.
  async streamSSE(path, body, handlers, abortSignal) {
    const res = await fetch(path, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Accept: "text/event-stream",
      },
      credentials: "same-origin",
      body: JSON.stringify(body),
      signal: abortSignal,
    });

    if (!res.ok) {
      let errPayload = {};
      try {
        errPayload = await res.json();
      } catch (_) {}
      const err = new Error(errPayload.error || `HTTP ${res.status}`);
      err.code = errPayload.code || "http_error";
      err.status = res.status;
      throw err;
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder("utf-8");
    let buffer = "";
    let currentEvent = "message";

    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });

      let boundary;
      while ((boundary = buffer.indexOf("\n\n")) !== -1) {
        const rawFrame = buffer.slice(0, boundary);
        buffer = buffer.slice(boundary + 2);

        const lines = rawFrame.split("\n");
        let evtName = currentEvent;
        let dataLines = [];
        for (const line of lines) {
          if (line.startsWith("event:")) {
            evtName = line.slice(6).trim();
          } else if (line.startsWith("data:")) {
            dataLines.push(line.slice(5).trim());
          }
        }
        if (dataLines.length > 0) {
          const joined = dataLines.join("\n");
          let parsed = joined;
          try {
            parsed = JSON.parse(joined);
          } catch (_) {}
          if (handlers && typeof handlers[evtName] === "function") {
            handlers[evtName](parsed);
          }
        }
      }
    }
  },
};

function applyThemePreference(theme) {
  const root = document.documentElement;
  if (theme === "system") {
    const prefersLight = window.matchMedia && window.matchMedia("(prefers-color-scheme: light)").matches;
    root.setAttribute("data-theme", prefersLight ? "light" : "dark");
  } else if (theme === "light") {
    root.setAttribute("data-theme", "light");
  } else {
    root.setAttribute("data-theme", "dark");
  }
}
