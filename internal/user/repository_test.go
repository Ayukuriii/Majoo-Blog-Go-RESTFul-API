package user

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&User{}))
	return db
}

func seedUserRow(t *testing.T, db *gorm.DB, email string) *User {
	t.Helper()
	id, err := uuid.NewV7()
	require.NoError(t, err)
	u := &User{PublicID: id.String(), Email: email, PasswordHash: "hash"}
	require.NoError(t, db.Create(u).Error)
	return u
}

func TestRepository_GetByPublicID_UnknownVsMalformed(t *testing.T) {
	db := setupUserDB(t)
	repo := NewRepository(db)
	u := seedUserRow(t, db, "a@example.com")
	ctx := context.Background()

	got, err := repo.GetByPublicID(ctx, u.PublicID)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
	assert.Equal(t, u.PublicID, got.PublicID)

	_, err = repo.GetByPublicID(ctx, "0190f0e2-8c3a-7b2d-9e4f-ffffffffffff")
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = repo.GetByPublicID(ctx, "not-a-uuid")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRepository_GetByID_AndGetByIDs(t *testing.T) {
	db := setupUserDB(t)
	repo := NewRepository(db)
	a := seedUserRow(t, db, "a@example.com")
	b := seedUserRow(t, db, "b@example.com")
	ctx := context.Background()

	got, err := repo.GetByID(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, a.PublicID, got.PublicID)

	_, err = repo.GetByID(ctx, 9999)
	assert.ErrorIs(t, err, ErrNotFound)

	users, err := repo.GetByIDs(ctx, []uint64{a.ID, b.ID})
	require.NoError(t, err)
	assert.Len(t, users, 2)

	empty, err := repo.GetByIDs(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestRepository_GetByEmail_NotFound(t *testing.T) {
	db := setupUserDB(t)
	repo := NewRepository(db)
	seedUserRow(t, db, "a@example.com")

	got, err := repo.GetByEmail(context.Background(), "a@example.com")
	require.NoError(t, err)
	assert.Equal(t, "a@example.com", got.Email)

	_, err = repo.GetByEmail(context.Background(), "missing@example.com")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRepository_GetByPublicID_ExcludesSoftDeleted(t *testing.T) {
	db := setupUserDB(t)
	repo := NewRepository(db)
	u := seedUserRow(t, db, "a@example.com")
	require.NoError(t, db.Delete(u).Error)

	_, err := repo.GetByPublicID(context.Background(), u.PublicID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRepository_Create(t *testing.T) {
	db := setupUserDB(t)
	repo := NewRepository(db)
	id, err := uuid.NewV7()
	require.NoError(t, err)
	u := &User{PublicID: id.String(), Email: "new@example.com", PasswordHash: "hash"}
	require.NoError(t, repo.Create(context.Background(), u))
	assert.NotZero(t, u.ID)

	got, err := repo.GetByEmail(context.Background(), "new@example.com")
	require.NoError(t, err)
	assert.Equal(t, u.PublicID, got.PublicID)
}
