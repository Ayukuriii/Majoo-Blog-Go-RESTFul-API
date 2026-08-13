package post

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRepository_GetByPublicID_UnknownVsMalformed(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	author := seedUser(t, db)
	p := seedDraftPost(t, db, author.ID)
	ctx := context.Background()

	got, err := repo.GetByPublicID(ctx, p.PublicID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, got.ID)

	_, err = repo.GetByPublicID(ctx, "0190f0e2-8c3a-7b2d-9e4f-ffffffffffff")
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = repo.GetByPublicID(ctx, "not-a-uuid")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRepository_GetByPublicID_ExcludesSoftDeleted(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	author := seedUser(t, db)
	p := seedDraftPost(t, db, author.ID)
	require.NoError(t, repo.Delete(context.Background(), p))

	_, err := repo.GetByPublicID(context.Background(), p.PublicID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRepository_List_PaginationWhitelist(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	author := seedUser(t, db)
	ctx := context.Background()

	charlie := seedPostAt(t, db, author.ID, "Charlie", StatusDraft, time.Now().Add(-3*time.Hour))
	alpha := seedPostAt(t, db, author.ID, "Alpha", StatusPublished, time.Now().Add(-2*time.Hour))
	bravo := seedPostAt(t, db, author.ID, "Bravo", StatusDraft, time.Now().Add(-1*time.Hour))

	byTitle, _, err := repo.List(ctx, ListQuery{Page: 1, PerPage: 15, Sort: "title", Order: "asc"})
	require.NoError(t, err)
	require.Len(t, byTitle, 3)
	assert.Equal(t, []string{alpha.Title, bravo.Title, charlie.Title}, titles(byTitle))

	// Unknown sort key falls back to created_at (never interpolated into SQL).
	fallback, _, err := repo.List(ctx, ListQuery{Page: 1, PerPage: 15, Sort: "id;drop table posts", Order: "asc"})
	require.NoError(t, err)
	require.Len(t, fallback, 3)
	assert.Equal(t, []string{charlie.Title, alpha.Title, bravo.Title}, titles(fallback))

	published, total, err := repo.List(ctx, ListQuery{
		Page: 1, PerPage: 15,
		Filters: map[string]string{"status": StatusPublished},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, published, 1)
	assert.Equal(t, alpha.Title, published[0].Title)

	// Unknown filter keys are ignored — not used as column names.
	ignored, total, err := repo.List(ctx, ListQuery{
		Page: 1, PerPage: 15,
		Filters: map[string]string{"author_id": "1", "title": "Alpha"},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, ignored, 3)
}

func TestRepository_List_PageDefaultsAndCap(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	author := seedUser(t, db)
	for i := 0; i < 16; i++ {
		seedDraftPost(t, db, author.ID)
	}

	page, total, err := repo.List(context.Background(), ListQuery{Page: 0, PerPage: 0})
	require.NoError(t, err)
	assert.Equal(t, int64(16), total)
	assert.Len(t, page, defaultPerPage)

	page2, _, err := repo.List(context.Background(), ListQuery{Page: 2, PerPage: 15})
	require.NoError(t, err)
	assert.Len(t, page2, 1)
}

func TestRepository_GetByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	_, err := repo.GetByID(context.Background(), 999)
	assert.ErrorIs(t, err, ErrNotFound)
}

func seedPostAt(t *testing.T, db *gorm.DB, authorID uint64, title, status string, at time.Time) *Post {
	t.Helper()
	p := seedDraftPost(t, db, authorID)
	p.Title = title
	p.Status = status
	p.CreatedAt = at
	p.UpdatedAt = at
	require.NoError(t, db.Save(p).Error)
	return p
}

func titles(posts []Post) []string {
	out := make([]string, len(posts))
	for i := range posts {
		out[i] = posts[i].Title
	}
	return out
}
