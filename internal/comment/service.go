package comment

import (
	"blog-api/internal/post"
	"blog-api/internal/response"
	"blog-api/internal/user"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrForbidden = errors.New("forbidden")

type Service interface {
	Create(ctx context.Context, callerPublicID string, req CreateCommentRequest) (CommentResponse, error)
	List(ctx context.Context, postPublicID string, q ListQuery) ([]CommentResponse, response.Meta, response.Links, error)
	Delete(ctx context.Context, callerPublicID, commentPublicID string) error
}
type service struct {
	repo     Repository
	posts    post.Repository
	users    user.Repository
	db       *gorm.DB
	validate *validator.Validate
}

func NewService(repo Repository, posts post.Repository, users user.Repository, db *gorm.DB, validate *validator.Validate) Service {
	return &service{repo: repo, posts: posts, users: users, db: db, validate: validate}
}
func (s *service) Create(ctx context.Context, callerPublicID string, req CreateCommentRequest) (CommentResponse, error) {
	if err := s.validate.Struct(req); err != nil {
		return CommentResponse{}, err
	}
	// §8 resolve flow for the parent post (client sent post_public_id, never post_id).
	// 1. Lookup by public_id
	p, err := s.posts.GetByPublicID(ctx, req.PostPublicID)
	if err != nil {
		return CommentResponse{}, err // post.ErrNotFound if missing / soft-deleted
	}
	// 2. Validate: GetByPublicID already excludes soft-deleted posts. Existence is enough here.
	// Same flow for the caller (JWT sub is a user public_id).
	author, err := s.users.GetByPublicID(ctx, callerPublicID)
	if err != nil {
		return CommentResponse{}, err
	}
	publicID, err := uuid.NewV7()
	if err != nil {
		return CommentResponse{}, fmt.Errorf("generate public ID: %w", err)
	}
	// 3. Use internal ids for FKs — never trust numeric ids from the client.
	c := &Comment{
		PublicID: publicID.String(),
		PostID:   p.ID,
		AuthorID: author.ID,
		Body:     req.Body,
	}
	if err := s.repo.Create(ctx, c); err != nil {
		return CommentResponse{}, err
	}
	return toCommentResponse(c, p.PublicID, author.PublicID), nil
}
func (s *service) List(ctx context.Context, postPublicID string, q ListQuery) ([]CommentResponse, response.Meta, response.Links, error) {
	// Nested list: same resolve → validate → query by internal post_id.
	p, err := s.posts.GetByPublicID(ctx, postPublicID)
	if err != nil {
		return nil, response.Meta{}, response.Links{}, err
	}
	q.PostID = p.ID
	comments, total, err := s.repo.ListByPostID(ctx, q)
	if err != nil {
		return nil, response.Meta{}, response.Links{}, err
	}
	authors, err := s.users.GetByIDs(ctx, uniqueAuthorIDs(comments))
	if err != nil {
		return nil, response.Meta{}, response.Links{}, err
	}
	byID := make(map[uint64]string, len(authors))
	for i := range authors {
		byID[authors[i].ID] = authors[i].PublicID
	}
	out := make([]CommentResponse, 0, len(comments))
	for i := range comments {
		authorPublicID, ok := byID[comments[i].AuthorID]
		if !ok {
			return nil, response.Meta{}, response.Links{}, user.ErrNotFound
		}
		out = append(out, toCommentResponse(&comments[i], p.PublicID, authorPublicID))
	}
	page, perPage := normalizePage(q.Page, q.PerPage)
	meta, links := buildPage(q, page, perPage, len(out), total)
	return out, meta, links, nil
}
func (s *service) Delete(ctx context.Context, callerPublicID, commentPublicID string) error {
	caller, err := s.users.GetByPublicID(ctx, callerPublicID)
	if err != nil {
		return err
	}
	c, err := s.repo.GetByPublicID(ctx, commentPublicID)
	if err != nil {
		return err
	}
	// Parent is stored as post_id (internal). Resolve that row explicitly so we can
	// compare the post author's internal id — do not skip this and compare public_ids ad hoc.
	parent, err := s.posts.GetByID(ctx, c.PostID)
	if err != nil {
		return err
	}
	isCommentAuthor := c.AuthorID == caller.ID
	isPostAuthor := parent.AuthorID == caller.ID
	// Both roles may delete:
	// - comment author: you can take back your own comment
	// - post author: you moderate discussion on a post you own
	// Anyone else is forbidden even if they guess the comment public_id.
	if !isCommentAuthor && !isPostAuthor {
		return ErrForbidden
	}
	return s.repo.Delete(ctx, c)
}
func uniqueAuthorIDs(comments []Comment) []uint64 {
	seen := make(map[uint64]struct{}, len(comments))
	ids := make([]uint64, 0, len(comments))
	for i := range comments {
		id := comments[i].AuthorID
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}
func toCommentResponse(c *Comment, postPublicID, authorPublicID string) CommentResponse {
	return CommentResponse{
		PublicID:  c.PublicID,
		Post:      PostRef{PublicID: postPublicID},
		Author:    AuthorRef{PublicID: authorPublicID},
		Body:      c.Body,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}
func buildPage(q ListQuery, page, perPage, n int, total int64) (response.Meta, response.Links) {
	path := q.Path
	if path == "" {
		path = "/api/comments"
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
	return b.String()
}
