// Copyright 2022, 2025 The kpt and Nephio Authors
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
	"fmt"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/internal/kpt/builtins"
	"github.com/nephio-project/porch/internal/kpt/fnruntime"
	kptfile "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/kpt/fn"
	"github.com/nephio-project/porch/pkg/repository"
	pkgerrors "github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
	"sigs.k8s.io/kustomize/kyaml/comments"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

var _ TaskHandler = &genericTaskHandler{}

type genericTaskHandler struct {
	runnerOptionsResolver func(namespace string) fnruntime.RunnerOptions
	runtime               fn.FunctionRuntime
	repoOpener            repository.RepositoryOpener
	credentialResolver    repository.CredentialResolver
	referenceResolver     repository.ReferenceResolver
	cloneStrategy         api.PackageMergeStrategy
}

func (th *genericTaskHandler) GetRuntime() fn.FunctionRuntime {
	return th.runtime
}

func (th *genericTaskHandler) SetRunnerOptionsResolver(runnerOptionsResolver func(namespace string) fnruntime.RunnerOptions) {
	th.runnerOptionsResolver = runnerOptionsResolver
}

func (th *genericTaskHandler) SetRuntime(runtime fn.FunctionRuntime) {
	th.runtime = runtime
}

func (th *genericTaskHandler) SetRepoOpener(repoOpener repository.RepositoryOpener) {
	th.repoOpener = repoOpener
}

func (th *genericTaskHandler) SetCredentialResolver(credentialResolver repository.CredentialResolver) {
	th.credentialResolver = credentialResolver
}

func (th *genericTaskHandler) SetReferenceResolver(referenceResolver repository.ReferenceResolver) {
	th.referenceResolver = referenceResolver
}

func (th *genericTaskHandler) ApplyTask(ctx context.Context, draft repository.PackageRevisionDraft, repositoryObj *configapi.Repository, obj *api.PackageRevision, packageConfig *builtins.PackageConfig) error {
	if len(obj.Spec.Tasks) != 1 {
		return pkgerrors.New("task list must contain exactly 1 task")
	}

	if cloneTask := obj.Spec.Tasks[0].Clone; cloneTask != nil {
		klog.Infof("Clone strategy is %s", cloneTask.Strategy)
		th.cloneStrategy = cloneTask.Strategy
	}

	mut, err := th.mapTaskToMutation(obj, &obj.Spec.Tasks[0], repositoryObj.Spec.Deployment, packageConfig)
	if err != nil {
		return err
	}

	resources, taskResult, err := mut.apply(ctx, repository.PackageResources{})
	if err != nil {
		return err
	}

	// Render package after creation.
	draftMeta := draft.GetMeta()
	resources, _, err = th.renderMutation(draftMeta.GetNamespace()).apply(ctx, resources)
	if err != nil {
		return err
	}

	prr := &api.PackageRevisionResources{
		Spec: api.PackageRevisionResourcesSpec{
			Resources: resources.Contents,
		},
	}

	return draft.UpdateResources(ctx, prr, taskResult.Task)
}

func (th *genericTaskHandler) DoPRMutations(
	ctx context.Context,
	repoPR repository.PackageRevision,
	oldObj, newObj *api.PackageRevision,
	draft repository.PackageRevisionDraft) error {
	ctx, span := tracer.Start(ctx, "genericTaskHandler::DoPRMutations", trace.WithAttributes())
	defer span.End()

	// Update package contents only if the package is in draft state
	if oldObj.Spec.Lifecycle == api.PackageRevisionLifecycleDraft {
		apiResources, err := repoPR.GetResources(ctx)
		if err != nil {
			return fmt.Errorf("cannot get package resources: %w", err)
		}
		resources := repository.PackageResources{
			Contents: apiResources.Spec.Resources,
		}

		{
			newKptfile, err := patchKptfile(ctx, repoPR, newObj)
			if err != nil {
				return err
			}
			marshalled, err := yaml.Marshal(newKptfile)
			if err != nil {
				return err
			}
			if str := string(marshalled); str != "{}\n" {
				resources.Contents[kptfile.KptFileName] = str
			}
		}

		// render
		draftMeta := draft.GetMeta()
		resources, _, err = th.renderMutation(draftMeta.GetNamespace()).apply(ctx, resources)
		if err != nil {
			klog.Error(err)
			return renderError(err)
		}

		prr := &api.PackageRevisionResources{
			Spec: api.PackageRevisionResourcesSpec{
				Resources: resources.Contents,
			},
		}

		return draft.UpdateResources(ctx, prr, &api.Task{Type: api.TaskTypeRender})
	}

	return nil
}

