// Copyright 2026 The kpt and Nephio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package porchcontext

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestGetRequestID(t *testing.T) {
	id := uuid.New()
	ctx := WithRequestID(context.Background(), id)
	if got := GetRequestID(ctx); got != id {
		t.Errorf("got %v, want %v", got, id)
	}
	//nolint:SA1012 // intentionally testing nil context handling
	if got := GetRequestID(nil); got != uuid.Nil {
		t.Errorf("got %v, want uuid.Nil", got)
	}
}

func TestWithNewRequestID(t *testing.T) {
	ctx := WithNewRequestID(context.Background())
	if got := GetRequestID(ctx); got == uuid.Nil {
		t.Error("expected non-nil UUID")
	}
}

func TestGetPackageRevision(t *testing.T) {
	ctx := WithPackageRevision(context.Background(), "test-pr")
	if got := GetPackageRevision(ctx); got != "test-pr" {
		t.Errorf("got %v, want test-pr", got)
	}
	//nolint:SA1012 // intentionally testing nil context handling
	if got := GetPackageRevision(nil); got != EmptyPRName {
		t.Errorf("got %v, want %v", got, EmptyPRName)
	}
}

func TestWithNewRequestIDAndPackageRevision(t *testing.T) {
	ctx := WithNewRequestIDAndPackageRevision(context.Background(), "test-pr")
	if got := GetRequestID(ctx); got == uuid.Nil {
		t.Error("expected non-nil UUID")
	}
	if got := GetPackageRevision(ctx); got != "test-pr" {
		t.Errorf("got %v, want test-pr", got)
	}
}

func TestLogMetadataFrom(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want int
	}{
		{"nil context", nil, 0}, //nolint:SA1012
		{"empty context", context.Background(), 0},
		{"with requestID", WithRequestID(context.Background(), uuid.New()), 2},
		{"with packageRevision", WithPackageRevision(context.Background(), "test-pr"), 2},
		{"with both", WithPackageRevision(WithRequestID(context.Background(), uuid.New()), "test-pr"), 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LogMetadataFrom(tt.ctx); len(got) != tt.want {
				t.Errorf("got len %d, want %d", len(got), tt.want)
			}
		})
	}
}

func TestLogMetadataFromWithExtras(t *testing.T) {
	ctx := WithRequestID(context.Background(), uuid.New())
	got := LogMetadataFromWithExtras(ctx, "key", "value")
	if len(got) != 4 {
		t.Errorf("got len %d, want 4", len(got))
	}
}
