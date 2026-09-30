package blog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput = errors.New("状态内容无效")
	ErrConflict     = errors.New("该发布请求已经存在，内容不同")
)

const ImageBaseURL = "https://pic.aowugong.top/"

type Image struct {
	Key string `json:"key"`
	URL string `json:"url"`
}
type Status struct {
	ID          string     `json:"id"`
	AuthorID    int64      `json:"-"`
	Body        string     `json:"body"`
	Images      []Image    `json:"images"`
	Published   bool       `json:"published"`
	PublishedAt *time.Time `json:"published_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
type StatusInput struct {
	ID          string     `json:"id"`
	Body        string     `json:"body"`
	Images      []Image    `json:"images"`
	Published   bool       `json:"published"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	Create      bool       `json:"-"`
}
type StatusService struct{ repository *Repository }

func NewStatusService(repository *Repository) *StatusService {
	return &StatusService{repository: repository}
}
func (s *StatusService) List(ctx context.Context, publishedOnly bool, limit, offset int) ([]Status, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, ErrInvalidInput
	}
	return s.repository.List(ctx, publishedOnly, limit, offset)
}
func validateImages(images []Image, legacy bool) error {
	if len(images) > 9 {
		return fmt.Errorf("%w: 最多九张图片", ErrInvalidInput)
	}
	for _, image := range images {
		allowed := strings.HasPrefix(image.Key, "blog/status/") || (legacy && strings.HasPrefix(image.Key, "pic/"))
		if !allowed || path.Clean(image.Key) != image.Key || strings.ContainsAny(image.Key, "\\?#%") || image.URL != ImageBaseURL+image.Key {
			return fmt.Errorf("%w: 图片必须来自已上传的博客图片", ErrInvalidInput)
		}
	}
	return nil
}
func (s *StatusService) Save(ctx context.Context, actorID int64, input StatusInput) (Status, error) {
	return s.save(ctx, actorID, input, false)
}
func (s *StatusService) save(ctx context.Context, actorID int64, input StatusInput, legacy bool) (Status, error) {
	id, err := uuid.Parse(input.ID)
	if err != nil || id.String() != input.ID || actorID < 1 {
		return Status{}, ErrInvalidInput
	}
	input.Body = strings.TrimSpace(input.Body)
	if (input.Body == "" && len(input.Images) == 0) || utf8.RuneCountInString(input.Body) > 30000 {
		return Status{}, fmt.Errorf("%w: 正文或图片至少填写一项，正文最多三万字", ErrInvalidInput)
	}
	if err := validateImages(input.Images, legacy); err != nil {
		return Status{}, err
	}
	if input.Images == nil {
		input.Images = []Image{}
	}
	now := time.Now().UTC()
	record := Status{ID: input.ID, AuthorID: actorID, Body: input.Body, Images: input.Images, Published: input.Published, PublishedAt: input.PublishedAt, CreatedAt: now, UpdatedAt: now}
	if !input.Create {
		old, err := s.repository.Get(ctx, input.ID)
		if err != nil {
			return Status{}, err
		}
		record.AuthorID = old.AuthorID
		record.CreatedAt = old.CreatedAt
		if record.PublishedAt == nil {
			record.PublishedAt = old.PublishedAt
		}
	}
	if record.Published && record.PublishedAt == nil {
		record.PublishedAt = &now
	}
	if record.PublishedAt != nil && record.PublishedAt.After(now.Add(time.Minute)) {
		return Status{}, fmt.Errorf("%w: 不支持未来定时发布", ErrInvalidInput)
	}
	if input.Create {
		inserted, err := s.repository.Insert(ctx, record)
		if err != nil {
			return Status{}, err
		}
		if !inserted {
			old, err := s.repository.Get(ctx, input.ID)
			if err != nil {
				return Status{}, err
			}
			oldImages, _ := json.Marshal(old.Images)
			newImages, _ := json.Marshal(record.Images)
			if old.AuthorID != actorID || old.Body != record.Body || old.Published != record.Published || string(oldImages) != string(newImages) {
				return Status{}, ErrConflict
			}
			return old, nil
		}
	} else if err := s.repository.Update(ctx, record); err != nil {
		return Status{}, err
	}
	return s.repository.Get(ctx, input.ID)
}
func (s *StatusService) Delete(ctx context.Context, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return ErrInvalidInput
	}
	return s.repository.Delete(ctx, id)
}
