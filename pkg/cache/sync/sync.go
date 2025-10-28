// Copyright 2025 The kpt and Nephio Authors
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
	"time"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/robfig/cron/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	timeFormat        = "2006-01-02 15:04:05"
	nextSyncLogFormat = "repositorySync %+v: next scheduled time: %v"
)

// SyncHandler defines the interface for repository sync operations
type SyncHandler interface {
	// SyncOnce performs a single sync operation
	SyncOnce(ctx context.Context) error
	// Key returns the repository key
	Key() repository.RepositoryKey
	// GetSpec returns the repository spec
	GetSpec() *configapi.Repository
}

// SyncManager handles repository synchronization with scheduling
type SyncManager struct {
	handler       SyncHandler
	cancel        context.CancelFunc
	nextSyncTime  *time.Time
	syncCountdown time.Duration
	lastCronExpr  string
	lastSyncError error
	coreClient    client.WithWatch
}

// NewSyncManager creates a new sync manager
func NewSyncManager(handler SyncHandler, coreClient client.WithWatch) *SyncManager {
	m := &SyncManager{
		handler:    handler,
		coreClient: coreClient,
	}
	return m
}

// Start begins the sync process with periodic and one-time scheduling
func (m *SyncManager) Start(ctx context.Context, defaultSyncFrequency time.Duration) {
	// Create cancellable context for goroutines
	syncCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	go m.syncForever(syncCtx, defaultSyncFrequency)
	go m.handleRunOnceAt(syncCtx)
}

// Stop stops the sync manager
func (m *SyncManager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

// GetLastSyncError returns the last sync error
func (m *SyncManager) GetLastSyncError() error {
	return m.lastSyncError
}

func (m *SyncManager) syncForever(ctx context.Context, defaultSyncFrequency time.Duration) {
	// Sync once at startup
	m.lastSyncError = m.handler.SyncOnce(ctx)
	m.syncCountdown = m.calculateWaitDuration(defaultSyncFrequency)
	klog.Infof(nextSyncLogFormat, m.handler.Key(), m.nextSyncTime)
	m.updateRepositoryCondition(ctx)

	tickInterval := 5 * time.Second
	if defaultSyncFrequency < 10*time.Second {
		tickInterval = 1 * time.Second
	}
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			klog.Infof("repositorySync %+v: exiting repository sync, because context is done: %v", m.handler.Key(), ctx.Err())
			return
		case <-ticker.C:
			currentCronExpr := ""
			if m.hasValidSyncSpec() {
				currentCronExpr = m.handler.GetSpec().Spec.Sync.Schedule
			}
			if m.lastCronExpr != currentCronExpr {
				m.lastCronExpr = currentCronExpr
				m.syncCountdown = m.calculateWaitDuration(defaultSyncFrequency)
				klog.Infof(nextSyncLogFormat, m.handler.Key(), m.nextSyncTime)
				m.updateRepositoryCondition(ctx)
				continue
			}
			m.syncCountdown -= tickInterval
			if m.syncCountdown <= 0 {
				m.lastSyncError = m.handler.SyncOnce(ctx)
				m.syncCountdown = m.calculateWaitDuration(defaultSyncFrequency)
				klog.Infof(nextSyncLogFormat, m.handler.Key(), m.nextSyncTime)
				m.updateRepositoryCondition(ctx)
			}
		}
	}
}

