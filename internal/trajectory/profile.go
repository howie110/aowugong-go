package trajectory

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/google/uuid"
)

// ProfileCredential derives a reproducible companion credential without
// replacing the original device key or retaining its plaintext.
func (s *Service) ProfileCredential(ctx context.Context, owner int64, id string) (Credential, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Credential{}, ErrNotFound
	}
	if s.profileSecret == "" {
		return Credential{}, errors.New("trajectory profile signing is not configured")
	}
	var c Credential
	var hash string
	err := s.db.QueryRowContext(ctx, "SELECT "+deviceColumns+",token_hash FROM trajectory_devices WHERE id=? AND owner_id=? AND revoked_at IS NULL", id, owner).Scan(&c.Device.ID, &c.Device.Name, &c.Device.CreatedAt, &c.Device.RevokedAt, &c.Device.LastReceivedAt, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return Credential{}, ErrNotFound
	}
	if err != nil {
		return Credential{}, err
	}
	c.Token = s.profileToken(id, hash)
	return c, nil
}
func (s *Service) profileToken(id, hash string) string {
	mac := hmac.New(sha256.New, []byte(s.profileSecret))
	mac.Write([]byte("trajectory-profile-v1\x00" + id + "\x00" + hash))
	return strings.ReplaceAll(id, "-", "") + hex.EncodeToString(mac.Sum(nil)[:16])
}
func (s *Service) validUploadToken(token, id, hash string) bool {
	if hmac.Equal([]byte(hashToken(token)), []byte(hash)) {
		return true
	}
	return s.profileSecret != "" && hmac.Equal([]byte(token), []byte(s.profileToken(id, hash)))
}
