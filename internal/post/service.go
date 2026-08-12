package post

import (
	"blog-api/internal/response"
	"blog-api/internal/user"
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrForbidden = errors.New("forbidden")

// Service is the post business-logic API.
type Service interface {
	Create(ctx context.Context, authorPublicID string, req CreatePostRequest) (PostResponse, error)
	List(ctx context.Context, q ListQuery) ([]PostResponse, response.Meta, response.Links, error)
	Update(ctx context.Context, authorPublicID, postPublicID string, req UpdatePostRequest) (PostResponse, error)
	Delete(ctx context.Context, authorPublicID, postPublicID string) error
	PublishWithSnapshot(ctx context.Context, authorPublicID, postPublicID string) (PostResponse, error)
}

type service struct {
	repo     Repository
	users    user.Repository
	db       *gorm.DB
	validate *validator.Validate
}

func NewService(repo Repository, users user.Repository, db *gorm.DB, validate *validator.Validate) Service {
	return &service{repo: repo, users: users, db: db, validate: validate}
}

func (s *service) Create(ctx context.Context, authorPublicID string, req CreatePostRequest) (PostResponse, error) {
	if err := s.validate.Struct(req); err != nil {
		return PostResponse{}, err
	}

	// 1. Lookup author by public_id
	author, err := s.users.GetByPublicID(ctx, authorPublicID)
	if err != nil {
		return PostResponse{}, err
	}

	// 2. Validate: GetByPublicID already excludes soft-deleted users.
	// 3. Use internal id for the FK.
	publicID, err := uuid.NewV7()
	if err != nil {
		return PostResponse{}, fmt.Errorf("generate public ID: %w", err)
	}
	p := &Post{
		PublicID: publicID.String(),
		AuthorID: author.ID, // internal PK only
		Title:    req.Title,
		Body:     req.Body,
		Status:   StatusDraft,
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return PostResponse{}, err
	}
	return toPostResponse(p, author.PublicID), nil
}

func (s *service) List(ctx context.Context, q ListQuery) ([]PostResponse, response.Meta, response.Links, error) {
	posts, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, response.Meta{}, response.Links{}, err
	}
	page, perPage := normalizePage(q.Page, q.PerPage)
	authorIDs := make(map[uint64]string, len(posts))
	out := make([]PostResponse, 0, len(posts))
	for i := range posts {
		authorPublicID, err := s.authorPublicID(ctx, authorIDs, posts[i].AuthorID)
		if err != nil {
			return nil, response.Meta{}, response.Links{}, err
		}
		out = append(out, toPostResponse(&posts[i], authorPublicID))
	}
	meta, links := buildPage(q, page, perPage, len(out), total)
	return out, meta, links, nil
}

func (s *service) Update(ctx context.Context, authorPublicID, postPublicID string, req UpdatePostRequest) (PostResponse, error) {
	if err := s.validate.Struct(req); err != nil {
		return PostResponse{}, err
	}
	p, author, err := s.ownedPost(ctx, s.repo, s.users, authorPublicID, postPublicID)
	if err != nil {
		return PostResponse{}, err
	}
	if req.Title != nil {
		p.Title = *req.Title
	}
	if req.Body != nil {
		p.Body = *req.Body
	}
	if err := s.repo.Update(ctx, p); err != nil {
		return PostResponse{}, err
	}
	return toPostResponse(p, author.PublicID), nil
}

func (s *service) Delete(ctx context.Context, authorPublicID, postPublicID string) error {
	p, _, err := s.ownedPost(ctx, s.repo, s.users, authorPublicID, postPublicID)
	if err != nil {
		return err
	}
	return s.repo.Delete(ctx, p)
}

