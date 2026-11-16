package server

import (
	"crypto/subtle"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	SessionCookieName = "ps_token"
	MaxBodyBytes      = 4 * 1024 * 1024 // 4 MB (32k pasted prompts and every other JSON write)
	// MaxImportBytes caps POST /api/import: a full backup is far larger than any other write,
	// and GET /api/export has no size limit, so restore must accept what export produces.
	MaxImportBytes = 32 * 1024 * 1024
	// sessionCookieMaxAge keeps the browser signed in across restarts (the token is stable).
	sessionCookieMaxAge = 400 * 24 * 60 * 60

	importPath = "/api/import"
)

const unauthorizedPageHTML = `<!doctype html>
<html lang="vi"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Prompt Studio — cần xác thực</title></head>
<body><main><h1>Chưa xác thực phiên làm việc</h1>
<p>Hãy mở đường dẫn có <code>?token=…</code> được in ra ở terminal khi khởi động Prompt Studio.
Bạn chỉ cần làm việc này một lần trên trình duyệt này.</p></main></body></html>
`

type SecurityConfig struct {
	SessionToken string
	RequireToken bool
}

// Middleware enforces DNS-rebinding Host checks and Origin checks on EVERY request,
// exchanges startup `?token=...` into an HttpOnly cookie, enforces session token on API writes,
// caps request body size, and logs API calls safely without exposing keys or prompts.
func Middleware(cfg SecurityConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[ERROR] panic recovered on %s %s: %v", r.Method, r.URL.Path, rec)
				WriteError(w, http.StatusInternalServerError, "internal_error", "Lỗi máy chủ nội bộ")
			}
		}()

		// 1. Check Host & Origin on EVERY request (DNS rebinding & cross-origin defense — PLAN.md §2)
		if !isAllowedOriginAndHost(r) {
			WriteError(w, http.StatusForbidden, "forbidden_origin_or_host", "Yêu cầu bị từ chối do Host hoặc Origin không hợp lệ")
			return
		}

		// 2. Startup ?token=... exchange into HttpOnly cookie + clean URL redirect.
		// This is the ONLY way a browser obtains the cookie.
		if cfg.SessionToken != "" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			qToken := r.URL.Query().Get("token")
			if qToken != "" && tokenEqual(qToken, cfg.SessionToken) {
				setSessionCookie(w, cfg.SessionToken)
				cleanQ := r.URL.Query()
				cleanQ.Del("token")
				target := r.URL.Path
				if target == "" {
					target = "/"
				}
				if enc := cleanQ.Encode(); enc != "" {
					target += "?" + enc
				}
				http.Redirect(w, r, target, http.StatusFound)
				return
			}
		}

		// 3. Every API call (reads included: they expose prompts and the export) and every
		// HTML page requires the session token. Static JS/CSS stay public: they hold no data.
		if cfg.RequireToken && cfg.SessionToken != "" && !hasValidToken(r, cfg.SessionToken) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				WriteError(w, http.StatusUnauthorized, "unauthorized_local_session", "Phiên làm việc cục bộ chưa được xác thực — hãy mở lại URL khởi động (có ?token=…) in ở terminal")
				return
			}
			if isPagePath(r.URL.Path) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(unauthorizedPageHTML))
				return
			}
		}

		isWrite := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete
		if isWrite {
			// Browsers can send text/plain, form or multipart bodies cross-origin without a
			// CORS preflight; requiring JSON closes that CSRF path for every write endpoint.
			if !isJSONOrEmptyBody(r) {
				WriteError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Yêu cầu ghi phải dùng Content-Type: application/json")
				return
			}
			limit := int64(MaxBodyBytes)
			if r.Method == http.MethodPost && r.URL.Path == importPath {
				limit = MaxImportBytes
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}

		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)

		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("[HTTP] %s %s -> %d (%dms)", r.Method, r.URL.Path, sw.status, time.Since(start).Milliseconds())
		}
	})
}

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   sessionCookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func tokenEqual(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// hasValidToken accepts the HttpOnly cookie (browser) or the X-Prompt-Studio-Token header (scripts/tests).
func hasValidToken(r *http.Request, token string) bool {
	if c, err := r.Cookie(SessionCookieName); err == nil && tokenEqual(c.Value, token) {
		return true
	}
	if hdr := r.Header.Get("X-Prompt-Studio-Token"); hdr != "" && tokenEqual(hdr, token) {
		return true
	}
	return false
}

// isPagePath reports whether path serves an HTML page (which bootstraps data via the API).
func isPagePath(path string) bool {
	switch path {
	case "/", "/library", "/settings", "/showcase":
		return true
	}
	return strings.HasSuffix(path, ".html")
}

func isAllowedOriginAndHost(r *http.Request) bool {
	host := strings.ToLower(strings.TrimSpace(r.Host))
	if host != "" && !isLoopbackHost(host) {
		return false
	}

	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	// The Origin must be this exact app (scheme host:port). Any other loopback port is a
	// different site (a dev server, another local app) and must not drive our API.
	originHost := strings.ToLower(u.Host)
	return host != "" && originHost == host
}

func isJSONOrEmptyBody(r *http.Request) bool {
	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if ct == "" {
		return r.ContentLength == 0
	}
	mediaType := strings.TrimSpace(strings.SplitN(ct, ";", 2)[0])
	return mediaType == "application/json"
}

func isLoopbackHost(hostPort string) bool {
	h := hostPort
	if idx := strings.LastIndexByte(h, ':'); idx >= 0 && !strings.Contains(h, "]") {
		h = h[:idx]
	}
	h = strings.Trim(h, "[]")
	return h == "127.0.0.1" || h == "localhost" || h == "::1" || strings.HasSuffix(h, ".localhost")
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
