package comment

import (
	"context"
	"testing"
	"time"

	"blog-api/internal/post"
	"blog-api/internal/user"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCommentDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&user.User{}, &post.Post{}, &Comment{}))
	return db
}

func seedAuthor(t *testing.T, db *gorm.DB, email string) *user.User {
	t.Helper()
	id, err := uuid.NewV7()
	require.NoError(t, err)
	u := &user.User{PublicID: id.String(), Email: email, PasswordHash: "x"}
	require.NoError(t, db.Create(u).Error)
	return u
}

func seedParentPost(t *testing.T, db *gorm.DB, authorID uint64) *post.Post {
	t.Helper()
	id, err := uuid.NewV7()
	require.NoError(t, err)
	p := &post.Post{
		PublicID: id.String(),
		AuthorID: authorID,
		Title:    "Hello",
		Body:     "World",
		Status:   post.StatusDraft,
	}
	require.NoError(t, db.Create(p).Error)
	return p
}

func seedComment(t *testing.T, db *gorm.DB, postID, authorID uint64, body string) *Comment {
	t.Helper()
	id, err := uuid.NewV7()
	require.NoError(t, err)
	c := &Comment{PublicID: id.String(), PostID: postID, AuthorID: authorID, Body: body}
	require.NoError(t, db.Create(c).Error)
	return c
}

func TestRepository_CreateAndListScopedToPost(t *testing.T) {
	db := setupCommentDB(t)
	repo := NewRepository(db)
	author := seedAuthor(t, db, "a@example.com")
	p := seedParentPost(t, db, author.ID)
	id, err := uuid.NewV7()
	require.NoError(t, err)
	c := &Comment{PublicID: id.String(), PostID: p.ID, AuthorID: author.ID, Body: "hello"}
	require.NoError(t, repo.Create(context.Background(), c))
	assert.NotZero(t, c.ID)
}

func TestRepository_GetByPublicID_UnknownVsMalformed(t *testing.T) {
	db := setupCommentDB(t)
	repo := NewRepository(db)
	author := seedAuthor(t, db, "a@example.com")
	p := seedParentPost(t, db, author.ID)
	c := seedComment(t, db, p.ID, author.ID, "hello")
	ctx := context.Background()

	got, err := repo.GetByPublicID(ctx, c.PublicID)
	require.NoError(t, err)
	assert.Equal(t, c.ID, got.ID)

	_, err = repo.GetByPublicID(ctx, "0190f0e2-8c3a-7b2d-9e4f-ffffffffffff")
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = repo.GetByPublicID(ctx, "not-a-uuid")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRepository_GetByPublicID_ExcludesSoftDeleted(t *testing.T) {
	db := setupCommentDB(t)
	repo := NewRepository(db)
	author := seedAuthor(t, db, "a@example.com")
	p := seedParentPost(t, db, author.ID)
	c := seedComment(t, db, p.ID, author.ID, "hello")
	require.NoError(t, repo.Delete(context.Background(), c))

	_, err := repo.GetByPublicID(context.Background(), c.PublicID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRepository_ListByPostID_PaginationWhitelist(t *testing.T) {
	db := setupCommentDB(t)
	repo := NewRepository(db)
	author := seedAuthor(t, db, "a@example.com")
	p := seedParentPost(t, db, author.ID)
	other := seedParentPost(t, db, author.ID)

	first := seedComment(t, db, p.ID, author.ID, "alpha")
	require.NoError(t, db.Model(first).Update("created_at", time.Now().Add(-2*time.Hour)).Error)
	second := seedComment(t, db, p.ID, author.ID, "beta")
	require.NoError(t, db.Model(second).Update("created_at", time.Now().Add(-1*time.Hour)).Error)
	seedComment(t, db, other.ID, author.ID, "other-post")

	ctx := context.Background()

	asc, total, err := repo.ListByPostID(ctx, ListQuery{PostID: p.ID, Page: 1, PerPage: 15, Sort: "created_at", Order: "asc"})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, asc, 2)
	assert.Equal(t, []string{"alpha", "beta"}, bodies(asc))

	// Unknown sort falls back to created_at; never used as a column.
	fallback, _, err := repo.ListByPostID(ctx, ListQuery{
		PostID: p.ID, Page: 1, PerPage: 15, Sort: "body;drop table comments", Order: "asc",
	})
	require.NoError(t, err)
	require.Len(t, fallback, 2)
	assert.Equal(t, []string{"alpha", "beta"}, bodies(fallback))

	search, total, err := repo.ListByPostID(ctx, ListQuery{PostID: p.ID, Page: 1, PerPage: 15, Search: "alp"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, search, 1)
	assert.Equal(t, "alpha", search[0].Body)
}

func TestRepository_ListByPostID_PageDefaults(t *testing.T) {
	db := setupCommentDB(t)
	repo := NewRepository(db)
	author := seedAuthor(t, db, "a@example.com")
	p := seedParentPost(t, db, author.ID)
	for i := 0; i < 16; i++ {
		seedComment(t, db, p.ID, author.ID, "body")
	}

	page, total, err := repo.ListByPostID(context.Background(), ListQuery{PostID: p.ID, Page: 0, PerPage: 0})
	require.NoError(t, err)
	assert.Equal(t, int64(16), total)
	assert.Len(t, page, defaultPerPage)
}

func bodies(comments []Comment) []string {
	out := make([]string, len(comments))
	for i := range comments {
		out[i] = comments[i].Body
	}
	return out
}
