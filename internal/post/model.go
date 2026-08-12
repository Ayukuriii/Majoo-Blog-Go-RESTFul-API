package post

import (
	"time"

	"gorm.io/gorm"
)

type Post struct {
	ID        uint64         `gorm:"column:id;primaryKey"`
	PublicID  string         `gorm:"column:public_id;size:36;uniqueIndex;not null"`
	AuthorID  uint64         `gorm:"column:author_id;index;not null"`
	Title     string         `gorm:"column:title;size:255;not null"`
	Body      string         `gorm:"column:body;type:text;not null"`
	Status    string         `gorm:"column:status;size:20;not null;default:draft"`
	CreatedAt time.Time      `gorm:"column:created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (Post) TableName() string {
	return "posts"
}

// CreatePostRequest is the body for creating a post.
type CreatePostRequest struct {
	Title string `json:"title" validate:"required,max=255"`
	Body  string `json:"body" validate:"required"`
}

// UpdatePostRequest is the body for partial post updates.
// Pointers distinguish "omitted" from "set to empty string".
type UpdatePostRequest struct {
	Title *string `json:"title" validate:"omitempty,max=255"`
	Body  *string `json:"body" validate:"omitempty"`
}

// AuthorRef is the nested author object on PostResponse.
// The service fills PublicID from the users table — never from author_id.
type AuthorRef struct {
	PublicID string `json:"public_id"`
}

// PostResponse is the public post DTO (no id, no author_id).
type PostResponse struct {
	PublicID  string    `json:"public_id"`
	Author    AuthorRef `json:"author"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}