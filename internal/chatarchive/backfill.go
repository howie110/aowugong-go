package chatarchive

import "slices"

// 补传只增加历史消息标记，不改变原始扫描进度；标记必须与实际消息逐个对应。
func (u UploadRequest) validateBackfill() (bool, error) {
	old := []string(nil)
	if u.ExpectedState != nil {
		old = u.ExpectedState.BackfillMarkers
	}
	next := u.State.BackfillMarkers
	if len(next) == 0 || slices.Equal(old, next) {
		return false, nil
	}
	reject := func() (bool, error) { return false, invalidIngestField("state.native_user_backfill_v1") }
	previous := u.ExpectedState
	if previous == nil || u.Source != "codex" || previous.Offset != u.State.Offset || previous.PrefixSHA256 != u.State.PrefixSHA256 || previous.ConversationID != u.State.ConversationID || previous.ParserVersion != u.State.ParserVersion {
		return reject()
	}
	markers := map[string]bool{}
	for _, m := range old {
		markers[m] = true
	}
	added := 0
	for _, b := range u.Batches {
		for _, m := range b.Messages {
			if m.Role != "user" {
				return reject()
			}
			marker := digestString(pythonJSON([]any{m.Position, m.MessageKey, m.Body}, false))
			if markers[marker] {
				return reject()
			}
			markers[marker] = true
			added++
		}
	}
	if added == 0 || len(next) != len(markers) {
		return reject()
	}
	for _, m := range next {
		if !markers[m] {
			return reject()
		}
	}
	return true, nil
}
