package crawl

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "media_agent/xhs_service/biz/domain/crawl"
)

func TestServiceAuthorizationAndMockPersistence(t *testing.T) {
	checker := &fakeChecker{allowed: true}
	repository := NewMockRepository()
	now := time.Date(2026, 8, 29, 3, 0, 0, 0, time.UTC)
	svc := New(repository, checker, func() string { return "task-1" }, func() time.Time { return now })

	task, err := svc.StartTask(context.Background(), StartTaskCommand{
		Subject: "identity:alice", OrganizationID: "G", Keywords: []string{" 技术 ", "技术", "Agent"},
	})
	if err != nil {
		t.Fatalf("StartTask() error = %v", err)
	}
	if task.ID != "task-1" || len(task.Keywords) != 2 {
		t.Fatalf("StartTask() = %#v", task)
	}

	checker.allowed = false
	if _, err := svc.UpdateKeywords(context.Background(), UpdateKeywordsCommand{
		Subject: "identity:bob", OrganizationID: "G", Keywords: []string{"Agent"},
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("UpdateKeywords() error = %v, want ErrForbidden", err)
	}
	checker.allowed = true
	keywords, err := svc.GetKeywords(context.Background(), OrganizationQuery{Subject: "identity:alice", OrganizationID: "G"})
	if err != nil {
		t.Fatalf("GetKeywords() error = %v", err)
	}
	if len(keywords) != 0 {
		t.Fatalf("GetKeywords() = %v, want unchanged mock data", keywords)
	}
}

func TestServiceFailsClosedWhenKetoUnavailable(t *testing.T) {
	svc := New(NewMockRepository(), &fakeChecker{err: errors.New("Keto down")}, func() string { return "task-1" }, time.Now)
	if _, err := svc.StartTask(context.Background(), StartTaskCommand{
		Subject: "identity:alice", OrganizationID: "G", Keywords: []string{"技术"},
	}); !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("StartTask() error = %v, want ErrDependencyUnavailable", err)
	}
}

func TestMockPermissionCheckerDeniesReservedOrganization(t *testing.T) {
	checker := MockPermissionChecker{}
	allowed, err := checker.Check(context.Background(), "identity:alice", "Organization", "forbidden", "view_crawl_content")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if allowed {
		t.Fatal("Check() allowed reserved organization, want denied")
	}
}

func TestMockRepositoryProvidesDemoContents(t *testing.T) {
	contents, err := NewMockRepository().ListContents(context.Background(), "demo")
	if err != nil {
		t.Fatalf("ListContents() error = %v", err)
	}
	if len(contents) != 2 || contents[0].ID != "note-001" {
		t.Fatalf("ListContents() = %#v, want fixed demo contents", contents)
	}
}

func TestMockRepositoryProvidesOrganizationGContents(t *testing.T) {
	contents, err := NewMockRepository().ListContents(context.Background(), "G")
	if err != nil {
		t.Fatalf("ListContents() error = %v", err)
	}
	if len(contents) != 2 || contents[1].SourceKeyword != "Hertz" {
		t.Fatalf("ListContents() = %#v, want fixed organization G contents", contents)
	}
}

func TestNormalizeKeywords(t *testing.T) {
	got, err := domain.NormalizeKeywords([]string{" a ", "a", "b"})
	if err != nil || len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("NormalizeKeywords() = %v, %v", got, err)
	}
}

type fakeChecker struct {
	allowed bool
	err     error
}

func (f *fakeChecker) Check(context.Context, string, string, string, string) (bool, error) {
	return f.allowed, f.err
}
