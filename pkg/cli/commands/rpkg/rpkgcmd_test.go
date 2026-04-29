package rpkg

import (
	"context"
	"testing"
)

func TestNewCommand(t *testing.T) {
	cmd := NewCommand(context.Background(), "test")
	if cmd == nil {
		t.Fatal("NewCommand returned nil")
	}
	if cmd.Use != "rpkg" {
		t.Errorf("expected Use=rpkg, got %s", cmd.Use)
	}
}
