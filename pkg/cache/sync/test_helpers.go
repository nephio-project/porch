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

package sync

import (
	"context"
	"fmt"

	configapi "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	"k8s.io/apimachinery/pkg/runtime"
)

// testSyncHandler implements SyncHandler for testing
type testSyncHandler struct {
	repoKey   repository.RepositoryKey
	spec      *configapi.Repository
	syncCount int
	syncError error
}

func (m *testSyncHandler) SyncOnce(ctx context.Context) error {
	m.syncCount++
	return m.syncError
}

func (m *testSyncHandler) Key() repository.RepositoryKey {
	return m.repoKey
}

func (m *testSyncHandler) GetSpec() *configapi.Repository {
	return m.spec
}

// testEventRecorder for testing
type testEventRecorder struct {
	events []testEvent
}

type testEvent struct {
	eventType   string
	reason      string
	message     string
	annotations map[string]string
}

func (m *testEventRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	m.events = append(m.events, testEvent{
		eventType: eventtype,
		reason:    reason,
		message:   message,
	})
}

func (m *testEventRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	m.Event(object, eventtype, reason, fmt.Sprintf(messageFmt, args...))
}

func (m *testEventRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	m.events = append(m.events, testEvent{
		eventType:   eventtype,
		reason:      reason,
		message:     fmt.Sprintf(messageFmt, args...),
		annotations: annotations,
	})
}