func (th *genericTaskHandler) DoPRResourceMutations(
	ctx context.Context,
	pr2Update repository.PackageRevision,
	draft repository.PackageRevisionDraft,
	oldRes, newRes *api.PackageRevisionResources) (*api.RenderStatus, error) {
	ctx, span := tracer.Start(ctx, "genericTaskHandler::DoPRResourceMutations", trace.WithAttributes())
	defer span.End()

	mut := &replaceResourcesMutation{
		newResources: newRes,
		oldResources: oldRes,
	}
	prevResources, err := pr2Update.GetResources(ctx)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "cannot get package resources")
	}
	resources := repository.PackageResources{
		Contents: prevResources.Spec.Resources,
	}

	appliedResources, _, err := mut.apply(ctx, resources)
	if err != nil {
		return nil, err
	}

	// Render the package
	// Render failure will fail the overall API operation.
	// The render error and result are captured as part of renderStatus above
	// and are returned in the PackageRevisionResources API's status field.
	// We do not push the package further to remote:
	// the user's changes are captured on their local package,
	// and can be amended using the error returned as a reference point to ensure
	// the package renders properly, before retrying the push.
	var (
		renderStatus *api.RenderStatus
		renderResult *api.TaskResult
	)
	appliedResources, renderResult, err = th.renderMutation(oldRes.GetNamespace()).apply(ctx, appliedResources)
	// keep last render result on empty patch
	if renderResult != nil &&
		renderResult.RenderStatus != nil &&
		(renderResult.RenderStatus.Err != "" ||
			len(renderResult.RenderStatus.Result.Items) != 0) {
		renderStatus = renderResult.RenderStatus
	}
	if err != nil {
		klog.Error(err)
		return renderStatus, renderError(err)
	}

	prr := &api.PackageRevisionResources{
		Spec: api.PackageRevisionResourcesSpec{
			Resources: appliedResources.Contents,
		},
	}

	return renderStatus, draft.UpdateResources(ctx, prr, &api.Task{Type: api.TaskTypeRender})
}

func (th *genericTaskHandler) renderMutation(namespace string) mutation {
	return &renderPackageMutation{
		runnerOptions: th.runnerOptionsResolver(namespace),
		runtime:       th.runtime,
	}
}

func renderError(err error) error {
	return pkgerrors.Wrap(err, `
Error occurred rendering package in kpt function pipeline.
Package has NOT been pushed to remote.
Please fix package locally (modify until 'kpt fn render' succeeds) and retry.
Details`)
}

func (th *genericTaskHandler) mapTaskToMutation(obj *api.PackageRevision, task *api.Task, isDeployment bool, packageConfig *builtins.PackageConfig) (mutation, error) {
	switch task.Type {
	case api.TaskTypeInit:
		if task.Init == nil {
			return nil, fmt.Errorf("init not set for task of type %q", task.Type)
		}
		return &initPackageMutation{
			name: obj.Spec.PackageName,
			task: task,
		}, nil
	case api.TaskTypeClone:
		if task.Clone == nil {
			return nil, fmt.Errorf("clone not set for task of type %q", task.Type)
		}
		return &clonePackageMutation{
			task:               task,
			namespace:          obj.Namespace,
			name:               obj.Spec.PackageName,
			isDeployment:       isDeployment,
			repoOpener:         th.repoOpener,
			credentialResolver: th.credentialResolver,
			referenceResolver:  th.referenceResolver,
			packageConfig:      packageConfig,
		}, nil

	case api.TaskTypeUpgrade:
		if task.Upgrade == nil {
			return nil, fmt.Errorf("upgrade field not set for task of type %q", task.Type)
		}
		return &upgradePackageMutation{
			upgradeTask:       task,
			namespace:         obj.Namespace,
			repoOpener:        th.repoOpener,
			referenceResolver: th.referenceResolver,
			pkgName:           obj.Spec.PackageName,
		}, nil

	case api.TaskTypeEdit:
		if task.Edit == nil {
			return nil, fmt.Errorf("edit not set for task of type %q", task.Type)
		}
		return &editPackageMutation{
			task:              task,
			namespace:         obj.Namespace,
			packageName:       obj.Spec.PackageName,
			repositoryName:    obj.Spec.RepositoryName,
			repoOpener:        th.repoOpener,
			referenceResolver: th.referenceResolver,
		}, nil

	default:
		return nil, fmt.Errorf("task of type %q not supported", task.Type)
	}
}

