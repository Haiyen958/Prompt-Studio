package server

import (
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	SessionCookieName = "ps_token"
	MaxBodyBytes      = 4 * 1024 * 1024 // 4 MB (supports JSON backup import & 32k pasted prompts)
)

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

		// 2. Startup ?token=... exchange into HttpOnly cookie + clean URL redirect
		if cfg.SessionToken != "" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			qToken := r.URL.Query().Get("token")
			if qToken != "" && qToken == cfg.SessionToken {
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

			// Also bind HttpOnly cookie on direct same-origin local navigation when Sec-Fetch-Site is not cross-site
			secSite := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")))
			if secSite != "cross-site" && !strings.HasPrefix(r.URL.Path, "/api/") {
				if c, err := r.Cookie(SessionCookieName); err != nil || c.Value != cfg.SessionToken {
					setSessionCookie(w, cfg.SessionToken)
				}
			}
		}

		isWrite := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete
		if isWrite {
			if cfg.RequireToken && cfg.SessionToken != "" {
				tokenOK := false
				if c, err := r.Cookie(SessionCookieName); err == nil && c.Value == cfg.SessionToken {
					tokenOK = true
				}
				if hdr := r.Header.Get("X-Prompt-Studio-Token"); hdr != "" && hdr == cfg.SessionToken {
					tokenOK = true
				}
				if !tokenOK {
					WriteError(w, http.StatusUnauthorized, "unauthorized_local_session", "Phiên làm việc cục bộ chưa được xác thực — hãy tải lại trang từ URL khởi động")
					return
				}
			}
			// Browsers can send text/plain, form or multipart bodies cross-origin without a
			// CORS preflight; requiring JSON closes that CSRF path for every write endpoint.
			if !isJSONOrEmptyBody(r) {
				WriteError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Yêu cầu ghi phải dùng Content-Type: application/json")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
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
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
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
