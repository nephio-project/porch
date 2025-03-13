// Copyright 2022, 2024 The kpt and Nephio Authors
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
	"bytes"
	"context"
	"fmt"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/internal/kpt/builtins"
	"github.com/nephio-project/porch/internal/kpt/fnruntime"
	kptfile "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/kpt/fn"
	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func (th *genericTaskHandler) ApplyTasks(ctx context.Context, draft repository.PackageRevisionDraft, repositoryObj *configapi.Repository, obj *api.PackageRevision, packageConfig *builtins.PackageConfig) error {
	var mutations []mutation

	// Unless first task is Init or Clone, insert Init to create an empty package.
	tasks := obj.Spec.Tasks
	if len(tasks) == 0 || !taskTypeOneOf(tasks[0].Type, api.TaskTypeInit, api.TaskTypeClone, api.TaskTypeEdit) {
		mutations = append(mutations, &initPackageMutation{
			name: obj.Spec.PackageName,
			task: &api.Task{
				Init: &api.PackageInitTaskSpec{
					Subpackage:  "",
					Description: fmt.Sprintf("%s description", obj.Spec.PackageName),
				},
			},
		})
	}

	for i := range tasks {
		task := &tasks[i]
		mutation, err := th.mapTaskToMutation(ctx, obj, task, repositoryObj.Spec.Deployment, packageConfig)
		if err != nil {
			return err
		}
		mutations = append(mutations, mutation)
	}

	// Render package after creation.
	mutations = th.conditionalAddRender(obj, mutations)

	baseResources := repository.PackageResources{}
	if _, _, err := applyResourceMutations(ctx, draft, baseResources, mutations); err != nil {
		return err
	}

	return nil
}

func (th *genericTaskHandler) DoPRMutations(
	ctx context.Context,
	namespace string,
	repoPR repository.PackageRevision,
	oldObj *api.PackageRevision, newObj *api.PackageRevision,
	draft repository.PackageRevisionDraft,
) error {
	ctx, span := tracer.Start(ctx, "genericTaskHandler::DoPRMutations", trace.WithAttributes())
	defer span.End()

	var mutations []mutation
	if len(oldObj.Spec.Tasks) > len(newObj.Spec.Tasks) {
		return fmt.Errorf("removing tasks is not yet supported")
	}
	for i := range oldObj.Spec.Tasks {
		oldTask := &oldObj.Spec.Tasks[i]
		newTask := &newObj.Spec.Tasks[i]
		if oldTask.Type != newTask.Type {
			return fmt.Errorf("changing task types is not yet supported")
		}
	}
	if len(newObj.Spec.Tasks) > len(oldObj.Spec.Tasks) {
		if len(newObj.Spec.Tasks) > len(oldObj.Spec.Tasks)+1 {
			return fmt.Errorf("can only append one task at a time")
		}

		newTask := newObj.Spec.Tasks[len(newObj.Spec.Tasks)-1]
		if newTask.Type != api.TaskTypeUpdate {
			return fmt.Errorf("appended task is type %q, must be type %q", newTask.Type, api.TaskTypeUpdate)
		}
		if newTask.Update == nil {
			return fmt.Errorf("update not set for updateTask of type %q", newTask.Type)
		}

		cloneTask := findCloneTask(oldObj)
		if cloneTask == nil {
			return fmt.Errorf("upstream source not found for package rev %q; only cloned packages can be updated", oldObj.Spec.PackageName)
		}

		mutation := &updatePackageMutation{
			cloneTask:         cloneTask,
			updateTask:        &newTask,
			repoOpener:        th.repoOpener,
			referenceResolver: th.referenceResolver,
			namespace:         namespace,
			pkgName:           oldObj.GetName(),
		}
		mutations = append(mutations, mutation)
	}

	// Re-render if we are making changes.
	mutations = th.conditionalAddRender(newObj, mutations)

	// If any of the fields in the API that are projections from the Kptfile
	// must be updated in the Kptfile as well.
	kfPatchTask, created, err := createKptfilePatchTask(ctx, repoPR, newObj)
	if err != nil {
		return err
	}
	if created {
		kfPatchMutation, err := buildPatchMutation(ctx, kfPatchTask)
		if err != nil {
			return err
		}
		mutations = append(mutations, kfPatchMutation)
	}

	// Re-render if we are making changes.
	mutations = th.conditionalAddRender(newObj, mutations)

	// TODO: Handle the case if alongside lifecycle change, tasks are changed too.
	// Update package contents only if the package is in draft state
	if oldObj.Spec.Lifecycle == api.PackageRevisionLifecycleDraft {
		apiResources, err := repoPR.GetResources(ctx)
		if err != nil {
			return fmt.Errorf("cannot get package resources: %w", err)
		}
		resources := repository.PackageResources{
			Contents: apiResources.Spec.Resources,
		}

		if _, _, err := applyResourceMutations(ctx, draft, resources, mutations); err != nil {
			return err
		}
	}

	return nil
}

