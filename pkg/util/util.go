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
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/nephio-project/porch/api/porch/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	registrationapi "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	invalidConst string = " invalid:"
	uuidSpace    string = "aac71d91-5c67-456f-8fd2-902ef6da820e"
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
	apiSvcName := v1alpha1.SchemeGroupVersion.Version + "." + v1alpha1.SchemeGroupVersion.Group
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

func ValidateDirectoryName(directory string, mandatory bool) error {
	// A directory must follow the rules for RFC 1123 DNS labels except that we allow '/' characters
	var dirErrs []string
	if strings.Contains(directory, "//") {
		dirErrs = append(dirErrs, "consecutive '/' characters are not allowed")
	}
	dirNoSlash := strings.ReplaceAll(directory, "/", "")
	if mandatory || len(dirNoSlash) > 0 {
		dirErrs = append(dirErrs, validation.IsDNS1123Label(dirNoSlash)...)
	} else {
		// The directory is "/"
		dirErrs = nil
	}

	if dirErrs == nil {
		return nil
	} else {
		return errors.New(strings.Join(dirErrs, ","))
	}
}

func ValidateRepository(repoName, directory string) error {
	// The repo name must follow the rules for RFC 1123 DNS labels
	nameErrs := validation.IsDNS1123Label(repoName)

	dirErr := ValidateDirectoryName(directory, false)

	if nameErrs == nil && dirErr == nil {
		return nil
	}

	repoErrString := ""

	if nameErrs != nil {
		repoErrString = "repository name " + repoName + invalidConst + strings.Join(nameErrs, ",") + "\n"
	}

	dirErrString := ""
	if dirErr != nil {
		dirErrString = "directory name " + directory + invalidConst + dirErr.Error() + "\n"
	}

	return errors.New(repoErrString + dirErrString)
}

func ComposePkgObjName(repoName, path, packageName string) string {
	dottedPath := strings.ReplaceAll(filepath.Join(path, packageName), "/", ".")
	dottedPath = strings.Trim(dottedPath, ".")
	return fmt.Sprintf("%s.%s", repoName, dottedPath)
}

func ComposePkgRevObjName(repoName, path, packageName, workspace string) string {
	dottedPath := strings.ReplaceAll(filepath.Join(path, packageName), "/", ".")
	dottedPath = strings.Trim(dottedPath, ".")
	return fmt.Sprintf("%s.%s.%s", repoName, dottedPath, workspace)
}

func ValidPkgObjName(repoName, path, packageName string) error {
	var errSlice []string

	if err := ValidateRepository(repoName, path); err != nil {
		errSlice = append(errSlice, err.Error())
	}

	if err := ValidateDirectoryName(packageName, true); err != nil {
		errSlice = append(errSlice, "package name "+packageName+invalidConst+err.Error()+"\n")
	}

	if len(errSlice) == 0 {
		return nil
	} else {
		return errors.New("package object name invalid:\n" + strings.Join(errSlice, ""))
	}
}

func ValidPkgRevObjName(repoName, path, packageName, workspace string) error {
	var errSlice []string

	if err := ValidateRepository(repoName, path); err != nil {
		errSlice = append(errSlice, err.Error())
	}

	if err := ValidateDirectoryName(packageName, true); err != nil {
		errSlice = append(errSlice, "package name "+packageName+invalidConst+err.Error()+"\n")
	}

	if err := ValidateK8SName(string(workspace)); err != nil {
		errSlice = append(errSlice, "workspace name "+workspace+invalidConst+err.Error())
	}

	if len(errSlice) == 0 {
		objName := ComposePkgRevObjName(repoName, path, packageName, workspace)

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

func GenerateUid(prefix string, kubeNs string, kubeName string) types.UID {
	space := uuid.MustParse(uuidSpace)
	buff := bytes.Buffer{}
	buff.WriteString(prefix)
	buff.WriteString(strings.ToLower(v1alpha1.SchemeGroupVersion.Identifier()))
	buff.WriteString("/")
	buff.WriteString(strings.ToLower(kubeNs))
	buff.WriteString("/")
	buff.WriteString(strings.ToLower(kubeName))
	return types.UID(uuid.NewSHA1(space, buff.Bytes()).String())
}

func SafeReverse[S ~[]E, E any](s S) {
	if s == nil {
		return
	}
	slices.Reverse(s)
}

func CompareObjectMeta(left metav1.ObjectMeta, right metav1.ObjectMeta) bool {
	if result := strings.Compare(left.Name, right.Name); result != 0 {
		return false
	}

	if result := strings.Compare(left.Namespace, right.Namespace); result != 0 {
		return false
	}

	if result := reflect.DeepEqual(left.Labels, right.Labels); !result {
		return false
	}

	if result := reflect.DeepEqual(left.Annotations, right.Annotations); !result {
		return false
	}

	if result := reflect.DeepEqual(left.Finalizers, right.Finalizers); !result {
		return false
	}

	if result := reflect.DeepEqual(left.OwnerReferences, right.OwnerReferences); !result {
		return false
	}

	return true
}

// RetryOnErrorConditional retries f up to retries times if it returns an error that matches shouldRetryFunc
func RetryOnErrorConditional(retries int, shouldRetryFunc func(error) bool, f func(retryNumber int) error) error {
	var err error
	for i := 0; i < retries; i++ {
		err = f(i)
		if err == nil || !shouldRetryFunc(err) {
			return err
		}
	}
	return err
}
