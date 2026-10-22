package httpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/chatarchive"
	"golang.org/x/time/rate"
)

type archiveSyncService interface {
	LegacyCounts(context.Context) (chatarchive.LegacyCounts, error)
	SyncState(context.Context, string) (*chatarchive.SyncState, error)
	Upload(context.Context, chatarchive.UploadRequest) error
	VerifyIngest(context.Context) (chatarchive.VerifyResult, error)
}

const archiveSyncMaxBody = 16 << 20

// Optional so existing sync-only implementations do not need reconciliation stubs.
type archiveReconciliationService interface {
	Check(context.Context, chatarchive.CheckRequest) (chatarchive.CheckResult, error)
	Repair(context.Context, chatarchive.RepairRequest) (chatarchive.RepairResult, error)
}

func registerArchiveSyncRoutes(api chi.Router, service archiveSyncService, key string) {
	expected := sha256.Sum256([]byte(key))
	enabled := len(key) >= 32 && len(key) <= 4096 && strings.IndexFunc(key, func(r rune) bool { return r <= 32 || r >= 127 }) < 0
	limiter := rate.NewLimiter(10, 120)
	slots := make(chan struct{}, 2)
	handle := func(action string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			parts := strings.Fields(r.Header.Get("Authorization"))
			supplied := ""
			if len(parts) == 2 {
				supplied = parts[1]
			}
			actual := sha256.Sum256([]byte(supplied))
			match := subtle.ConstantTimeCompare(expected[:], actual[:])
			if !enabled || len(r.Header.Values("Authorization")) != 1 || len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || match != 1 {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeError(w, 401, "unauthorized", "Archive sync authentication required")
				return
			}
			if !limiter.Allow() {
				w.Header().Set("Retry-After", "1")
				writeError(w, 429, "rate_limited", "Archive sync busy")
				return
			}
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			default:
				w.Header().Set("Retry-After", "1")
				writeError(w, 429, "rate_limited", "Archive sync busy")
				return
			}
			if service == nil {
				writeError(w, 503, "unavailable", "Archive sync unavailable")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
			defer cancel()
			var result any
			var err error
			switch action {
			case "check", "repair":
				reconciler, ok := service.(archiveReconciliationService)
				if !ok {
					writeError(w, 503, "unavailable", "Archive reconciliation unavailable")
					return
				}
				if action == "check" {
					var input chatarchive.CheckRequest
					if !decodeArchiveSync(w, r, &input) {
						return
					}
					if err = input.Validate(); err == nil {
						result, err = reconciler.Check(ctx, input)
					}
				} else {
					var input chatarchive.RepairRequest
					if !decodeArchiveSync(w, r, &input) {
						return
					}
					if err = input.Validate(); err == nil {
						result, err = reconciler.Repair(ctx, input)
					}
				}
			case "status":
				result, err = service.LegacyCounts(ctx)
			case "verify":
				result, err = service.VerifyIngest(ctx)
			case "state":
				var input struct {
					SourceKey string `json:"source_key"`
				}
				if !decodeArchiveSync(w, r, &input) {
					return
				}
				if !chatarchive.ValidSourceKey(input.SourceKey) {
					syncError(w, chatarchive.ErrInvalidIngest)
					return
				}
				var state *chatarchive.SyncState
				state, err = service.SyncState(ctx, input.SourceKey)
				result = struct {
					State *chatarchive.SyncState `json:"state"`
				}{state}
			case "upload":
				var input chatarchive.UploadRequest
				if !decodeArchiveSync(w, r, &input) {
					return
				}
				if err = input.Validate(); err == nil {
					err = service.Upload(ctx, input)
				}
				result = struct {
					Uploaded bool `json:"uploaded"`
				}{true}
			}
			if err != nil {
				syncError(w, err)
				return
			}
			writeJSON(w, 200, result)
		}
	}
	base := "/api/v1/chat-archive-sync"
	api.Get(base+"/status", handle("status"))
	api.Post(base+"/state", handle("state"))
	api.Post(base+"/upload", handle("upload"))
	api.Post(base+"/check", handle("check"))
	api.Post(base+"/repair", handle("repair"))
	api.Get(base+"/verify", handle("verify"))
}

func syncError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, chatarchive.ErrInvalidIngest):
		field := "request"
		var detail *chatarchive.IngestValidationError
		if errors.As(err, &detail) {
			field = detail.Field
		}
		slog.Warn("chat_archive_sync_rejected", "field", field)
		writeError(w, 400, "invalid_request", "Invalid archive sync request: "+field)
	case errors.Is(err, chatarchive.ErrIngestConflict):
		writeError(w, 409, "sync_conflict", "Archive sync conflict")
	default:
		writeError(w, 500, "sync_failed", "Archive sync failed")
	}
}