func taskTypeOneOf(taskType api.TaskType, oneOf ...api.TaskType) bool {
	for _, tt := range oneOf {
		if taskType == tt {
			return true
		}
	}
	return false
}

func (th *genericTaskHandler) DoPRResourceMutations(ctx context.Context, pr2Update repository.PackageRevision, draft repository.PackageRevisionDraft, oldRes, newRes *api.PackageRevisionResources) (*api.RenderStatus, error) {
	ctx, span := tracer.Start(ctx, "genericTaskHandler::DoPRResourceMutations", trace.WithAttributes())
	defer span.End()

	runnerOptions := th.runnerOptionsResolver(oldRes.GetNamespace())

	mutations := []mutation{
		&replaceResourcesMutation{
			newResources: newRes,
			oldResources: oldRes,
		},
	}
	prevResources, err := pr2Update.GetResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get package resources: %w", err)
	}
	resources := repository.PackageResources{
		Contents: prevResources.Spec.Resources,
	}

	appliedResources, _, err := applyResourceMutations(ctx, draft, resources, mutations)
	if err != nil {
		return nil, err
	}

	var renderStatus *api.RenderStatus
	if len(appliedResources.Contents) > 0 {
		// Render the package
		// Render failure will fail the overall API operation.
		// The render error and result are captured as part of renderStatus above
		// and are returned in the PackageRevisionResources API's status field.
		// We do not push the package further to remote:
		// the user's changes are captured on their local package,
		// and can be amended using the error returned as a reference point to ensure
		// the package renders properly, before retrying the push.
		_, renderStatus, err = applyResourceMutations(ctx,
			draft,
			appliedResources,
			[]mutation{&renderPackageMutation{
				runnerOptions: runnerOptions,
				runtime:       th.runtime,
			}})
		if err != nil {
			return renderStatus, err
		}
	} else {
		renderStatus = nil
	}

	return renderStatus, nil
}

func (th *genericTaskHandler) mapTaskToMutation(ctx context.Context, obj *api.PackageRevision, task *api.Task, isDeployment bool, packageConfig *builtins.PackageConfig) (mutation, error) {
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

	case api.TaskTypeUpdate:
		if task.Update == nil {
			return nil, fmt.Errorf("update not set for task of type %q", task.Type)
		}
		cloneTask := findCloneTask(obj)
		if cloneTask == nil {
			return nil, fmt.Errorf("upstream source not found for package rev %q; only cloned packages can be updated", obj.Spec.PackageName)
		}
		return &updatePackageMutation{
			cloneTask:         cloneTask,
			updateTask:        task,
			namespace:         obj.Namespace,
			repoOpener:        th.repoOpener,
			referenceResolver: th.referenceResolver,
			pkgName:           obj.Spec.PackageName,
		}, nil

	case api.TaskTypePatch:
		return buildPatchMutation(ctx, task)

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

	case api.TaskTypeEval:
		if task.Eval == nil {
			return nil, fmt.Errorf("eval not set for task of type %q", task.Type)
		}
		// TODO: We should find a different way to do this. Probably a separate
		// task for render.
		if task.Eval.Image == "render" {
			runnerOptions := th.runnerOptionsResolver(obj.Namespace)
			return &renderPackageMutation{
				runnerOptions: runnerOptions,
				runtime:       th.runtime,
			}, nil
		} else {
			runnerOptions := th.runnerOptionsResolver(obj.Namespace)
			return &evalFunctionMutation{
				runnerOptions: runnerOptions,
				runtime:       th.runtime,
				task:          task,
			}, nil
		}

	default:
		return nil, fmt.Errorf("task of type %q not supported", task.Type)
	}
}

