package utils

import (
	"github.com/google/uuid"
)

// GenerateSessionID 生成一个基于UUID的会话ID
func GenerateSessionID() string {
	return uuid.New().String()
}
