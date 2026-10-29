package articleanalysis

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/howiedata/aowugong-go/internal/client"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

func TestMissingArticleClientReportsStatusWriteFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT id, source_code, source_name, source_type, feed_url, is_active").WillReturnRows(sqlmock.NewRows([]string{"id", "source_code", "source_name", "source_type", "feed_url", "is_active", "description", "last_fetch_at", "last_fetch_status", "last_fetch_message"}).AddRow(1, "test", "测试来源", "feed", "", 1, "", "", "", ""))
	failure := errors.New("database unavailable")
	mock.ExpectExec("UPDATE investment_article_source").WillReturnError(failure)
	_, err = NewService(NewRepository(db), ServiceOptions{}).Sync(context.Background(), 30, false, 0)
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "文章客户端未配置") {
		t.Fatalf("missing original or persistence error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWeReadFetchPreservesUpstreamAndPersistenceErrors(t *testing.T) {
	for _, failWrites := range []bool{false, true} {
		t.Run(map[bool]string{false: "upstream only", true: "upstream and database"}[failWrites], func(t *testing.T) {
			ctx := context.Background()
			db := testdatabase.Open(t)
			repo := NewRepository(db)
			if err := repo.upsertWeReadAccounts(ctx, []WeReadAccount{{AccountID: "MP_WXS_100", Title: "测试公众号"}}); err != nil {
				t.Fatal(err)
			}
			enabled := true
			if err := repo.updateWeReadAccount(ctx, "MP_WXS_100", nil, &enabled); err != nil {
				t.Fatal(err)
			}
			transport := articleAnalysisRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/mp/chapters" {
					if r.Header.Get("accessToken") == "old-access" {
						return articleAnalysisHTTPResponse(r, 401, `{"errCode":-2012}`), nil
					}
					return articleAnalysisHTTPResponse(r, 200, `{"errCode":-2041,"errMsg":"verify required"}`), nil
				}
				return articleAnalysisHTTPResponse(r, 200, `{"vid":"123456","accessToken":"new-access","refreshToken":"new-refresh","errCode":0}`), nil
			})
			source := NewWeReadSource(repo, client.NewWeReadArticleClient(&http.Client{Transport: transport}), "test-secret")
			if err := source.saveCredentials(ctx, client.WeReadArticleCredentials{VID: "123456", DeviceID: "device-test", AccessToken: "old-access", RefreshToken: "refresh-token"}, true); err != nil {
				t.Fatal(err)
			}
			if failWrites {
				for _, sql := range []string{`CREATE TRIGGER reject_credential_update BEFORE UPDATE ON weread_article_credential BEGIN SELECT RAISE(FAIL, 'test write failure'); END`, `CREATE TRIGGER reject_credential_insert BEFORE INSERT ON weread_article_credential BEGIN SELECT RAISE(FAIL, 'test write failure'); END`} {
					if _, err := db.Exec(sql); err != nil {
						t.Fatal(err)
					}
				}
			}
			_, err := source.Fetch(ctx, 1, "", 1)
			if !errors.Is(err, client.ErrWeReadArticleVerification) {
				t.Fatalf("original verification error lost: %v", err)
			}
			if failWrites {
				for _, fragment := range []string{"保存刷新后的微信读书凭据", "保存微信读书凭据检查状态", "test write failure"} {
					if !strings.Contains(err.Error(), fragment) {
						t.Fatalf("missing %s: %v", fragment, err)
					}
				}
			} else {
				credentials, loadErr := source.loadCredentials(ctx)
				if loadErr != nil || credentials.AccessToken != "new-access" {
					t.Fatalf("refreshed credentials not saved: %v", loadErr)
				}
			}
		})
	}
}
