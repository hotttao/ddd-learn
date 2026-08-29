package crawl

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrInvalidInput = errors.New("invalid crawl input")

type Task struct {
	ID             string
	OrganizationID string
	Keywords       []string
	Status         string
	CreatedAt      time.Time
}

type Content struct {
	ID            string
	Title         string
	SourceKeyword string
}

type Repository interface {
	CreateTask(context.Context, Task) error
	ListContents(context.Context, string) ([]Content, error)
	GetKeywords(context.Context, string) ([]string, error)
	ReplaceKeywords(context.Context, string, []string) error
}

func NormalizeOrganizationID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 255 {
		return "", ErrInvalidInput
	}
	return value, nil
}

func NormalizeKeywords(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > 100 {
		return nil, ErrInvalidInput
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 128 {
			return nil, ErrInvalidInput
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil, ErrInvalidInput
	}
	return result, nil
}
