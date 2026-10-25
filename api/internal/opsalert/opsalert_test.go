package opsalert

import (
	"context"
	"testing"

	"gopkg.aoctech.app/api-commons/alerts"
)

func TestNewEmptyTopicIsNop(t *testing.T) {
	if _, ok := New(context.Background(), "us-east-1", "", "prod").(alerts.Nop); !ok {
		t.Fatal("empty topic must give Nop")
	}
}
