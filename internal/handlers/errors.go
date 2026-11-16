package handlers

import (
	"errors"
	"log"
	"net/http"

	"prompt-studio/internal/ai"
	"prompt-studio/internal/server"
	"prompt-studio/internal/storage"
	"prompt-studio/internal/studio"
)

// Error contract (PLAN.md §7): every error response is {"code", "error"}; the frontend
// branches on `code`, never on the message. Unexpected errors are logged server-side and
// answered with a generic message so SQL/driver details never reach the browser.

const internalErrorMessage = "Lỗi máy chủ nội bộ — xem log của Prompt Studio để biết chi tiết"

// writeServiceError maps service/storage errors to HTTP responses.
// notFoundMsg is the user-facing message for storage.ErrNotFound on this route.
func writeServiceError(w http.ResponseWriter, r *http.Request, err error, notFoundMsg string) {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		server.WriteError(w, http.StatusNotFound, "not_found", notFoundMsg)
	case errors.Is(err, storage.ErrVersionMismatch):
		server.WriteError(w, http.StatusBadRequest, "version_mismatch", "Phiên bản không thuộc phiên này")
	case errors.Is(err, storage.ErrProtectedResource):
		server.WriteError(w, http.StatusForbidden, "protected_collection", "Không thể sửa hoặc xóa collection hệ thống")
	case errors.Is(err, studio.ErrInvalidInput):
		server.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		writeInternalError(w, r, err)
	}
}

func writeInternalError(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("[ERROR] %s %s: %v", r.Method, r.URL.Path, err)
	server.WriteError(w, http.StatusInternalServerError, "internal_error", internalErrorMessage)
}

// sseErrorPayload is the data of the SSE `error` event: a stable code plus a Vietnamese
// message the UI can show as is.
func sseErrorPayload(r *http.Request, err error) map[string]any {
	code := ai.ErrorCode(err)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		code = "not_found"
	case errors.Is(err, studio.ErrInvalidInput):
		code = "invalid_request"
	}
	return map[string]any{"code": code, "message": userMessage(r, code, err)}
}

func userMessage(r *http.Request, code string, err error) string {
	switch code {
	case "unconfigured":
		return "Chưa cấu hình AI — hãy nhập API key trong Cài đặt."
	case "model_required":
		return "Chưa chọn model — hãy chọn model trong Cài đặt (bấm “Tải danh sách model”)."
	case "auth_error":
		return "API key không hợp lệ hoặc đã hết hạn (401/403). Kiểm tra lại trong Cài đặt."
	case "rate_limit":
		return "Nhà cung cấp AI đang giới hạn tốc độ hoặc đã hết hạn mức (429). Hãy thử lại sau ít phút."
	case "timeout":
		return "AI không phản hồi trong thời gian chờ. Phần đã viết (nếu có) được lưu thành bản chưa xong — hãy thử lại."
	case "canceled":
		return "Đã dừng."
	case "bad_response":
		// The upstream status/body (e.g. "model not found") is what the user needs to fix it.
		return "AI trả về phản hồi không hợp lệ: " + err.Error()
	case "not_found":
		return "Không tìm thấy phiên hoặc phiên bản."
	case "invalid_request":
		return err.Error()
	default:
		log.Printf("[ERROR] %s %s (stream): %v", r.Method, r.URL.Path, err)
		return internalErrorMessage
	}
}