func patchKptfile(ctx context.Context, oldPackage repository.PackageRevision, newObj *api.PackageRevision) (*kptfile.KptFile, error) {
	var kf *kptfile.KptFile
	{
		k, err := oldPackage.GetKptfile(ctx)
		if err != nil {
			return nil, err
		}
		kf = &k
	}

	var readinessGates []kptfile.ReadinessGate
	for _, rg := range newObj.Spec.ReadinessGates {
		readinessGates = append(readinessGates, kptfile.ReadinessGate{
			ConditionType: rg.ConditionType,
		})
	}

	var conditions []kptfile.Condition
	for _, c := range newObj.Status.Conditions {
		conditions = append(conditions, kptfile.Condition{
			Type:    c.Type,
			Status:  convertStatusToKptfile(c.Status),
			Reason:  c.Reason,
			Message: c.Message,
		})
	}

	if kf.Info == nil && len(readinessGates) > 0 {
		kf.Info = &kptfile.PackageInfo{}
	}
	if len(readinessGates) > 0 {
		kf.Info.ReadinessGates = readinessGates
	}

	if kf.Status == nil && len(conditions) > 0 {
		kf.Status = &kptfile.Status{}
	}
	if len(conditions) > 0 {
		kf.Status.Conditions = conditions
	}

	return kf, nil
}

func convertStatusToKptfile(s api.ConditionStatus) kptfile.ConditionStatus {
	switch s {
	case api.ConditionTrue:
		return kptfile.ConditionTrue
	case api.ConditionFalse:
		return kptfile.ConditionFalse
	case api.ConditionUnknown:
		return kptfile.ConditionUnknown
	default:
		panic(fmt.Errorf("unknown condition status: %v", s))
	}
}

func healConfig(old, new map[string]string) (map[string]string, error) {
	// Copy comments from old config to new
	oldResources, err := (&packageReader{
		input: repository.PackageResources{Contents: old},
		extra: map[string]string{},
	}).Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read old packge resources: %w", err)
	}

	var filter kio.FilterFunc = func(r []*yaml.RNode) ([]*yaml.RNode, error) {
		for _, n := range r {
			for _, original := range oldResources {
				if n.GetNamespace() == original.GetNamespace() &&
					n.GetName() == original.GetName() &&
					n.GetApiVersion() == original.GetApiVersion() &&
					n.GetKind() == original.GetKind() {
					err = comments.CopyComments(original, n)
					if err != nil {
						return nil, fmt.Errorf("failed to copy comments: %w", err)
					}
				}
			}
		}
		return r, nil
	}

	out := &packageWriter{
		output: repository.PackageResources{
			Contents: map[string]string{},
		},
	}

	extra := map[string]string{}

	if err := (kio.Pipeline{
		Inputs: []kio.Reader{&packageReader{
			input: repository.PackageResources{Contents: new},
			extra: extra,
		}},
		Filters:               []kio.Filter{filter},
		Outputs:               []kio.Writer{out},
		ContinueOnEmptyResult: true,
	}).Execute(); err != nil {
		return nil, err
	}

	healed := out.output.Contents

	for k, v := range extra {
		healed[k] = v
	}

	return healed, nil
}
