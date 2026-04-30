// Copyright 2023 The kpt and Nephio Authors
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
	"fmt"
	"strings"

	kptfilev1 "github.com/kptdev/kpt/pkg/api/kptfile/v1"
	"github.com/kptdev/kpt/pkg/lib/errors"
	fnsdk "github.com/kptdev/krm-functions-sdk/go/fn"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	cliutils "github.com/nephio-project/porch/internal/cliutils"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ResourceVersionAnnotation = "internal.kpt.dev/resource-version"
)

// InitClient creates a controller-runtime client and validates the namespace flag.
// If --namespace is specified without a value, it returns an error immediately.
func InitClient(cmd *cobra.Command, cfg *genericclioptions.ConfigFlags) (client.Client, error) {
	nsFlag := cmd.Flag("namespace")
	nFlag := cmd.Flag("n")
	if (nsFlag != nil && nsFlag.Changed && nsFlag.Value.String() == "") ||
		(nFlag != nil && nFlag.Changed && nFlag.Value.String() == "") {
		return nil, fmt.Errorf("namespace flag specified without a value; please provide a value for --namespace/-n or omit the flag")
	}

	client, err := cliutils.CreateClientWithFlags(cfg)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// CreateScheme returns a runtime.Scheme registered with the type sets that
// rpkg subcommands operate on: porchapi (PackageRevision and friends), the
// porchconfig API (Repository), corev1 (ConfigMap and other core kinds), and
// metav1 (Status, WatchEvent and other meta types required for list/watch
// decoding). The set mirrors the scheme used by cliutils.CreateClientWithFlags
// so that commands which build their own client see the same kinds.
func CreateScheme() (*runtime.Scheme, error) {
	return buildScheme([]func(*runtime.Scheme) error{
		porchapi.AddToScheme,
		configapi.AddToScheme,
		corev1.AddToScheme,
		metav1.AddMetaToScheme,
	})
}

// buildScheme registers the given AddToScheme functions on a fresh scheme,
// returning the first error encountered. Splitting this out lets tests inject
// a deliberately failing adder to cover the error branch.
func buildScheme(adders []func(*runtime.Scheme) error) (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	for _, add := range adders {
		if err := add(scheme); err != nil {
			return nil, err
		}
	}
	return scheme, nil
}

// MakePreRunE returns a cobra PreRunE function that validates the namespace flag
// and creates a controller-runtime client, storing it in *clientPtr.
// Pass the command's op constant for error context (e.g. command + ".preRunE").
func MakePreRunE(op errors.Op, cfg *genericclioptions.ConfigFlags, clientPtr *client.Client) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		client, err := InitClient(cmd, cfg)
		if err != nil {
			return errors.E(op, err)
		}
		*clientPtr = client
		return nil
	}
}

// Runner holds the common fields used by rpkg lifecycle commands.
type Runner struct {
	Ctx     context.Context
	Cfg     *genericclioptions.ConfigFlags
	Client  client.Client
	Command *cobra.Command
}

// retryOnConflict retries fn on conflict errors with exponential backoff.
// Unlike retry.RetryOnConflict from client-go, this wrapper tracks the last
// error from fn and returns it reliably even when the underlying library
// silently drops it due to a known edge case in ExponentialBackoff.
func retryOnConflict(fn func() error) error {
	var lastErr error
	wrapped := func() error {
		lastErr = fn()
		return lastErr
	}
	err := retry.RetryOnConflict(retry.DefaultRetry, wrapped)
	if err == nil && lastErr != nil {
		return lastErr
	}
	return err
}

// PackageAction processes a single PackageRevision and returns a success message.
// Return ("", nil) to skip printing (e.g. "already proposed" cases that write to stderr directly).
type PackageAction func(ctx context.Context, client client.Client, pr *porchapi.PackageRevision) (string, error)

