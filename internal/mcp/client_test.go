package mcp

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewRequiresURL(t *testing.T) {
	_, err := New(context.Background(), "", "token", time.Second)
	if !errors.Is(err, ErrNoURL) {
		t.Fatalf("want ErrNoURL for empty url, got %v", err)
	}
}
