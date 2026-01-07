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

package v1alpha1

import (
	"context"
	"errors"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

//+kubebuilder:webhook:path=/validate-config-porch-kpt-dev-v1alpha1-repository,mutating=false,failurePolicy=fail,sideEffects=None,groups=config.porch.kpt.dev,resources=repositories,verbs=create;update,versions=v1alpha1,name=vrepository.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/mutate-config-porch-kpt-dev-v1alpha1-repository,mutating=true,failurePolicy=fail,sideEffects=None,groups=config.porch.kpt.dev,resources=repositories,verbs=create;update,versions=v1alpha1,name=mrepository.kb.io,admissionReviewVersions=v1

//+kubebuilder:object:generate=false

// RepositoryWebhook handles Repository admission control
type RepositoryWebhook struct {
	Client client.Client
}

var _ admission.CustomDefaulter = &RepositoryWebhook{}
var _ admission.CustomValidator = &RepositoryWebhook{}

// Default implements admission.CustomDefaulter
func (r *RepositoryWebhook) Default(ctx context.Context, obj runtime.Object) error {
	repo := obj.(*Repository)
	
	// Set default branch for Git repositories
	if repo.Spec.Git != nil && repo.Spec.Git.Branch == "" {
		repo.Spec.Git.Branch = "main"
	}
	
	return nil
}

// ValidateCreate implements admission.CustomValidator
func (r *RepositoryWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	repo := obj.(*Repository)
	return nil, r.validate(ctx, repo)
}

// ValidateUpdate implements admission.CustomValidator
func (r *RepositoryWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newRepo := newObj.(*Repository)
	oldRepo := oldObj.(*Repository)
	
	// Always immutable
	if oldRepo.Spec.Type != newRepo.Spec.Type {
		return nil, errors.New("spec.type is immutable")
	}
	
	return nil, r.validate(ctx, newRepo)
}

// ValidateDelete implements admission.CustomValidator
func (r *RepositoryWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	// No validation needed for delete
	return nil, nil
}

// validate performs common validation logic
func (r *RepositoryWebhook) validate(ctx context.Context, repo *Repository) error {
	// Basic structure validation
	if repo.Spec.Type == "" {
		return errors.New("spec.type is required")
	}
	
	// Git-specific validation
	if repo.Spec.Type == RepositoryTypeGit {
		if repo.Spec.Git == nil {
			return errors.New("spec.git is required when type is git")
		}
		if repo.Spec.Git.Repo == "" {
			return errors.New("spec.git.repo is required")
		}
	}
	
	// OCI-specific validation
	if repo.Spec.Type == RepositoryTypeOCI {
		if repo.Spec.Oci == nil {
			return errors.New("spec.oci is required when type is oci")
		}
		if repo.Spec.Oci.Registry == "" {
			return errors.New("spec.oci.registry is required")
		}
	}
	
	// TODO: Add directory conflict detection when PR #335 merges
	
	return nil
}

// SetupWebhookWithManager registers the webhook with the manager
func (r *RepositoryWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	r.Client = mgr.GetClient()
	
	return ctrl.NewWebhookManagedBy(mgr).
		For(&Repository{}).
		WithDefaulter(r).
		WithValidator(r).
		Complete()
}