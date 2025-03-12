// Copyright 2023-2025 The kpt and Nephio Authors
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

package util

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"path/filepath"
	"strings"

	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	registrationapi "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	invalidConst string = " invalid:"
)

func GetInClusterNamespace() (string, error) {
	ns, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "", fmt.Errorf("failed to read in-cluster namespace: %w", err)
	}
	return string(ns), nil
}

func GetPorchApiServiceKey(ctx context.Context) (client.ObjectKey, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return client.ObjectKey{}, fmt.Errorf("failed to get K8s config: %w", err)
	}

	scheme := runtime.NewScheme()
	err = registrationapi.AddToScheme(scheme)
	if err != nil {
		return client.ObjectKey{}, fmt.Errorf("failed to add apiregistration API to scheme: %w", err)
	}

	c, err := client.New(cfg, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return client.ObjectKey{}, fmt.Errorf("failed to create K8s client: %w", err)
	}

	apiSvc := registrationapi.APIService{}
	apiSvcName := porchapi.SchemeGroupVersion.Version + "." + porchapi.SchemeGroupVersion.Group
	err = c.Get(ctx, client.ObjectKey{
		Name: apiSvcName,
	}, &apiSvc)
	if err != nil {
		return client.ObjectKey{}, fmt.Errorf("failed to get APIService %q: %w", apiSvcName, err)
	}

	return client.ObjectKey{
		Namespace: apiSvc.Spec.Service.Namespace,
		Name:      apiSvc.Spec.Service.Name,
	}, nil
}

func SchemaToMetaGVR(gvr schema.GroupVersionResource) metav1.GroupVersionResource {
	return metav1.GroupVersionResource{
		Group:    gvr.Group,
		Version:  gvr.Version,
		Resource: gvr.Resource,
	}
}

func ValidateK8SName(k8sName string) error {
	if k8sNameErrs := validation.IsDNS1123Label(k8sName); k8sNameErrs != nil {
		return errors.New(strings.Join(k8sNameErrs, ","))
	}

	return nil
}

func ValidateRepository(repoName, directory string) error {
	// The repo name must follow the rules for RFC 1123 DNS labels
	nameErrs := validation.IsDNS1123Label(repoName)

	// The repo name must follow the rules for RFC 1123 DNS labels except that we allow '/' characters
	var dirErrs []string
	if strings.Contains(directory, "//") {
		dirErrs = append(dirErrs, "consecutive '/' characters are not allowed")
	}
	dirNoSlash := strings.ReplaceAll(directory, "/", "")
	if len(dirNoSlash) > 0 {
		dirErrs = append(dirErrs, validation.IsDNS1123Label(dirNoSlash)...)
	} else {
		// The directory is "/"
		dirErrs = nil
	}

	if nameErrs == nil && dirErrs == nil {
		return nil
	}

	repoErrString := ""

	if nameErrs != nil {
		repoErrString = "repository name " + repoName + invalidConst + strings.Join(nameErrs, ",") + "\n"
	}

	dirErrString := ""
	if dirErrs != nil {
		dirErrString = "directory name " + directory + invalidConst + strings.Join(dirErrs, ",") + "\n"
	}

	return errors.New(repoErrString + dirErrString)
}

func ComposePkgRevObjName(repoName, directory, packageName, workspace string) string {
	dottedPath := strings.ReplaceAll(filepath.Join(directory, packageName), "/", ".")
	dottedPath = strings.Trim(dottedPath, ".")
	return fmt.Sprintf("%s.%s.%s", repoName, dottedPath, workspace)
}

func ValidPkgRevObjName(repoName, directory, packageName, workspace string) error {
	var errSlice []string

	if err := ValidateRepository(repoName, directory); err != nil {
		errSlice = append(errSlice, err.Error())
	}

	if err := ValidateK8SName(packageName); err != nil {
		errSlice = append(errSlice, "package name "+packageName+invalidConst+err.Error()+"\n")
	}

	if err := ValidateK8SName(string(workspace)); err != nil {
		errSlice = append(errSlice, "workspace name "+workspace+invalidConst+err.Error())
	}

	if len(errSlice) == 0 {
		objName := ComposePkgRevObjName(repoName, directory, packageName, workspace)

		if objNameErrs := validation.IsDNS1123Subdomain(objName); objNameErrs != nil {
			errSlice = append(errSlice, "complete object name "+objName+invalidConst+strings.Join(objNameErrs, "")+"\n")
		}
	}

	if len(errSlice) == 0 {
		return nil
	} else {
		return errors.New("package revision object name invalid:\n" + strings.Join(errSlice, ""))
	}
}

func ParsePkgRevObjName(name string) ([]string, error) {
	const twoDotErrMsg = "malformed package revision name; expected at least two dots: %q"

	firstDot := strings.Index(name, ".")
	if firstDot < 0 {
		return nil, fmt.Errorf(twoDotErrMsg, name)
	}

	lastDot := strings.LastIndex(name, ".")
	if lastDot < 0 {
		return nil, fmt.Errorf(twoDotErrMsg, name)
	}

	if firstDot >= lastDot {
		return nil, fmt.Errorf(twoDotErrMsg, name)
	}

	parsedName := make([]string, 3)

	parsedName[0] = name[:firstDot]
	parsedName[1] = name[firstDot+1 : lastDot]
	parsedName[2] = name[lastDot+1:]

	return parsedName, nil
}

func ParsePkgRevObjNameField(pkgRevObjName string, field int) (string, error) {
	if parsedSlice, err := ParsePkgRevObjName(pkgRevObjName); err == nil {
		return parsedSlice[field], nil
	} else {
		return "", err
	}
}

// Parses valid YAML into a KubeObject using the kpt-functions-sdk's fn package.
// Wrapped in this function to unify deserialisation into a single approach
// everywhere in Porch.
func YamlToKubeObject(yaml string) (kubeObject *fn.KubeObject, err error) {
	if kubeObject, err = fn.ParseKubeObject([]byte(yaml)); err != nil {
		return nil, err
	}

	return kubeObject, nil
}

// Writes a KubeObject out to string-form YAML.
// Wrapped in this function to unify serialisation into a single approach
// everywhere in Porch.
func KubeObjectToYaml(kubeObject *fn.KubeObject) string {
	return kubeObject.String()
}

// Merge combines two slices of the same type (aSlice and bSlice) and
// returns a new slice containing all elements of aSlice, plus all
// elements of bSlice which do not exist in aSlice (where "do not
// exist in aSlice" means "for a bSlice element b, aSlice does not
// already contain an element for which reflect.DeepEqual(a, b)
// returns true")
func Merge[E any](aSlice, bSlice []E) []E {
	return MergeFunc(aSlice, bSlice,
		func(a, b E) bool { return reflect.DeepEqual(a, b) })
}

// Merge combines two slices of the same type (aSlice and bSlice) and
// returns a new slice containing all elements of aSlice, plus all
// elements of bSlice which do not exist in aSlice. "do not exist in
// aSlice" is determined by custom equality function existsFn, which
// must compare two objects (which MergeFunc will feed in, one from
// aSlice and one from bSlice) and return true if they can be considered
// equal based on custom criteria; false if not.
func MergeFunc[E any](aSlice, bSlice []E, existsFn func(a, b E) bool) []E {
	return func() (uniqueElements []E) {
		for _, each := range slices.Backward(slices.Concat(aSlice, bSlice)) {
			if !slices.ContainsFunc(uniqueElements, func(eachUnique E) bool {
				return existsFn(eachUnique, each)
			}) {
				uniqueElements = append(uniqueElements, each)
			}
		}
		slices.Reverse(uniqueElements)
		return
	}()
}
