package xhsservice

import "testing"

func TestServiceWorkflow(t *testing.T) {
	service := New()
	keyword, err := service.UpdateKeyword("技术")
	if err != nil {
		t.Fatal(err)
	}
	if keyword != "技术" || service.Keyword() != "技术" {
		t.Fatalf("keyword=%q stored=%q", keyword, service.Keyword())
	}
	task := service.StartCrawl("alice-id")
	if task.Keyword != "技术" || task.RequestedBy != "alice-id" || task.Status != "queued" {
		t.Fatalf("unexpected task: %#v", task)
	}
	if _, err := service.UpdateKeyword("   "); err != ErrInvalidKeyword {
		t.Fatalf("got %v, want ErrInvalidKeyword", err)
	}
}
