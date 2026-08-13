package comment

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrNotFound = errors.New("comment not found")

const (
	defaultPage    = 1
	defaultPerPage = 15
	maxPerPage     = 100
	defaultSort    = "created_at"
)

var allowedSort = map[string]string{
	"created_at": "created_at",
}

// ListQuery is parsed in the handler. PostID is set by the service after resolve.
type ListQuery struct {
	Page    int
	PerPage int
	Search  string
	Sort    string
	Order   string
	Path    string
	PostID  uint64 // internal FK only — never from the client
}

type Repository interface {
	WithTx(tx *gorm.DB) Repository
	Create(ctx context.Context, comment *Comment) error
	GetByPublicID(ctx context.Context, publicID string) (*Comment, error)
	ListByPostID(ctx context.Context, q ListQuery) (comments []Comment, total int64, err error)
	Delete(ctx context.Context, comment *Comment) error
}
type repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}
func (r *repository) WithTx(tx *gorm.DB) Repository {
	return &repository{db: tx}
}
func (r *repository) Create(ctx context.Context, comment *Comment) error {
	return r.db.WithContext(ctx).Create(comment).Error
}
func (r *repository) GetByPublicID(ctx context.Context, publicID string) (*Comment, error) {
	var c Comment
	err := r.db.WithContext(ctx).Where("public_id = ?", publicID).First(&c).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}
func (r *repository) ListByPostID(ctx context.Context, q ListQuery) ([]Comment, int64, error) {
	page, perPage := normalizePage(q.Page, q.PerPage)
	db := r.db.WithContext(ctx).Model(&Comment{}).Where("post_id = ?", q.PostID)
	if q.Search != "" {
		db = db.Where("body LIKE ?", "%"+q.Search+"%")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var comments []Comment
	err := db.
		Order(orderClause(q.Sort, q.Order)).
		Offset((page - 1) * perPage).
		Limit(perPage).
		Find(&comments).Error
	if err != nil {
		return nil, 0, err
	}
	return comments, total, nil
}
func (r *repository) Delete(ctx context.Context, comment *Comment) error {
	return r.db.WithContext(ctx).Delete(comment).Error
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
