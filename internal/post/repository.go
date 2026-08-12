package post

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrNotFound = errors.New("post not found")

const (
	defaultPage    = 1
	defaultPerPage = 15
	maxPerPage     = 100
	defaultSort    = "created_at"
)

// Hardcoded column names only. Client strings are keys into these maps,
// never concatenated into SQL.
var (
	allowedSort = map[string]string{
		"created_at": "created_at",
		"title":      "title",
	}
	allowedFilters = map[string]struct{}{
		"status": {},
	}
)

// ListQuery is the typed list input from the handler/service.
type ListQuery struct {
	Page    int
	PerPage int
	Search  string
	Sort    string
	Order   string // asc | desc
	Filters map[string]string
}

// Repository persists and loads posts.
type Repository interface {
	Create(ctx context.Context, post *Post) error
	GetByPublicID(ctx context.Context, publicID string) (*Post, error)
	List(ctx context.Context, q ListQuery) (posts []Post, total int64, err error)
	Update(ctx context.Context, post *Post) error
	Delete(ctx context.Context, post *Post) error
}

type repository struct {
	db *gorm.DB
}

// NewRepository reurns a GORM-backed post Repository.
func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

func (r *repository) Create(ctx context.Context, post *Post) error {
	return r.db.WithContext(ctx).Create(post).Error
}

func (r *repository) GetByPublicID(ctx context.Context, publicID string) (*Post, error) {
	var p Post
	err := r.db.WithContext(ctx).Where("public_id = ?", publicID).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (r *repository) List(ctx context.Context, q ListQuery) ([]Post, int64, error) {
	page, perPage := normalizePage(q.Page, q.PerPage)
	db := r.applyListFilters(r.db.WithContext(ctx).Model(&Post{}), q)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var posts []Post
	err := db.
		Order(orderClause(q.Sort, q.Order)).
		Offset((page - 1) * perPage).
		Limit(perPage).
		Find(&posts).Error
	if err != nil {
		return nil, 0, err
	}
	return posts, total, nil
}

func (r *repository) Update(ctx context.Context, post *Post) error {
	return r.db.WithContext(ctx).Save(post).Error
}

func (r *repository) Delete(ctx context.Context, post *Post) error {
	// Soft-delete: GORM sets deleted_at because Post has gorm.DeletedAt.
	return r.db.WithContext(ctx).Delete(post).Error
}

func (r *repository) applyListFilters(db *gorm.DB, q ListQuery) *gorm.DB {
	if q.Search != "" {
		db = db.Where("title LIKE ?", "%"+q.Search+"%")
	}

	for key, value := range q.Filters {
		if value == "" {
			continue
		}
		if _, ok := allowedFilters[key]; !ok {
			continue // unknown filter key is ignored, never use as a column
		}
		switch key {
		case "status":
			db = db.Where("status = ?", value)
		}
	}
	return db
}

func normalizePage(page, perPage int) (int, int) {
	if page < 1 {
		page = defaultPage
	}
	if perPage < 1 {
		perPage = defaultPerPage
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}
	return page, perPage
}

func orderClause(sort, order string) clause.OrderByColumn {
	col, ok := allowedSort[sort]
	if !ok {
		col = allowedSort[defaultSort]
	}
	return clause.OrderByColumn{
		Column: clause.Column{Name: col},
		Desc:   strings.EqualFold(order, "desc"),
	}
}
