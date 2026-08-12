package user

import (
	"time"

	"gorm.io/gorm"
)

// User is the GORM entity for the users table.
type User struct {
	ID           uint64         `gorm:"column:id;primaryKey"`
	PublicID     string         `gorm:"column:public_id;size:36;uniqueIndex;not null"`
	Email        string         `gorm:"column:email;size:255;uniqueIndex;not null"`
	PasswordHash string         `gorm:"column:password_hash;size:255;not null"`
	DisplayName  *string        `gorm:"column:display_name;size:255"`
	CreatedAt    time.Time      `gorm:"column:created_at"`
	UpdatedAt    time.Time      `gorm:"column:updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (User) TableName() string {
	return "users"
}

// RegisterRequest is the body for user registration.
type RegisterRequest struct {
	Email       string `json:"email" validate:"required,email"`
	Password    string `json:"password" validate:"required,min=8"`
	DisplayName string `json:"display_name"`
}

// LoginRequest is the body for user login.
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// UserResponse is the public user DTO (no id or password_hash).
type UserResponse struct {
	PublicID    string    `json:"public_id"`
	Email       string    `json:"email"`
	DisplayName *string   `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
