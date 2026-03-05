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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=functionconfigs,singular=functionconfig
// kubebuilder:printcolumn:name="Applied",type=integer,JSONPath=`.status.applied`
// +kubebuilder:printcolumn:name="Server Applied",type=integer,JSONPath=`.status.apiServerObservedGeneration`
// +kubebuilder:printcolumn:name="FnRunner Applied",type=integer,JSONPath=`.status.functionRunnerObservedGeneration`
type FunctionConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FunctionConfigSpec   `json:"spec,omitempty"`
	Status FunctionConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type FunctionConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []FunctionConfig `json:"items"`
}

// +kubebuilder:validation:XValidation:message="At least one configuration must be specified",rule="has(self.podExecutorConfig) || has(self.binaryExecutorConfig) || has(self.goExecutorConfig)"
type FunctionConfigSpec struct {
	// +kubebuilder:validation:MinLength=1
	Image                string                `json:"image"`
	Prefixes             []string              `json:"prefixes,omitempty"`
	PodExecutorConfig    *PodExecutorConfig    `json:"podExecutorConfig,omitempty"`
	BinaryExecutorConfig *BinaryExecutorConfig `json:"binaryExecutorConfig,omitempty"`
	GoExecutorConfig     *GoExecutorConfig     `json:"goExecutorConfig,omitempty"`
}

type FunctionConfigStatus struct {
	// ObservedGeneration indicates which resource version of the function configuration has been read by Porch
	// ObservedGeneration int64 `json:"observedGeneration"`

	// ApiServerObservedGeneration indicates which generation of the config the porch server has applied to the build-in runtime
	ApiServerObservedGeneration int64 `json:"apiServerObservedGeneration,omitempty"`
	// FunctionRunnerObservedGeneration indicates which generation of the config the function-runner has applied to the executable and pod evaluator
	FunctionRunnerObservedGeneration int64 `json:"functionRunnerObservedGeneration,omitempty"`
	// Contains an error message if one occurred whilst trying to apply the FunctionConfig
	Error string `json:"error,omitempty"`
}

type PodExecutorConfig struct {
	// Image tags which the pod executor configuration will be applied to.
	// If tags is empty, the configuration will apply to all pods created for the image. TODO: this is not implemented
	Tags []string `json:"tags,omitempty"`
	// +kubebuilder:default="30m"
	// +kubebuilder:validation:Format=duration
	TimeToLive              metav1.Duration `json:"timeToLive,omitempty"`
	MaxParallelExecutions   int             `json:"maxParallelExecutions,omitempty"`
	PreferredMaxQueueLength int             `json:"preferredMaxQueueLength,omitempty"`

	TemplateOverrides *TemplateOverrides `json:"templateOverrides,omitempty"`
	// TODO: add warmup section
}

type TemplateOverrides struct {
	ServiceAccountName string                     `json:"serviceAccountName,omitempty"`
	SecurityContext    *corev1.PodSecurityContext `json:"securityContext,omitempty"`
	InitContainer      *ContainerOverrides        `json:"initContainer,omitempty"`
	Container          *ContainerOverrides        `json:"container,omitempty"`
}

type ContainerOverrides struct {
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	EnvFrom   []corev1.EnvFromSource       `json:"envFrom,omitempty"`
	Env       []corev1.EnvVar              `json:"env,omitempty"`
}

// +kubebuilder:validation:XValidation:message="Exactly one of relPath or absPath must be specified",rule="has(self.relPath) != has(self.absPath)"
type BinaryExecutorConfig struct {
	// Image tags which can be substituted with the specified KRM function binary.
	// +kubebuilder:validation:MinItems=1
	Tags []string `json:"tags"`
	// RelPath defines the file path of the binary relative to the default `functions` directory
	RelPath string `json:"relPath,omitempty"`
	// AbsPath defines the absolute file path of the binary within the function runner container
	AbsPath string `json:"absPath,omitempty"`
}

type GoExecutorConfig struct {
	// Image tags which can be substituted with a go function call.
	// +kubebuilder:validation:MinItems=1
	Tags []string `json:"tags"`
	// ID defines how the function is registered in the internal go executor of Porch.
	// If empty, `.spec.image` will be used instead.
	ID *string `json:"id,omitempty"`
}