func (s *service) PublishWithSnapshot(ctx context.Context, authorPublicID, postPublicID string) (PostResponse, error) {
	var out PostResponse
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := s.repo.WithTx(tx)
		users := s.users.WithTx(tx)
		p, author, err := s.ownedPost(ctx, repo, users, authorPublicID, postPublicID)
		if err != nil {
			return err
		}
		p.Status = StatusPublished
		if err := repo.Update(ctx, p); err != nil {
			return err
		}
		if err := repo.CreatePublishLog(ctx, &PublishLog{
			PostID:      p.ID, // internal FK, never public_id
			PublishedAt: time.Now(),
		}); err != nil {
			return err // returning an error rolls back the status change
		}
		out = toPostResponse(p, author.PublicID)
		return nil
	})
	if err != nil {
		return PostResponse{}, err
	}
	return out, nil
}

// ownedPost: resolve post + author by public_id, then compare internal ids.
func (s *service) ownedPost(ctx context.Context, repo Repository, users user.Repository, authorPublicID, postPublicID string) (*Post, *user.User, error) {
	author, err := users.GetByPublicID(ctx, authorPublicID)
	if err != nil {
		return nil, nil, err
	}
	p, err := repo.GetByPublicID(ctx, postPublicID)
	if err != nil {
		return nil, nil, err
	}
	if p.AuthorID != author.ID {
		return nil, nil, ErrForbidden
	}
	return p, author, nil
}
func (s *service) authorPublicID(ctx context.Context, cache map[uint64]string, authorID uint64) (string, error) {
	if id, ok := cache[authorID]; ok {
		return id, nil
	}
	u, err := s.users.GetByID(ctx, authorID)
	if err != nil {
		return "", err
	}
	cache[authorID] = u.PublicID
	return u.PublicID, nil
}
func toPostResponse(p *Post, authorPublicID string) PostResponse {
	return PostResponse{
		PublicID:  p.PublicID,
		Author:    AuthorRef{PublicID: authorPublicID},
		Title:     p.Title,
		Body:      p.Body,
		Status:    p.Status,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}
func buildPage(q ListQuery, page, perPage, n int, total int64) (response.Meta, response.Links) {
	path := q.Path
	if path == "" {
		path = "/api/posts"
	}
	last := lastPage(total, perPage)
	var from, to *int
	if n > 0 {
		f := (page-1)*perPage + 1
		t := f + n - 1
		from, to = &f, &t
	}
	meta := response.Meta{
		CurrentPage: page,
		From:        from,
		LastPage:    last,
		Path:        path,
		PerPage:     perPage,
		To:          to,
		Total:       total,
		Search:      q.Search,
		Sort:        q.Sort,
		Order:       q.Order,
		Filters:     echoFilters(q.Filters),
	}
	qs := func(p int) string { return path + "?" + listQueryString(q, p, perPage) }
	links := response.Links{
		First: qs(1),
		Last:  qs(last),
	}
	if page > 1 {
		s := qs(page - 1)
		links.Prev = &s
	}
	if page < last {
		s := qs(page + 1)
		links.Next = &s
	}
	return meta, links
}
func lastPage(total int64, perPage int) int {
	if total == 0 || perPage < 1 {
		return 1
	}
	return int((total + int64(perPage) - 1) / int64(perPage))
}
func echoFilters(in map[string]string) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if v == "" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// listQueryString keeps filter[status] unescaped (matches the architecture example).
// Only `page` changes across first/last/prev/next.
func listQueryString(q ListQuery, page, perPage int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "page=%d&per_page=%d", page, perPage)
	if q.Search != "" {
		b.WriteString("&search=")
		b.WriteString(url.QueryEscape(q.Search))
	}
	if q.Sort != "" {
		b.WriteString("&sort=")
		b.WriteString(url.QueryEscape(q.Sort))
	}
	if q.Order != "" {
		b.WriteString("&order=")
		b.WriteString(url.QueryEscape(q.Order))
	}
	keys := make([]string, 0, len(q.Filters))
	for k, v := range q.Filters {
		if v != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString("&filter[")
		b.WriteString(k)
		b.WriteString("]=")
		b.WriteString(url.QueryEscape(q.Filters[k]))
	}
	return b.String()
}
