package trajectory

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"time"
)

type Service struct {
	db            *sql.DB
	profileSecret string
}

func NewService(db *sql.DB, secret ...string) *Service {
	s := &Service{db: db}
	if len(secret) > 0 {
		s.profileSecret = secret[0]
	}
	return s
}
func tokenPair() (string, string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", "", e
	}
	token := hex.EncodeToString(b)
	return token, hashToken(token), nil
}
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

const deviceColumns = "id,name,created_at,revoked_at,last_received_at"

type scanner interface{ Scan(...any) error }

func scanDevice(row scanner) (Device, error) {
	var d Device
	e := row.Scan(&d.ID, &d.Name, &d.CreatedAt, &d.RevokedAt, &d.LastReceivedAt)
	return d, e
}
func (s *Service) Devices(ctx context.Context, owner int64) ([]Device, error) {
	rows, e := s.db.QueryContext(ctx, "SELECT "+deviceColumns+" FROM trajectory_devices WHERE owner_id=? ORDER BY created_at DESC,id", owner)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		d, e := scanDevice(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Create 仅保存随机密钥摘要，明文仅在创建响应中返回。
func (s *Service) Create(ctx context.Context, owner int64, name string) (Credential, error) {
	if !validName(name) {
		return Credential{}, ErrInvalid
	}
	token, hash, e := tokenPair()
	if e != nil {
		return Credential{}, e
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return Credential{}, e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(8672,?)", owner); e != nil {
		return Credential{}, e
	}
	var n int
	if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM trajectory_devices WHERE owner_id=?", owner).Scan(&n); e != nil {
		return Credential{}, e
	}
	if n >= 20 {
		return Credential{}, ErrLimit
	}
	d, e := scanDevice(tx.QueryRowContext(ctx, "INSERT INTO trajectory_devices(id,owner_id,name,token_hash,created_at) VALUES(?,?,?,?,?) RETURNING "+deviceColumns, uuid.NewString(), owner, strings.TrimSpace(name), hash, time.Now().UTC()))
	if e != nil {
		return Credential{}, e
	}
	return Credential{d, token}, tx.Commit()
}
func (s *Service) Rotate(ctx context.Context, owner int64, id string) (Credential, error) {
	if _, e := uuid.Parse(id); e != nil {
		return Credential{}, ErrNotFound
	}
	token, hash, e := tokenPair()
	if e != nil {
		return Credential{}, e
	}
	d, e := scanDevice(s.db.QueryRowContext(ctx, "UPDATE trajectory_devices SET token_hash=? WHERE id=? AND owner_id=? AND revoked_at IS NULL RETURNING "+deviceColumns, hash, id, owner))
	if errors.Is(e, sql.ErrNoRows) {
		e = ErrNotFound
	}
	return Credential{d, token}, e
}
func (s *Service) Revoke(ctx context.Context, owner int64, id string) error {
	if _, e := uuid.Parse(id); e != nil {
		return ErrNotFound
	}
	result, e := s.db.ExecContext(ctx, "UPDATE trajectory_devices SET revoked_at=COALESCE(revoked_at,?) WHERE id=? AND owner_id=?", time.Now().UTC(), id, owner)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e == nil && n == 0 {
		return ErrNotFound
	}
	return e
}

// Collect 锁定设备使撤销/轮换与写入有明确先后；提交成功后才确认上传。
func (s *Service) Collect(ctx context.Context, token string, p Point) (bool, error) {
	if len(token) != 64 {
		return false, ErrUnauthorized
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return false, e
	}
	defer tx.Rollback()
	var device, storedHash string
	var owner int64
	query := `SELECT d.id,d.owner_id,d.token_hash FROM trajectory_devices d JOIN aowugong_fastapi_users u ON u.id=d.owner_id WHERE %s AND d.revoked_at IS NULL AND u.is_active=1 AND (u.is_superuser=1 OR EXISTS(SELECT 1 FROM aowugong_user_roles ur JOIN aowugong_roles r ON r.id=ur.role_id WHERE ur.user_id=u.id AND r.code='admin' AND r.is_active=1)) FOR UPDATE OF d`
	e = tx.QueryRowContext(ctx, fmt.Sprintf(query, "d.token_hash=?"), hashToken(token)).Scan(&device, &owner, &storedHash)
	if errors.Is(e, sql.ErrNoRows) && s.profileSecret != "" {
		id, err := uuid.Parse(token[:32])
		if err == nil {
			e = tx.QueryRowContext(ctx, fmt.Sprintf(query, "d.id=?"), id.String()).Scan(&device, &owner, &storedHash)
		}
	}

	if errors.Is(e, sql.ErrNoRows) {
		return false, ErrUnauthorized
	}
	if e != nil {
		return false, e
	}
	if !s.validUploadToken(token, device, storedHash) {
		return false, ErrUnauthorized
	}
	// 与同账号查询串行，确保分页快照不会混入尚未提交的补传。
	if _, e = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(8672,?)", owner); e != nil {
		return false, e
	}
	now := time.Now().UTC()
	raw, e := json.Marshal(p.Raw)
	if e != nil {
		return false, e
	}
	result, e := tx.ExecContext(ctx, `INSERT INTO trajectory_points(device_id,recorded_at,received_at,lat,lon,accuracy,altitude,speed,bearing,provider,raw_payload) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(device_id,recorded_at,lat,lon) DO NOTHING`, device, p.RecordedAt, now, p.Lat, p.Lon, p.Acc, p.Alt, p.Speed, p.Bearing, p.Provider, string(raw))
	if e != nil {
		return false, e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return false, e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE trajectory_devices SET last_received_at=? WHERE id=?", now, device); e != nil {
		return false, e
	}
	return n == 1, tx.Commit()
}
