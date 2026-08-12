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

func TestList_LoadsAuthorsInOneBatch(t *testing.T) {
	posts := []Post{
		{PublicID: "post-1", AuthorID: 1, Title: "A", Body: "a", Status: StatusDraft},
		{PublicID: "post-2", AuthorID: 2, Title: "B", Body: "b", Status: StatusDraft},
		{PublicID: "post-3", AuthorID: 1, Title: "C", Body: "c", Status: StatusDraft},
	}
	users := &stubUsers{
		byID: map[uint64]*user.User{
			1: {ID: 1, PublicID: "author-a"},
			2: {ID: 2, PublicID: "author-b"},
		},
	}
	svc := NewService(&stubRepo{posts: posts, total: 3}, users, nil, validator.New())

	out, _, _, err := svc.List(context.Background(), ListQuery{Page: 1, PerPage: 15})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if users.getByIDsCalls != 1 {
		t.Fatalf("GetByIDs calls = %d, want 1", users.getByIDsCalls)
	}
	if len(users.lastGetByIDs) != 2 {
		t.Fatalf("GetByIDs ids = %v, want 2 unique author ids", users.lastGetByIDs)
	}
	if len(out) != 3 {
		t.Fatalf("len(out) = %d, want 3", len(out))
	}
	if out[0].Author.PublicID != "author-a" || out[1].Author.PublicID != "author-b" || out[2].Author.PublicID != "author-a" {
		t.Fatalf("unexpected authors: %+v", out)
	}
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
	post  *Post
	posts []Post
	total int64
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
	return s.posts, s.total, nil
}
func (s *stubRepo) Update(context.Context, *Post) error                 { return nil }
func (s *stubRepo) Delete(context.Context, *Post) error                 { return nil }
func (s *stubRepo) CreatePublishLog(context.Context, *PublishLog) error { return nil }

type stubUsers struct {
	byPublicID    map[string]*user.User
	byID          map[uint64]*user.User
	getByIDsCalls int
	lastGetByIDs  []uint64
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
	s.getByIDsCalls++
	s.lastGetByIDs = append([]uint64(nil), ids...)
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
