// Copyright 2022, 2024-2025 The kpt and Nephio Authors
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

package task

import (
	"context"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/internal/kpt/builtins"
	"github.com/nephio-project/porch/internal/kpt/fnruntime"
	"github.com/nephio-project/porch/pkg/kpt/fn"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("task")

var (
	_ TaskHandler = &genericTaskHandler{}

	ConditionPipelineNotPassed = api.Condition{
		Type:    ConditionTypePipelinePassed,
		Status:  api.ConditionFalse,
		Reason:  "WaitingOnPipeline",
		Message: "waiting for package pipeline to pass",
	}
	ConditionPipelinePassed = api.Condition{
		Type:    ConditionTypePipelinePassed,
		Status:  api.ConditionTrue,
		Reason:  "PipelinePassed",
		Message: "package pipeline completed successfully",
	}
	DefaultReadinessConditions = []api.Condition{
		// ConditionPipelineNotPassed,
	}
)

const (
	ConditionTypePipelinePassed = "PackagePipelinePassed" // whether or not the package's pipeline has completed successfully
)

type TaskHandler interface {
	GetRuntime() fn.FunctionRuntime

	SetRunnerOptionsResolver(func(namespace string) fnruntime.RunnerOptions)
	SetRuntime(fn.FunctionRuntime)
	SetRepoOpener(repository.RepositoryOpener)
	SetCredentialResolver(repository.CredentialResolver)
	SetReferenceResolver(repository.ReferenceResolver)

	ApplyTasks(ctx context.Context, draft repository.PackageRevisionDraft, repositoryObj *configapi.Repository, obj *api.PackageRevision, packageConfig *builtins.PackageConfig) error
	DoPRMutations(ctx context.Context, namespace string, repoPR repository.PackageRevision, oldObj *api.PackageRevision, newObj *api.PackageRevision, draft repository.PackageRevisionDraft) error
	DoPRResourceMutations(ctx context.Context, pr2Update repository.PackageRevision, draft repository.PackageRevisionDraft, oldRes, newRes *api.PackageRevisionResources) (*api.RenderStatus, error)
}

type mutation interface {
	apply(ctx context.Context, resources repository.PackageResources) (repository.PackageResources, *api.TaskResult, error)
}

func GetDefaultTaskHandler() TaskHandler {
	return &genericTaskHandler{}
}