func decodeArchiveSync(w http.ResponseWriter, r *http.Request, target any) bool {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, archiveSyncMaxBody))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, 413, "payload_too_large", "Archive sync request too large")
		return false
	}
	if err != nil || !utf8.Valid(raw) || !validSyncEscapes(raw) {
		syncError(w, &chatarchive.IngestValidationError{Field: "json.encoding"})
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := strictSyncJSON(decoder, 0)
	if err == nil {
		_, err = decoder.Token()
		if err == io.EOF {
			err = nil
		} else {
			err = chatarchive.ErrInvalidIngest
		}
	}
	if err != nil {
		syncError(w, &chatarchive.IngestValidationError{Field: "json.syntax_or_duplicate_keys"})
		return false
	}
	// 只兼容上传 state 的附加客户端元数据，不保存、不参与进度比较。
	// 必填字段及其类型仍由后续结构与业务校验检查；其他层级保持严格。
	if _, upload := target.(*chatarchive.UploadRequest); upload {
		if object, ok := value.(map[string]any); ok {
			if state, ok := object["state"].(map[string]any); ok {
				for key := range state {
					switch key {
					case "offset", "prefix_sha256", "conversation_id", "parser_version", "native_user_backfill_v1":
					default:
						delete(state, key)
					}
				}
			}
		}
	}
	if issues := syncJSONShapeIssues(value, reflect.TypeOf(target).Elem()); issues != "" {
		syncError(w, &chatarchive.IngestValidationError{Field: issues})
		return false
	}
	// 使用白名单过滤后的结构解码，额外 state 字段不会进入持久化对象。
	normalized, marshalErr := json.Marshal(value)
	if marshalErr != nil {
		syncError(w, &chatarchive.IngestValidationError{Field: "json.types"})
		return false
	}
	decoder = json.NewDecoder(bytes.NewReader(normalized))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		syncError(w, &chatarchive.IngestValidationError{Field: "json.types"})
		return false
	}
	return true
}

// Reject duplicate keys, unknown/missing fields and null scalar values at every depth.
func strictSyncJSON(d *json.Decoder, depth int) (any, error) {
	if depth > 32 {
		return nil, chatarchive.ErrInvalidIngest
	}
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delimiter {
	case '{':
		object := map[string]any{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, chatarchive.ErrInvalidIngest
			}
			if _, exists := object[name]; exists {
				return nil, chatarchive.ErrInvalidIngest
			}
			value, err := strictSyncJSON(d, depth+1)
			if err != nil {
				return nil, err
			}
			object[name] = value
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return nil, chatarchive.ErrInvalidIngest
		}
		return object, nil
	case '[':
		array := []any{}
		for d.More() {
			value, err := strictSyncJSON(d, depth+1)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return nil, chatarchive.ErrInvalidIngest
		}
		return array, nil
	default:
		return nil, chatarchive.ErrInvalidIngest
	}
}
func syncJSONShape(v any, t reflect.Type) bool {
	return syncJSONShapeIssues(v, t) == ""
}

// 只输出 Go 类型声明中的字段名、数组下标和固定错误类别，最多32项。
// 客户端额外键名和值都不进入日志或响应。
func syncJSONShapeIssues(v any, t reflect.Type) string {
	issues := []string{}
	var walk func(any, reflect.Type, string)
	walk = func(v any, t reflect.Type, path string) {
		if len(issues) >= 32 {
			return
		}
		add := func(kind string) {
			if len(issues) < 32 {
				issues = append(issues, "json.structure."+path+"."+kind)
			}
		}
		if t.Kind() == reflect.Pointer {
			if v != nil {
				walk(v, t.Elem(), path)
			}
			return
		}
		switch t.Kind() {
		case reflect.Struct:
			m, ok := v.(map[string]any)
			if !ok {
				add("type")
				return
			}
			known := 0
			for i := 0; i < t.NumField(); i++ {
				field := t.Field(i)
				name, options, _ := strings.Cut(field.Tag.Get("json"), ",")
				value, exists := m[name]
				if !exists {
					if options == "omitempty" {
						continue
					}
					if len(issues) < 32 {
						issues = append(issues, "json.structure."+path+"."+name+".missing")
					}
					continue
				}
				known++
				walk(value, field.Type, path+"."+name)
			}
			if len(m) > known {
				add("extra_fields")
			}
		case reflect.Slice:
			a, ok := v.([]any)
			if !ok {
				add("type")
				return
			}
			for i, value := range a {
				walk(value, t.Elem(), fmt.Sprintf("%s[%d]", path, i))
				if len(issues) >= 32 {
					break
				}
			}
		case reflect.String:
			if _, ok := v.(string); !ok {
				add("type")
			}
		case reflect.Int64:
			if _, ok := v.(json.Number); !ok {
				add("type")
			}
		case reflect.Interface:
		default:
			add("type")
		}
	}
	walk(v, t, "$")
	return strings.Join(issues, "; ")
}

// encoding/json replaces isolated surrogates; reject them rather than silently
// changing text and computing a different hash from the sender's Unicode value.
func validSyncEscapes(raw []byte) bool {
	hex4 := func(p []byte) (uint16, bool) {
		if len(p) < 4 {
			return 0, false
		}
		var v uint16
		for _, c := range p[:4] {
			v <<= 4
			switch {
			case c >= '0' && c <= '9':
				v += uint16(c - '0')
			case c >= 'a' && c <= 'f':
				v += uint16(c - 'a' + 10)
			case c >= 'A' && c <= 'F':
				v += uint16(c - 'A' + 10)
			default:
				return 0, false
			}
		}
		return v, true
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		v, ok := hex4(raw[i+1:])
		if !ok {
			return false
		}
		i += 4
		if v >= 0xdc00 && v <= 0xdfff {
			return false
		}
		if v >= 0xd800 && v <= 0xdbff {
			if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return false
			}
			low, ok := hex4(raw[i+3:])
			if !ok || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}
