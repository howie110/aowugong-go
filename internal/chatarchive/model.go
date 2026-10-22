// Package chatarchive 提供个人聊天归档的只读查询，不接收公网写入。
package chatarchive

import (
	"encoding/json"
	"errors"
	"time"
)

var ErrInvalidFilter = errors.New("invalid archive filter")

type Filter struct {
	Source, From, To, Title, Query string
	Limit, Offset                  int
}

func (f Filter) Validate() error {
	if f.Limit < 1 || f.Limit > 200 || f.Offset < 0 || f.Offset > 1000000 || len(f.Title) > 500 || len(f.Query) > 500 {
		return ErrInvalidFilter
	}
	if f.Source != "" && f.Source != "codex" && f.Source != "claude-code" {
		return ErrInvalidFilter
	}
	for _, value := range []string{f.From, f.To} {
		if value != "" {
			if _, err := time.Parse("2006-01-02", value); err != nil {
				return ErrInvalidFilter
			}
		}
	}
	if f.From != "" && f.To != "" && f.From > f.To {
		return ErrInvalidFilter
	}
	return nil
}

type Conversation struct {
	AccountScope string `json:"account_scope"`
	ID           int64  `json:"id"`
	Source       string `json:"source"`
	SourceID     string `json:"source_id"`
	Title        string `json:"title"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type Message struct {
	ID              int64           `json:"id"`
	MessageKey      string          `json:"message_key"`
	VersionHash     string          `json:"version_hash"`
	ParentKey       *string         `json:"parent_key"`
	Role            string          `json:"role"`
	Body            string          `json:"body"`
	Blocks          json.RawMessage `json:"blocks"`
	Attachments     json.RawMessage `json:"attachments"`
	Completeness    string          `json:"completeness"`
	IdentityQuality string          `json:"identity_quality"`
	SourceTime      *string         `json:"source_time"`
	BatchID         int64           `json:"batch_id"`
	Position        int64           `json:"position"`
}
