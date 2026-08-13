package comment

import (
	"context"
	"testing"

	"blog-api/internal/post"
	"blog-api/internal/user"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCreate_InvalidPayload(t *testing.T) {
	svc := NewService(&stubComments{}, &stubPosts{}, &stubUsers{}, nil, validator.New())
	_, err := svc.Create(context.Background(), "author-a", CreateCommentRequest{})
	require.Error(t, err)
}

func TestCreate_UnknownVsMalformedPostPublicID(t *testing.T) {
	parent := &post.Post{ID: 7, PublicID: "0190f0e2-aaaa-7b2d-9e4f-1a2b3c4d5e6f", AuthorID: 1}
	author := &user.User{ID: 2, PublicID: "author-a"}
	svc := NewService(
		&stubComments{},
		&stubPosts{p: parent},
		&stubUsers{byPublicID: map[string]*user.User{author.PublicID: author}},
		nil,
		validator.New(),
	)

	_, err := svc.Create(context.Background(), author.PublicID, CreateCommentRequest{
		PostPublicID: "0190f0e2-ffff-7b2d-9e4f-ffffffffffff",
		Body:         "hi",
	})
	assert.ErrorIs(t, err, post.ErrNotFound)

	_, err = svc.Create(context.Background(), author.PublicID, CreateCommentRequest{
		PostPublicID: "not-a-uuid",
		Body:         "hi",
	})
	assert.ErrorIs(t, err, post.ErrNotFound)
}

func TestCreate_StoresInternalFKs(t *testing.T) {
	parent := &post.Post{ID: 7, PublicID: "post-1", AuthorID: 1}
	author := &user.User{ID: 2, PublicID: "author-a"}
	comments := &stubComments{}
	svc := NewService(
		comments,
		&stubPosts{p: parent},
		&stubUsers{byPublicID: map[string]*user.User{author.PublicID: author}},
		nil,
		validator.New(),
	)

	got, err := svc.Create(context.Background(), author.PublicID, CreateCommentRequest{
		PostPublicID: parent.PublicID,
		Body:         "hello",
	})
	require.NoError(t, err)
	require.NotNil(t, comments.created)
	assert.Equal(t, parent.ID, comments.created.PostID)
	assert.Equal(t, author.ID, comments.created.AuthorID)
	assert.Equal(t, parent.PublicID, got.Post.PublicID)
	assert.Equal(t, author.PublicID, got.Author.PublicID)
	assert.Len(t, got.PublicID, 36)
}

func TestList_UnknownVsMalformedPostPublicID(t *testing.T) {
	svc := NewService(&stubComments{}, &stubPosts{}, &stubUsers{}, nil, validator.New())

	_, _, _, err := svc.List(context.Background(), "0190f0e2-ffff-7b2d-9e4f-ffffffffffff", ListQuery{Page: 1, PerPage: 15})
	assert.ErrorIs(t, err, post.ErrNotFound)

	_, _, _, err = svc.List(context.Background(), "not-a-uuid", ListQuery{Page: 1, PerPage: 15})
	assert.ErrorIs(t, err, post.ErrNotFound)
}

func TestDelete_ForbiddenForBystander(t *testing.T) {
	commentAuthor := &user.User{ID: 2, PublicID: "commenter"}
	postAuthor := &user.User{ID: 1, PublicID: "post-author"}
	bystander := &user.User{ID: 3, PublicID: "bystander"}
	parent := &post.Post{ID: 7, PublicID: "post-1", AuthorID: postAuthor.ID}
	existing := &Comment{ID: 20, PublicID: "c-1", PostID: parent.ID, AuthorID: commentAuthor.ID, Body: "hi"}

	svc := NewService(
		&stubComments{c: existing},
		&stubPosts{p: parent},
		&stubUsers{byPublicID: map[string]*user.User{
			bystander.PublicID: bystander,
		}},
		nil,
		validator.New(),
	)
	err := svc.Delete(context.Background(), bystander.PublicID, existing.PublicID)
	assert.ErrorIs(t, err, ErrForbidden)
}

func TestDelete_AllowedForCommentAuthorAndPostAuthor(t *testing.T) {
	commentAuthor := &user.User{ID: 2, PublicID: "commenter"}
	postAuthor := &user.User{ID: 1, PublicID: "post-author"}
	parent := &post.Post{ID: 7, PublicID: "post-1", AuthorID: postAuthor.ID}
	existing := &Comment{ID: 20, PublicID: "c-1", PostID: parent.ID, AuthorID: commentAuthor.ID, Body: "hi"}

	users := &stubUsers{byPublicID: map[string]*user.User{
		commentAuthor.PublicID: commentAuthor,
		postAuthor.PublicID:    postAuthor,
	}}
	posts := &stubPosts{p: parent}

	comments := &stubComments{c: existing}
	svc := NewService(comments, posts, users, nil, validator.New())
	require.NoError(t, svc.Delete(context.Background(), commentAuthor.PublicID, existing.PublicID))
	assert.True(t, comments.deleted)

	comments = &stubComments{c: existing}
	svc = NewService(comments, posts, users, nil, validator.New())
	require.NoError(t, svc.Delete(context.Background(), postAuthor.PublicID, existing.PublicID))
	assert.True(t, comments.deleted)
}

func TestDelete_UnknownVsMalformedCommentPublicID(t *testing.T) {
	caller := &user.User{ID: 1, PublicID: "author-a"}
	svc := NewService(
		&stubComments{},
		&stubPosts{},
		&stubUsers{byPublicID: map[string]*user.User{caller.PublicID: caller}},
		nil,
		validator.New(),
	)

	err := svc.Delete(context.Background(), caller.PublicID, "0190f0e2-ffff-7b2d-9e4f-ffffffffffff")
	assert.ErrorIs(t, err, ErrNotFound)

	err = svc.Delete(context.Background(), caller.PublicID, "not-a-uuid")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestList_PreservesQueryInLinks(t *testing.T) {
	parent := &post.Post{ID: 7, PublicID: "post-1", AuthorID: 1}
	comments := []Comment{{PublicID: "c-1", PostID: 7, AuthorID: 2, Body: "hi"}}
	svc := NewService(
		&stubComments{list: comments, total: 40},
		&stubPosts{p: parent},
		&stubUsers{byID: map[uint64]*user.User{2: {ID: 2, PublicID: "author-a"}}},
		nil,
		validator.New(),
	)

	_, meta, links, err := svc.List(context.Background(), parent.PublicID, ListQuery{
		Page: 2, PerPage: 15, Search: "hi", Sort: "created_at", Order: "desc",
		Path: "/api/posts/post-1/comments",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, meta.CurrentPage)
	assert.Equal(t, "hi", meta.Search)
	assert.Contains(t, links.First, "search=hi")
	require.NotNil(t, links.Prev)
	require.NotNil(t, links.Next)
}

type stubComments struct {
	c       *Comment
	list    []Comment
	total   int64
	created *Comment
	deleted bool
}

func (s *stubComments) WithTx(*gorm.DB) Repository { return s }
func (s *stubComments) Create(_ context.Context, c *Comment) error {
	s.created = c
	return nil
}
func (s *stubComments) GetByPublicID(_ context.Context, publicID string) (*Comment, error) {
	if s.c == nil || s.c.PublicID != publicID {
		return nil, ErrNotFound
	}
	cp := *s.c
	return &cp, nil
}
func (s *stubComments) ListByPostID(context.Context, ListQuery) ([]Comment, int64, error) {
	return s.list, s.total, nil
}
func (s *stubComments) Delete(context.Context, *Comment) error {
	s.deleted = true
	return nil
}

type stubPosts struct {
	p *post.Post
}

func (s *stubPosts) WithTx(*gorm.DB) post.Repository          { return s }
func (s *stubPosts) Create(context.Context, *post.Post) error { return nil }
func (s *stubPosts) GetByID(_ context.Context, id uint64) (*post.Post, error) {
	if s.p == nil || s.p.ID != id {
		return nil, post.ErrNotFound
	}
	cp := *s.p
	return &cp, nil
}
func (s *stubPosts) GetByPublicID(_ context.Context, publicID string) (*post.Post, error) {
	if s.p == nil || s.p.PublicID != publicID {
		return nil, post.ErrNotFound
	}
	cp := *s.p
	return &cp, nil
}
func (s *stubPosts) List(context.Context, post.ListQuery) ([]post.Post, int64, error) {
	return nil, 0, nil
}
func (s *stubPosts) Update(context.Context, *post.Post) error                 { return nil }
func (s *stubPosts) Delete(context.Context, *post.Post) error                 { return nil }
func (s *stubPosts) CreatePublishLog(context.Context, *post.PublishLog) error { return nil }

type stubUsers struct {
	byPublicID map[string]*user.User
	byID       map[uint64]*user.User
}

func (s *stubUsers) WithTx(*gorm.DB) user.Repository          { return s }
func (s *stubUsers) Create(context.Context, *user.User) error { return nil }
func (s *stubUsers) GetByID(_ context.Context, id uint64) (*user.User, error) {
	u, ok := s.byID[id]
	if !ok {
		return nil, user.ErrNotFound
	}
	return u, nil
}
func (s *stubUsers) GetByIDs(_ context.Context, ids []uint64) ([]user.User, error) {
	out := make([]user.User, 0, len(ids))
	for _, id := range ids {
		u, ok := s.byID[id]
		if !ok {
			continue
		}
		out = append(out, *u)
	}
	return out, nil
}
func (s *stubUsers) GetByPublicID(_ context.Context, publicID string) (*user.User, error) {
	u, ok := s.byPublicID[publicID]
	if !ok {
		return nil, user.ErrNotFound
	}
	return u, nil
}
func (s *stubUsers) GetByEmail(context.Context, string) (*user.User, error) {
	return nil, user.ErrNotFound
}
