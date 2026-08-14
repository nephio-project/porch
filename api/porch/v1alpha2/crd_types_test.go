/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// copied from k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1@v0.36.3 with modifications
// so that we don't have to import it

package v1alpha2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type ConversionStrategyType string

type CustomResourceDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CustomResourceDefinitionSpec   `json:"spec"`
	Status CustomResourceDefinitionStatus `json:"status,omitempty"`
}

type CustomResourceDefinitionSpec struct {
	Group    string                            `json:"group"`
	Names    CustomResourceDefinitionNames     `json:"names"`
	Scope    ResourceScope                     `json:"scope"`
	Versions []CustomResourceDefinitionVersion `json:"versions"`

	Conversion *CustomResourceConversion `json:"conversion,omitempty"`

	PreserveUnknownFields bool `json:"preserveUnknownFields,omitempty"`
}

type CustomResourceConversion struct {
	Strategy ConversionStrategyType `json:"strategy"`

	Webhook *WebhookConversion `json:"webhook,omitempty"`
}

type WebhookConversion struct {
	ClientConfig *WebhookClientConfig `json:"clientConfig,omitempty"`

	ConversionReviewVersions []string `json:"conversionReviewVersions"`
}

type WebhookClientConfig struct {
	URL     *string           `json:"url,omitempty"`
	Service *ServiceReference `json:"service,omitempty"`

	CABundle []byte `json:"caBundle,omitempty"`
}

type ServiceReference struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`

	Path *string `json:"path,omitempty"`

	Port *int32 `json:"port,omitempty"`
}

type CustomResourceDefinitionVersion struct {
	Name                     string                           `json:"name"`
	Served                   bool                             `json:"served"`
	Storage                  bool                             `json:"storage"`
	Deprecated               bool                             `json:"deprecated,omitempty"`
	DeprecationWarning       *string                          `json:"deprecationWarning,omitempty"`
	Schema                   *CustomResourceValidation        `json:"schema,omitempty"`
	Subresources             *CustomResourceSubresources      `json:"subresources,omitempty"`
	AdditionalPrinterColumns []CustomResourceColumnDefinition `json:"additionalPrinterColumns,omitempty"`

	//
	SelectableFields []SelectableField `json:"selectableFields,omitempty"`
}

type SelectableField struct {
	JSONPath string `json:"jsonPath"`
}

type CustomResourceColumnDefinition struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Format      string `json:"format,omitempty"`
	Description string `json:"description,omitempty"`
	Priority    int32  `json:"priority,omitempty"`
	JSONPath    string `json:"jsonPath"`
}

type CustomResourceDefinitionNames struct {
	Plural     string   `json:"plural"`
	Singular   string   `json:"singular,omitempty"`
	ShortNames []string `json:"shortNames,omitempty"`
	Kind       string   `json:"kind"`
	ListKind   string   `json:"listKind,omitempty"`
	Categories []string `json:"categories,omitempty"`
}

type ResourceScope string

type ConditionStatus string

type CustomResourceDefinitionConditionType string

type CustomResourceDefinitionCondition struct {
	Type               CustomResourceDefinitionConditionType `json:"type"`
	Status             ConditionStatus                       `json:"status"`
	LastTransitionTime metav1.Time                           `json:"lastTransitionTime,omitempty"`
	Reason             string                                `json:"reason,omitempty"`
	Message            string                                `json:"message,omitempty"`
	ObservedGeneration int64                                 `json:"observedGeneration,omitempty"`
}

type CustomResourceDefinitionStatus struct {
	Conditions         []CustomResourceDefinitionCondition `json:"conditions"`
	AcceptedNames      CustomResourceDefinitionNames       `json:"acceptedNames"`
	StoredVersions     []string                            `json:"storedVersions"`
	ObservedGeneration int64                               `json:"observedGeneration,omitempty"`
}

type CustomResourceValidation struct {
	OpenAPIV3Schema *JSONSchemaProps `json:"openAPIV3Schema,omitempty"`
}

type JSONSchemaProps struct{}

type CustomResourceSubresources struct {
	Status *CustomResourceSubresourceStatus `json:"status,omitempty"`
	Scale  *CustomResourceSubresourceScale  `json:"scale,omitempty"`
}

type CustomResourceSubresourceStatus struct{}

type CustomResourceSubresourceScale struct {
	SpecReplicasPath   string  `json:"specReplicasPath"`
	StatusReplicasPath string  `json:"statusReplicasPath"`
	LabelSelectorPath  *string `json:"labelSelectorPath,omitempty"`
}
