package notification

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

type countingSender struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (s *countingSender) SendText(context.Context, string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.err
}

// 并发重试和服务重建后仍必须只发送一次，去重不能仅依赖内存。
func TestDispatchDeduplicatesAndRejectsChangedPayload(t *testing.T) {
	db := testdatabase.Open(t)
	sender := &countingSender{}
	s := NewService(NewRepository(db), sender)
	in := DispatchInput{Source: "receipt-split", RequestID: "bill-1", Title: "完成", Content: "已处理"}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Dispatch(context.Background(), in); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	result, err := NewService(NewRepository(db), sender).Dispatch(context.Background(), in)
	if err != nil || result.Status != "success" || !result.Duplicate || sender.calls != 1 {
		t.Fatalf("result=%+v err=%v sends=%d", result, err, sender.calls)
	}
	in.Content = "changed"
	if _, err = s.Dispatch(context.Background(), in); !errors.Is(err, ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM notification_log WHERE source = ? AND request_id = ?", in.Source, in.RequestID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("logs=%d err=%v", count, err)
	}
}

func TestDispatchUnknownDoesNotRetryOrLeakError(t *testing.T) {
	db := testdatabase.Open(t)
	sender := &countingSender{err: errors.New("https://example.invalid?key=SECRET")}
	s := NewService(NewRepository(db), sender)
	in := DispatchInput{Source: "test", RequestID: "timeout", Title: "test", Content: "body"}
	for range 2 {
		result, err := s.Dispatch(context.Background(), in)
		if err != nil || result.Status != "unknown" {
			t.Fatalf("%+v %v", result, err)
		}
	}
	if sender.calls != 1 {
		t.Fatalf("sent %d times", sender.calls)
	}
	var detail string
	if err := db.QueryRow("SELECT error_message FROM notification_log WHERE request_id = ?", in.RequestID).Scan(&detail); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(detail, "SECRET") {
		t.Fatal("leaked upstream secret")
	}
}

func TestDispatchValidationAndRateLimit(t *testing.T) {
	db := testdatabase.Open(t)
	sender := &countingSender{}
	s := NewService(NewRepository(db), sender)
	for _, in := range []DispatchInput{{}, {Source: "test", RequestID: "nul", Title: "test", Content: "a\x00b"}, {Source: "test", RequestID: "long", Title: "test", Content: strings.Repeat("中", 700)}} {
		if _, err := s.Dispatch(context.Background(), in); !errors.Is(err, ErrInvalid) {
			t.Fatalf("want invalid, got %v", err)
		}
	}
	for i := range 10 {
		_, err := s.Dispatch(context.Background(), DispatchInput{Source: "test", RequestID: string(rune('a' + i)), Title: "test", Content: "body"})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Dispatch(context.Background(), DispatchInput{Source: "test", RequestID: "extra", Title: "test", Content: "body"}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want rate limit, got %v", err)
	}
	if sender.calls != 10 {
		t.Fatalf("sends=%d", sender.calls)
	}
}

func TestDispatchReservationWaitHonorsDeadline(t *testing.T) {
	db := testdatabase.Open(t)
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(NewRepository(db), &countingSender{})
	first := make(chan error, 1)
	second := make(chan error, 1)
	go func() {
		_, err := s.Dispatch(context.Background(), DispatchInput{Source: "test", RequestID: "first", Title: "test", Content: "body"})
		first <- err
	}()
	defer func() { conn.Close(); <-first }()
	deadline := time.After(time.Second)
	for db.Stats().WaitCount == 0 {
		select {
		case <-deadline:
			t.Fatal("first request did not reach database")
		case <-time.After(time.Millisecond):
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	go func() {
		_, err := s.Dispatch(ctx, DispatchInput{Source: "test", RequestID: "second", Title: "test", Content: "body"})
		second <- err
	}()
	select {
	case err := <-second:
		if !errors.Is(err, ErrStorage) {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("reservation wait ignored caller deadline")
	}
}

// 发送前写库失败不能发送；发送后写回失败不能让调用方重复发送。
func TestDispatchDatabaseFailures(t *testing.T) {
	t.Run("reserve failure", func(t *testing.T) {
		db := testdatabase.Open(t)
		sender := &countingSender{}
		s := NewService(NewRepository(db), sender)
		if _, err := db.Exec(`CREATE TRIGGER reject_reserve BEFORE INSERT ON notification_log BEGIN SELECT RAISE(FAIL, 'unavailable'); END`); err != nil {
			t.Fatal(err)
		}
		_, err := s.Dispatch(context.Background(), DispatchInput{Source: "test", RequestID: "one", Title: "test", Content: "body"})
		if !errors.Is(err, ErrStorage) || sender.calls != 0 {
			t.Fatalf("err=%v sends=%d", err, sender.calls)
		}
	})
	t.Run("finish failure", func(t *testing.T) {
		db := testdatabase.Open(t)
		sender := &countingSender{}
		s := NewService(NewRepository(db), sender)
		if _, err := db.Exec(`CREATE TRIGGER reject_finish BEFORE UPDATE ON notification_log BEGIN SELECT RAISE(FAIL, 'unavailable'); END`); err != nil {
			t.Fatal(err)
		}
		in := DispatchInput{Source: "test", RequestID: "one", Title: "test", Content: "body"}
		if _, err := s.Dispatch(context.Background(), in); !errors.Is(err, ErrStorage) {
			t.Fatalf("err=%v", err)
		}
		result, err := NewService(NewRepository(db), sender).Dispatch(context.Background(), in)
		if err != nil || result.Status != "unknown" || !result.Duplicate || sender.calls != 1 {
			t.Fatalf("result=%+v err=%v sends=%d", result, err, sender.calls)
		}
	})
}

func TestDispatchIndependentServicesShareIdempotency(t *testing.T) {
	db := testdatabase.Open(t)
	sender := &countingSender{}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := NewService(NewRepository(db), sender).Dispatch(context.Background(), DispatchInput{Source: "test", RequestID: "concurrent", Title: "test", Content: "body"})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if sender.calls != 1 {
		t.Fatalf("sends=%d", sender.calls)
	}
}
