package blog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

const statusColumns = "id, author_id, body, images, published, published_at, created_at, updated_at"

func scanStatus(row interface{ Scan(...any) error }) (Status, error) {
	var result Status
	var images string
	var publishedAt sql.NullTime
	err := row.Scan(&result.ID, &result.AuthorID, &result.Body, &images, &result.Published, &publishedAt, &result.CreatedAt, &result.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Status{}, ErrNotFound
	}
	if err != nil {
		return Status{}, err
	}
	if publishedAt.Valid {
		result.PublishedAt = &publishedAt.Time
	}
	if err := json.Unmarshal([]byte(images), &result.Images); err != nil {
		return Status{}, err
	}
	return result, nil
}
func (r *Repository) Get(ctx context.Context, id string) (Status, error) {
	return scanStatus(r.db.QueryRowContext(ctx, "SELECT "+statusColumns+" FROM blog_statuses WHERE id = ?", id))
}
func (r *Repository) List(ctx context.Context, publishedOnly bool, limit, offset int) ([]Status, error) {
	query := "SELECT " + statusColumns + " FROM blog_statuses"
	if publishedOnly {
		query += " WHERE published = TRUE"
	}
	query += " ORDER BY COALESCE(published_at, created_at) DESC, id DESC LIMIT ? OFFSET ?"
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Status{}
	for rows.Next() {
		record, err := scanStatus(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}
func (r *Repository) Insert(ctx context.Context, record Status) (bool, error) {
	images, err := json.Marshal(record.Images)
	if err != nil {
		return false, err
	}
	res, err := r.db.ExecContext(ctx, `INSERT INTO blog_statuses (`+statusColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (id) DO NOTHING`, record.ID, record.AuthorID, record.Body, string(images), record.Published, record.PublishedAt, record.CreatedAt, record.UpdatedAt)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
func (r *Repository) Update(ctx context.Context, record Status) error {
	images, err := json.Marshal(record.Images)
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `UPDATE blog_statuses SET body = ?, images = ?, published = ?, published_at = ?, updated_at = ? WHERE id = ?`, record.Body, string(images), record.Published, record.PublishedAt, record.UpdatedAt, record.ID)
	return requireAffected(res, err)
}
func (r *Repository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM blog_statuses WHERE id = ?`, id)
	return requireAffected(res, err)
}
func requireAffected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
