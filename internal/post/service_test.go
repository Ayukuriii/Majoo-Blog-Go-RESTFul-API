package post

import (
	"context"
	"errors"
	"testing"

	"blog-api/internal/user"

	"github.com/glebarez/sqlite"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var errForcedPublishLog = errors.New("forced post_publish_log failure")

type failPublishLogRepo struct {
	Repository
	fail error
}

func (r failPublishLogRepo) WithTx(tx *gorm.DB) Repository {
	return failPublishLogRepo{Repository: r.Repository.WithTx(tx), fail: r.fail}
}

func (r failPublishLogRepo) CreatePublishLog(context.Context, *PublishLog) error {
	return r.fail
}

func TestPublishWithSnapshot_LogInsertFailureRollsBackStatus(t *testing.T) {
	db := setupTestDB(t)
	userRepo := user.NewRepository(db)
	postRepo := NewRepository(db)

	author := seedUser(t, db)
	p := seedDraftPost(t, db, author.ID)

	svc := NewService(
		failPublishLogRepo{Repository: postRepo, fail: errForcedPublishLog},
		userRepo,
		db,
		validator.New(),
	)

	_, err := svc.PublishWithSnapshot(context.Background(), author.PublicID, p.PublicID)
	if !errors.Is(err, errForcedPublishLog) {
		t.Fatalf("PublishWithSnapshot error = %v, want %v", err, errForcedPublishLog)
	}

	got, err := postRepo.GetByPublicID(context.Background(), p.PublicID)
	if err != nil {
		t.Fatalf("GetByPublicID: %v", err)
	}
	if got.Status != StatusDraft {
		t.Fatalf("status = %q, want %q (transaction should have rolled back)", got.Status, StatusDraft)
	}

	var logs int64
	if err := db.Model(&PublishLog{}).Where("post_id = ?", p.ID).Count(&logs).Error; err != nil {
		t.Fatalf("count logs: %v", err)
	}
	if logs != 0 {
		t.Fatalf("publish log rows = %d, want 0", logs)
	}
}

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(&user.User{}, &Post{}, &PublishLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedUser(t *testing.T, db *gorm.DB) *user.User {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	u := &user.User{
		PublicID:     id.String(),
		Email:        "author@example.com",
		PasswordHash: "x",
	}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
}

func seedDraftPost(t *testing.T, db *gorm.DB, authorID uint64) *Post {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	p := &Post{
		PublicID: id.String(),
		AuthorID: authorID,
		Title:    "Hello",
		Body:     "World",
		Status:   StatusDraft,
	}
	if err := db.Create(p).Error; err != nil {
		t.Fatalf("seed post: %v", err)
	}
	return p
}

func TestCreate_InvalidPayload(t *testing.T) {
	svc := NewService(&stubRepo{}, &stubUsers{}, nil, validator.New())
	_, err := svc.Create(context.Background(), "author-public-id", CreatePostRequest{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestUpdate_ForbiddenWhenNotOwner(t *testing.T) {
	author := &user.User{ID: 1, PublicID: "author-a"}
	other := &user.User{ID: 2, PublicID: "author-b"}
	existing := &Post{
		ID:       10,
		PublicID: "post-1",
		AuthorID: author.ID,
		Title:    "Hello",
		Body:     "World",
		Status:   StatusDraft,
	}

	svc := NewService(
		&stubRepo{post: existing},
		&stubUsers{byPublicID: map[string]*user.User{other.PublicID: other}},
		nil,
		validator.New(),
	)

	title := "Hacked"
	_, err := svc.Update(context.Background(), other.PublicID, existing.PublicID, UpdatePostRequest{Title: &title})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
}

type stubRepo struct {
	post *Post
}

func (s *stubRepo) WithTx(*gorm.DB) Repository          { return s }
func (s *stubRepo) Create(context.Context, *Post) error { return nil }
func (s *stubRepo) GetByPublicID(context.Context, string) (*Post, error) {
	if s.post == nil {
		return nil, ErrNotFound
	}
	cp := *s.post
	return &cp, nil
}
func (s *stubRepo) List(context.Context, ListQuery) ([]Post, int64, error) {
	return nil, 0, nil
}
func (s *stubRepo) Update(context.Context, *Post) error                 { return nil }
func (s *stubRepo) Delete(context.Context, *Post) error                 { return nil }
func (s *stubRepo) CreatePublishLog(context.Context, *PublishLog) error { return nil }

type stubUsers struct {
	byPublicID map[string]*user.User
}

func (s *stubUsers) WithTx(*gorm.DB) user.Repository          { return s }
func (s *stubUsers) Create(context.Context, *user.User) error { return nil }
func (s *stubUsers) GetByID(context.Context, uint64) (*user.User, error) {
	return nil, user.ErrNotFound
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
