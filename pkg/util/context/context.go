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

package context

import (
	"context"

	"github.com/google/uuid"
)

type porchContextKey string

const (
	requestIDKey       porchContextKey = "requestID"
	packageRevisionKey porchContextKey = "packageRevision"

	EmptyPRName = "<undefined>"
)

func getter[T any](ctx context.Context, key any, defVal T) T {
	if ctx != nil {
		if v := ctx.Value(key); v != nil {
			if vv, ok := v.(T); ok {
				return vv
			}
		}
	}
	return defVal
}

func GetRequestID(ctx context.Context) uuid.UUID {
	return getter(ctx, requestIDKey, uuid.Nil)
}

func WithRequestID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

func WithNewRequestID(ctx context.Context) context.Context {
	return context.WithValue(ctx, requestIDKey, uuid.New())
}

func GetPackageRevision(ctx context.Context) string {
	return getter(ctx, packageRevisionKey, EmptyPRName)
}

func WithPackageRevision(ctx context.Context, prName string) context.Context {
	return context.WithValue(ctx, packageRevisionKey, prName)
}

func WithNewRequestIDAndPackageRevision(ctx context.Context, prName string) context.Context {
	return WithPackageRevision(WithNewRequestID(ctx), prName)
}