func (m *SyncManager) handleRunOnceAt(ctx context.Context) {
	var runOnceTimer *time.Timer
	var runOnceChan <-chan time.Time
	var scheduledRunOnceAt time.Time
	ctxDoneLog := "repositorySync %+v: exiting repository handleRunOnceAt sync, because context is done: %v"

	ticker := time.NewTicker(5 * time.Second)
	defer func() {
		ticker.Stop()
		if runOnceTimer != nil {
			runOnceTimer.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			klog.Infof(ctxDoneLog, m.handler.Key(), ctx.Err())
			return
		case <-runOnceChan:
			klog.Infof("repositorySync %+v: Triggering scheduled one-time sync", m.handler.Key())
			m.lastSyncError = m.handler.SyncOnce(context.Background())
			m.updateRepositoryCondition(ctx)
			klog.Infof("repositorySync %+v: Finished one-time sync", m.handler.Key())
			runOnceTimer, runOnceChan, scheduledRunOnceAt = nil, nil, time.Time{}
		case <-ticker.C:
			if !m.hasValidSyncSpec() {
				klog.V(2).Infof("repositorySync %+v: repo or sync spec is nil, skipping runOnceAt check", m.handler.Key())
				continue
			}

			runOnceAt := m.handler.GetSpec().Spec.Sync.RunOnceAt
			if m.shouldScheduleRunOnce(runOnceAt, scheduledRunOnceAt) {
				if runOnceTimer != nil {
					runOnceTimer.Stop()
					runOnceTimer = nil
				}
				delay := time.Until(runOnceAt.Time)
				if delay > 0 {
					klog.Infof("repositorySync %+v: Scheduling one-time sync at %s", m.handler.Key(), runOnceAt.Time.Format(timeFormat))
					runOnceTimer = time.NewTimer(delay)
					runOnceChan = runOnceTimer.C
					scheduledRunOnceAt = runOnceAt.Time
				} else {
					klog.V(2).Infof("repositorySync %+v: runOnceAt time is in the past (%s), skipping", m.handler.Key(), runOnceAt.Time.Format(time.RFC3339))
					runOnceTimer, runOnceChan, scheduledRunOnceAt = nil, nil, time.Time{}
				}
			}
		}
	}
}

func (m *SyncManager) setDefaultNextSyncTime(defaultDuration time.Duration) time.Duration {
	next := time.Now().Add(defaultDuration)
	m.nextSyncTime = &next
	return defaultDuration
}

func (m *SyncManager) calculateWaitDuration(defaultDuration time.Duration) time.Duration {
	if !m.hasValidSyncSpec() {
		klog.Warningf("repositorySync %+v: repo or sync spec is nil, falling back to default interval: %v", m.handler.Key(), defaultDuration)
		return m.setDefaultNextSyncTime(defaultDuration)
	}

	cronExpr := m.handler.GetSpec().Spec.Sync.Schedule
	if cronExpr == "" {
		klog.V(2).Infof("repositorySync %+v: sync.schedule is empty, falling back to default interval: %v", m.handler.Key(), defaultDuration)
		return m.setDefaultNextSyncTime(defaultDuration)
	}

	schedule, err := cron.ParseStandard(cronExpr)
	if err != nil {
		klog.Warningf("repositorySync %+v: invalid cron expression '%s', falling back to default interval: %v", m.handler.Key(), cronExpr, defaultDuration)
		return m.setDefaultNextSyncTime(defaultDuration)
	}

	next := schedule.Next(time.Now())
	m.nextSyncTime = &next
	return time.Until(next)
}

func (m *SyncManager) hasValidSyncSpec() bool {
	spec := m.handler.GetSpec()
	return spec != nil && spec.Spec.Sync != nil
}

func (m *SyncManager) shouldScheduleRunOnce(runOnceAt *metav1.Time, scheduled time.Time) bool {
	return runOnceAt != nil && !runOnceAt.IsZero() && (scheduled.IsZero() || !runOnceAt.Time.Equal(scheduled))
}

func (m *SyncManager) updateRepositoryCondition(ctx context.Context) {
	status := "ready"
	if m.lastSyncError != nil {
		klog.Warningf("repositorySync %+v: sync error: %v", m.handler.Key(), m.lastSyncError)
		status = "error"
	}
	if err := m.SetRepositoryCondition(ctx, status); err != nil {
		klog.Warningf("repositorySync %+v: failed to set repository condition: %v", m.handler.Key(), err)
	}
}

func (m *SyncManager) SetRepositoryCondition(ctx context.Context, status string) error {
	return SetRepositoryCondition(ctx, m.coreClient, m.handler.Key(), status, m.lastSyncError, m.nextSyncTime)
}
