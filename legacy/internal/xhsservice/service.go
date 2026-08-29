package xhsservice

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var ErrInvalidKeyword = errors.New("keyword must not be empty")

type CrawlTask struct {
	ID          string    `json:"id"`
	Keyword     string    `json:"keyword"`
	Status      string    `json:"status"`
	RequestedBy string    `json:"requested_by"`
	CreatedAt   time.Time `json:"created_at"`
}

type Content struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Keyword   string    `json:"keyword"`
	CreatedAt time.Time `json:"created_at"`
}

type Service struct {
	mu       sync.RWMutex
	keyword  string
	contents []Content
	sequence atomic.Uint64
	now      func() time.Time
}

func New() *Service {
	return &Service{
		keyword: "AI Agent",
		contents: []Content{
			{ID: "content-001", Title: "Hertz 微服务入门", Keyword: "AI Agent", CreatedAt: time.Now().UTC()},
		},
		now: time.Now,
	}
}

func (s *Service) StartCrawl(subject string) CrawlTask {
	s.mu.RLock()
	keyword := s.keyword
	s.mu.RUnlock()
	return CrawlTask{
		ID: fmt.Sprintf("task-%06d", s.sequence.Add(1)), Keyword: keyword, Status: "queued",
		RequestedBy: subject, CreatedAt: s.now().UTC(),
	}
}

func (s *Service) Contents() []Content {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Content, len(s.contents))
	copy(result, s.contents)
	return result
}

func (s *Service) UpdateKeyword(keyword string) (string, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return "", ErrInvalidKeyword
	}
	s.mu.Lock()
	s.keyword = keyword
	s.mu.Unlock()
	return keyword, nil
}

func (s *Service) Keyword() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.keyword
}