// RunForEachPackage loops over package names, fetches each PackageRevision,
// optionally runs a pre-check, then runs the action. Errors are collected
// and reported. When withRetry is true, the cycle is retried on conflict.
// When checkReadiness is true, readiness gates are verified before the action.
//
// The cmdName argument is the calling sub-command's identifier (typically the
// per-package "command" const such as "cmdrpkgapprove"). It is suffixed with
// ".runE" to build the errors.Op tag so failures align with the existing
// op-naming convention used by the other rpkg sub-commands.
func RunForEachPackage(
	ctx context.Context,
	cmdName string,
	c client.Client,
	cmd *cobra.Command,
	namespace string,
	args []string,
	withRetry bool,
	checkReadiness bool,
	action PackageAction,
) error {
	op := errors.Op(cmdName + ".runE")
	var messages []string

	for _, name := range args {
		var successMsg string

		run := func() error {
			var pr porchapi.PackageRevision
			if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &pr); err != nil {
				return err
			}
			if checkReadiness && !porchapi.PackageRevisionIsReady(pr.Spec.ReadinessGates, pr.Status.Conditions) {
				return fmt.Errorf("readiness conditions not met")
			}
			msg, err := action(ctx, c, &pr)
			if err != nil {
				return err
			}
			successMsg = msg
			return nil
		}

		var err error
		if withRetry {
			err = retryOnConflict(run)
		} else {
			err = run()
		}

		if err != nil {
			messages = append(messages, err.Error())
			fmt.Fprintf(cmd.ErrOrStderr(), "%s failed (%s)\n", name, err)
		} else if successMsg != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", successMsg)
		}
	}

	if len(messages) > 0 {
		return errors.E(op, fmt.Errorf("errors:\n  %s", strings.Join(messages, "\n  ")))
	}
	return nil
}

func GetResourceFileKubeObject(prr *porchapi.PackageRevisionResources, file, kind, name string) (*fnsdk.KubeObject, error) {
	if prr.Spec.Resources == nil {
		return nil, fmt.Errorf("nil resources found for PackageRevisionResources '%s/%s'", prr.Namespace, prr.Name)
	}

	if _, ok := prr.Spec.Resources[file]; !ok {
		return nil, fmt.Errorf("%q not found in PackageRevisionResources '%s/%s'", file, prr.Namespace, prr.Name)
	}

	ko, err := fnsdk.ParseKubeObject([]byte(prr.Spec.Resources[file]))
	if err != nil {
		return nil, fmt.Errorf("failed to parse %q of PackageRevisionResources %s/%s: %w", file, prr.Namespace, prr.Name, err)
	}
	if kind != "" && ko.GetKind() != kind {
		return nil, fmt.Errorf("%q does not contain kind %q in PackageRevisionResources '%s/%s'", file, kind, prr.Namespace, prr.Name)
	}
	if name != "" && ko.GetName() != name {
		return nil, fmt.Errorf("%q does not contain resource named %q in PackageRevisionResources '%s/%s'", file, name, prr.Namespace, prr.Name)
	}

	return ko, nil
}

func GetResourceVersion(prr *porchapi.PackageRevisionResources) (string, error) {
	ko, err := GetResourceFileKubeObject(prr, kptfilev1.RevisionMetaDataFileName, kptfilev1.RevisionMetaDataKind, "")
	if err != nil {
		return "", err
	}
	rv, _, _ := ko.NestedString("metadata", "resourceVersion")
	return rv, nil
}

func AddRevisionMetadata(prr *porchapi.PackageRevisionResources) error {
	kptMetaDataKo := fnsdk.NewEmptyKubeObject()
	if err := kptMetaDataKo.SetAPIVersion(prr.APIVersion); err != nil {
		return fmt.Errorf("cannot set Api Version: %v", err)
	}
	if err := kptMetaDataKo.SetKind(kptfilev1.RevisionMetaDataKind); err != nil {
		return fmt.Errorf("cannot set Kind: %v", err)
	}
	if err := kptMetaDataKo.SetNestedField(prr.GetObjectMeta(), "metadata"); err != nil {
		return fmt.Errorf("cannot set metadata: %v", err)
	}
	prr.Spec.Resources[kptfilev1.RevisionMetaDataFileName] = kptMetaDataKo.String()

	return nil
}

func RemoveRevisionMetadata(prr *porchapi.PackageRevisionResources) error {
	delete(prr.Spec.Resources, kptfilev1.RevisionMetaDataFileName)
	return nil
}
