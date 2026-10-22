package trajectory

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"strings"
	"testing"
	"time"
)

func TestProfileCredentialPreservesExistingKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewService(db, "server-secret")
	id := uuid.NewString()
	old := strings.Repeat("a", 64)
	hash := hashToken(old)
	mock.ExpectQuery("SELECT").WithArgs(id, int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "created_at", "revoked_at", "last_received_at", "token_hash"}).AddRow(id, "phone", time.Now(), nil, nil, hash))
	c, err := s.ProfileCredential(context.Background(), 1, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Token) != 64 || c.Token == old {
		t.Fatal("invalid compatible token")
	}
	if !s.validUploadToken(old, id, hash) || !s.validUploadToken(c.Token, id, hash) {
		t.Fatal("both credentials must work")
	}
	if s.validUploadToken(c.Token, uuid.NewString(), hash) || s.validUploadToken(c.Token, id, hashToken("rotated")) || NewService(db, "other-secret").validUploadToken(c.Token, id, hash) {
		t.Fatal("signature not bound to device/key/server")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	} // no INSERT or UPDATE when copying
}
