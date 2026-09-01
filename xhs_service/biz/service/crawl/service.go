package crawl

import (
	"context"
	"errors"
	"fmt"
	"time"

	domain "media_agent/xhs_service/biz/domain/crawl"
)

var (
	ErrUnauthenticated       = errors.New("unauthenticated")
	ErrForbidden             = errors.New("permission denied")
	ErrDependencyUnavailable = errors.New("authorization dependency unavailable")
)

const (
	organizationNamespace = "Organization"
	userNamespace         = "User"

	PermissionStartCrawl     = "start_crawl"
	PermissionViewContent    = "view_content"
	PermissionModifyKeywords = "modify_keywords"
)

type PermissionChecker interface {
	Check(ctx context.Context, subject, namespace, object, relation string) (bool, error)
}

type Service struct {
	repository  domain.Repository
	permissions PermissionChecker
	newID       func() string
	now         func() time.Time
}

func New(repository domain.Repository, permissions PermissionChecker, newID func() string, now func() time.Time) *Service {
	return &Service{repository: repository, permissions: permissions, newID: newID, now: now}
}

func (s *Service) StartTask(ctx context.Context, command StartTaskCommand) (domain.Task, error) {
	organizationID, err := domain.NormalizeOrganizationID(command.OrganizationID)
	if err != nil {
		return domain.Task{}, err
	}
	keywords, err := domain.NormalizeKeywords(command.Keywords)
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.authorize(ctx, command.Subject, organizationID, PermissionStartCrawl); err != nil {
		return domain.Task{}, err
	}
	task := domain.Task{ID: s.newID(), OrganizationID: organizationID, Keywords: keywords, Status: "pending", CreatedAt: s.now().UTC()}
	if err := s.repository.CreateTask(ctx, task); err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

func (s *Service) ListContents(ctx context.Context, query OrganizationQuery) ([]domain.Content, error) {
	organizationID, err := domain.NormalizeOrganizationID(query.OrganizationID)
	if err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, query.Subject, organizationID, PermissionViewContent); err != nil {
		return nil, err
	}
	return s.repository.ListContents(ctx, organizationID)
}

func (s *Service) GetKeywords(ctx context.Context, query OrganizationQuery) ([]string, error) {
	organizationID, err := domain.NormalizeOrganizationID(query.OrganizationID)
	if err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, query.Subject, organizationID, PermissionViewContent); err != nil {
		return nil, err
	}
	return s.repository.GetKeywords(ctx, organizationID)
}

func (s *Service) UpdateKeywords(ctx context.Context, command UpdateKeywordsCommand) ([]string, error) {
	organizationID, err := domain.NormalizeOrganizationID(command.OrganizationID)
	if err != nil {
		return nil, err
	}
	keywords, err := domain.NormalizeKeywords(command.Keywords)
	if err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, command.Subject, organizationID, PermissionModifyKeywords); err != nil {
		return nil, err
	}
	if err := s.repository.ReplaceKeywords(ctx, organizationID, keywords); err != nil {
		return nil, err
	}
	return keywords, nil
}

func (s *Service) authorize(ctx context.Context, subject, object, relation string) error {
	if subject == "" {
		return ErrUnauthenticated
	}
	allowed, err := s.permissions.Check(ctx, userNamespace+":"+subject, organizationNamespace, object, relation)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDependencyUnavailable, err)
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}
