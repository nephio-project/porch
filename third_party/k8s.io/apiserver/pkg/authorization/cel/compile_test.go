/*
Copyright 2023 The Kubernetes Authors.

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

package cel

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	v1 "k8s.io/api/authorization/v1"
	apiservercel "k8s.io/apiserver/pkg/cel"
	"k8s.io/apiserver/pkg/cel/environment"
)

func TestCompileCELExpression(t *testing.T) {
	cases := []struct {
		name          string
		expression    string
		expectedError string
	}{
		{
			name:       "SubjectAccessReviewSpec user comparison",
			expression: "request.user == 'bob'",
		},
		{
			name:          "undefined fields",
			expression:    "request.time == 'now'",
			expectedError: "undefined field",
		},
		{
			name:          "Syntax errors",
			expression:    "request++'",
			expectedError: "Syntax error",
		},
		{
			name:          "bad return type",
			expression:    "request.user",
			expectedError: "must evaluate to bool",
		},
		{
			name:          "undeclared reference",
			expression:    "x.user",
			expectedError: "undeclared reference",
		},
	}

	compiler := NewCompiler(environment.MustBaseEnvSet(environment.DefaultCompatibilityVersion(), true))

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := compiler.CompileCELExpression(&SubjectAccessReviewMatchCondition{
				Expression: tc.expression,
			})
			if len(tc.expectedError) > 0 && (err == nil || !strings.Contains(err.Error(), tc.expectedError)) {
				t.Fatalf("expected error: %s compiling expression %s, got: %v", tc.expectedError, tc.expression, err)
			}
			if len(tc.expectedError) == 0 && err != nil {
				t.Fatalf("unexpected error %v compiling expression %s", err, tc.expression)
			}
		})
	}
}

func TestBuildRequestType(t *testing.T) {
	f := func(name string, declType *apiservercel.DeclType, required bool) *apiservercel.DeclField {
		return apiservercel.NewDeclField(name, declType, required, nil, nil)
	}
	fs := func(fields ...*apiservercel.DeclField) map[string]*apiservercel.DeclField {
		result := make(map[string]*apiservercel.DeclField, len(fields))
		for _, f := range fields {
			result[f.Name] = f
		}
		return result
	}

	requestDeclType := buildRequestType(f, fs)
	requestType := reflect.TypeOf(v1.SubjectAccessReviewSpec{})
	if len(requestDeclType.Fields) != requestType.NumField() {
		t.Fatalf("expected %d fields for SubjectAccessReviewSpec, got %d", requestType.NumField(), len(requestDeclType.Fields))
	}
	resourceAttributesDeclType := buildResourceAttributesType(f, fs)
	resourceAttributeType := reflect.TypeOf(v1.ResourceAttributes{})
	if len(resourceAttributesDeclType.Fields) != resourceAttributeType.NumField() {
		t.Fatalf("expected %d fields for ResourceAttributes, got %d", resourceAttributeType.NumField(), len(resourceAttributesDeclType.Fields))
	}
	nonResourceAttributesDeclType := buildNonResourceAttributesType(f, fs)
	nonResourceAttributeType := reflect.TypeOf(v1.NonResourceAttributes{})
	if len(nonResourceAttributesDeclType.Fields) != nonResourceAttributeType.NumField() {
		t.Fatalf("expected %d fields for NonResourceAttributes, got %d", nonResourceAttributeType.NumField(), len(nonResourceAttributesDeclType.Fields))
	}
	if err := compareFieldsForType(t, requestType, requestDeclType, f, fs); err != nil {
		t.Error(err)
	}
}

func compareFieldsForType(t *testing.T, nativeType reflect.Type, declType *apiservercel.DeclType, field func(name string, declType *apiservercel.DeclType, required bool) *apiservercel.DeclField, fields func(fields ...*apiservercel.DeclField) map[string]*apiservercel.DeclField) error {
	for i := 0; i < nativeType.NumField(); i++ {
		nativeField := nativeType.Field(i)
		jsonTagParts := strings.Split(nativeField.Tag.Get("json"), ",")
		if len(jsonTagParts) < 1 {
			t.Fatal("expected json tag to be present")
		}
		fieldName := jsonTagParts[0]

		declField, ok := declType.Fields[fieldName]
		if !ok {
			t.Fatalf("expected field %q to be present", nativeField.Name)
		}
		declFieldType := nativeTypeToCELType(t, nativeField.Type, field, fields)
		if declFieldType != nil && declFieldType.CelType().Equal(declField.Type.CelType()).Value() != true {
			return fmt.Errorf("expected native field %q to have type %v, got %v", nativeField.Name, nativeField.Type, declField.Type)
		}
	}
	return nil
}

func nativeTypeToCELType(t *testing.T, nativeType reflect.Type, field func(name string, declType *apiservercel.DeclType, required bool) *apiservercel.DeclField, fields func(fields ...*apiservercel.DeclField) map[string]*apiservercel.DeclField) *apiservercel.DeclType {
	switch nativeType {
	case reflect.TypeOf(""):
		return apiservercel.StringType
	case reflect.TypeOf([]string{}):
		return apiservercel.NewListType(apiservercel.StringType, -1)
	case reflect.TypeOf(map[string]v1.ExtraValue{}):
		return apiservercel.NewMapType(apiservercel.StringType, apiservercel.NewListType(apiservercel.StringType, -1), -1)
	case reflect.TypeOf(&v1.ResourceAttributes{}):
		resourceAttributesDeclType := buildResourceAttributesType(field, fields)
		if err := compareFieldsForType(t, reflect.TypeOf(v1.ResourceAttributes{}), resourceAttributesDeclType, field, fields); err != nil {
			t.Error(err)
			return nil
		}
		return resourceAttributesDeclType
	case reflect.TypeOf(&v1.NonResourceAttributes{}):
		nonResourceAttributesDeclType := buildNonResourceAttributesType(field, fields)
		if err := compareFieldsForType(t, reflect.TypeOf(v1.NonResourceAttributes{}), nonResourceAttributesDeclType, field, fields); err != nil {
			t.Error(err)
			return nil
		}
		return nonResourceAttributesDeclType
	default:
		t.Fatalf("unsupported type %v", nativeType)
	}
	return nil
}
