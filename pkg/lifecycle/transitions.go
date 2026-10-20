// Copyright 2026 The kpt Authors
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

package lifecycle

import (
	"fmt"
	"slices"
	"strings"
)

// Options controls lifecycle policy enforced by the API server and controllers.
// Database triggers only implement technical rules; they do not read this policy.
type Options struct {
	AllowDirectPublicationFromDraft bool
}

// Stage names match porch PackageRevision lifecycle values.
const (
	StageDraft            = "Draft"
	StageProposed         = "Proposed"
	StagePublished        = "Published"
	StageDeletionProposed = "DeletionProposed"
)

// AllowedTransitions returns the lifecycle state machine for the given options.
func AllowedTransitions(opts Options) map[string][]string {
	draftTargets := []string{StageProposed}
	if opts.AllowDirectPublicationFromDraft {
		draftTargets = append(draftTargets, StagePublished)
	}
	return map[string][]string{
		StageDraft:            draftTargets,
		StageProposed:         {StageDraft, StagePublished},
		StagePublished:        {StageDeletionProposed},
		StageDeletionProposed: {StagePublished},
	}
}

// TransitionAllowed reports whether from → to is permitted under opts.
func TransitionAllowed(opts Options, from, to string) bool {
	if from == to {
		return true
	}
	allowed, ok := AllowedTransitions(opts)[from]
	if !ok {
		return false
	}
	return slices.Contains(allowed, to)
}

// ValidateTransition returns an error if from → to is not permitted under opts.
func ValidateTransition(opts Options, from, to string) error {
	if TransitionAllowed(opts, from, to) {
		return nil
	}
	allowed := AllowedTransitions(opts)[from]
	allowedStr := make([]string, len(allowed))
	copy(allowedStr, allowed)
	if from == StageDraft && to == StagePublished && !opts.AllowDirectPublicationFromDraft {
		return fmt.Errorf("invalid transition %s → %s: direct publication from Draft is disabled (allowed: %s); propose first or enable allow-direct-publication-from-draft",
			from, to, strings.Join(allowedStr, ", "))
	}
	return fmt.Errorf("invalid transition %s → %s (allowed: %s)", from, to, strings.Join(allowedStr, ", "))
}