func createKptfilePatchTask(ctx context.Context, oldPackage repository.PackageRevision, newObj *api.PackageRevision) (*api.Task, bool, error) {
	kf, err := oldPackage.GetKptfile(ctx)
	if err != nil {
		return nil, false, err
	}

	var orgKfString string
	{
		var buf bytes.Buffer
		d := yaml.NewEncoder(&buf)
		if err := d.Encode(kf); err != nil {
			return nil, false, err
		}
		orgKfString = buf.String()
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

	var newKfString string
	{
		var buf bytes.Buffer
		d := yaml.NewEncoder(&buf)
		if err := d.Encode(kf); err != nil {
			return nil, false, err
		}
		newKfString = buf.String()
	}
	patchSpec, err := GeneratePatch(kptfile.KptFileName, orgKfString, newKfString)
	if err != nil {
		return nil, false, err
	}
	// If patch is empty, don't create a Task.
	if patchSpec.Contents == "" {
		return nil, false, nil
	}

	return &api.Task{
		Type: api.TaskTypePatch,
		Patch: &api.PackagePatchTaskSpec{
			Patches: []api.PatchSpec{
				patchSpec,
			},
		},
	}, true, nil
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

// conditionalAddRender adds a render mutation to the end of the mutations slice if the last
// entry is not already a render mutation.
func (th *genericTaskHandler) conditionalAddRender(subject client.Object, mutations []mutation) []mutation {
	if len(mutations) == 0 || isRenderMutation(mutations[len(mutations)-1]) {
		return mutations
	}

	runnerOptions := th.runnerOptionsResolver(subject.GetNamespace())

	return append(mutations, &renderPackageMutation{
		runnerOptions: runnerOptions,
		runtime:       th.runtime,
	})
}

func isRenderMutation(m mutation) bool {
	if m == nil {
		return false
	}
	_, isRender := m.(*renderPackageMutation)
	return isRender
}

// applyResourceMutations mutates the resources and returns the most recent renderResult.
func applyResourceMutations(
	ctx context.Context,
	draft repository.PackageRevisionDraft,
	baseResources repository.PackageResources,
	mutations []mutation,
) (
	applied repository.PackageResources,
	renderStatus *api.RenderStatus,
	err error,
) {
	ctx, span := tracer.Start(ctx, "genericTaskHandler::applyResourceMutations", trace.WithAttributes())
	defer span.End()

	var lastApplied mutation
	for _, m := range mutations {
		updatedResources, taskResult, err := m.apply(ctx, baseResources)
		if err != nil {
			klog.Error(err)
			return updatedResources, renderStatus, err
		}
		if taskResult == nil {
			// a nil taskResult means nothing changed
			continue
		}

		task := taskResult.Task
		if task.Type == api.TaskTypeEval {
			// NOTE: a failed render doesn't return with and error (anymore),
			// so the results of render failures will be correctly returned, as well
			renderStatus = taskResult.RenderStatus
		}

		// if the last applied mutation was a render mutation, and so is this one, skip it
		if lastApplied != nil && isRenderMutation(m) && isRenderMutation(lastApplied) {
			continue
		}
		lastApplied = m

		if err := draft.UpdateResources(ctx, &api.PackageRevisionResources{
			Spec: api.PackageRevisionResourcesSpec{
				Resources: updatedResources.Contents,
			},
		}, task); err != nil {
			return updatedResources, renderStatus, err
		}
		baseResources = updatedResources
		applied = updatedResources
	}

	return applied, renderStatus, nil
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
