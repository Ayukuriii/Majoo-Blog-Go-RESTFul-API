package comment

import (
	"time"

	"gorm.io/gorm"
)

type Comment struct {
	ID        uint64         `gorm:"column:id;primaryKey"`
	PublicID  string         `gorm:"column:public_id;size:36;uniqueIndex;not null"`
	PostID    uint64         `gorm:"column:post_id;index;not null"`
	AuthorID  uint64         `gorm:"column:author_id;index;not null"`
	Body      string         `gorm:"column:body;type:text;not null"`
	CreatedAt time.Time      `gorm:"column:created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (Comment) TableName() string {
	return "comments"
}

// CreateCommentRequest is the create body. Parent is a public_id, never post_id.
type CreateCommentRequest struct {
	PostPublicID string `json:"post_public_id" validate:"required"`
	Body         string `json:"body" validate:"required"`
}
type AuthorRef struct {
	PublicID string `json:"public_id"`
}
type PostRef struct {
	PublicID string `json:"public_id"`
}

// CommentResponse never includes id, post_id, or author_id.
type CommentResponse struct {
	PublicID  string    `json:"public_id"`
	Post      PostRef   `json:"post"`
	Author    AuthorRef `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
