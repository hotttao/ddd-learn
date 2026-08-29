package crawl

import (
	"context"
	"sync"

	domain "media_agent/xhs_service/biz/domain/crawl"
)

// MockRepository is intentionally in-memory: this project exercises the
// authentication and authorization chain, not crawl persistence.
type MockRepository struct {
	mu       sync.RWMutex
	tasks    map[string]domain.Task
	keywords map[string][]string
	contents map[string][]domain.Content
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		tasks:    make(map[string]domain.Task),
		keywords: make(map[string][]string),
		contents: make(map[string][]domain.Content),
	}
}

func (r *MockRepository) CreateTask(_ context.Context, task domain.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[task.ID] = task
	return nil
}

func (r *MockRepository) ListContents(_ context.Context, organizationID string) ([]domain.Content, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]domain.Content(nil), r.contents[organizationID]...), nil
}

func (r *MockRepository) GetKeywords(_ context.Context, organizationID string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.keywords[organizationID]...), nil
}

func (r *MockRepository) ReplaceKeywords(_ context.Context, organizationID string, values []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keywords[organizationID] = append([]string(nil), values...)
	return nil
}
