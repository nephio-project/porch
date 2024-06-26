// Copyright 2022 The kpt and Nephio Authors
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

package reject

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func createScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	for _, api := range (runtime.SchemeBuilder{
		porchapi.AddToScheme,
	}) {
		if err := api(scheme); err != nil {
			return nil, err
		}
	}
	scheme.AddKnownTypes(porchapi.SchemeGroupVersion, &porchapi.PackageRevision{})
	return scheme, nil
}

func TestCmd(t *testing.T) {
	pkgRevName := "test-pr"
	repoName := "test-repo"
	ns := "ns"
	var scheme, err = createScheme()
	if err != nil {
		t.Fatalf("error creating scheme: %v", err)
	}
	testCases := map[string]struct {
		output  string
		wantErr bool
		fc      client.WithWatch
	}{
		"Package not found in ns": {
			wantErr: true,
			fc: fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(&porchapi.PackageRevision{
					TypeMeta: metav1.TypeMeta{
						Kind:       "PackageRevision",
						APIVersion: porchapi.SchemeGroupVersion.Identifier(),
					},
					Spec: porchapi.PackageRevisionSpec{
						Lifecycle: porchapi.PackageRevisionLifecycleDraft,
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "dummy-ns",
						Name:      pkgRevName,
					}}).Build(),
		},
		"Reject deletion proposed": {
			output: pkgRevName + " no longer proposed for deletion\n",
			fc: fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(&porchapi.PackageRevision{
					TypeMeta: metav1.TypeMeta{
						Kind:       "PackageRevision",
						APIVersion: porchapi.SchemeGroupVersion.Identifier(),
					},
					Spec: porchapi.PackageRevisionSpec{
						Lifecycle:      porchapi.PackageRevisionLifecycleDeletionProposed,
						RepositoryName: repoName,
						PackageName:    pkgRevName,
						WorkspaceName:  "v1",
						Revision:       "latest",
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      pkgRevName,
					}}).Build(),
		},
		"Reject draft": {
			wantErr: true,
			output:  "cannot reject " + pkgRevName + " with lifecycle 'Draft'\n",
			fc: fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(&porchapi.PackageRevision{
					TypeMeta: metav1.TypeMeta{
						Kind:       "PackageRevision",
						APIVersion: porchapi.SchemeGroupVersion.Identifier(),
					},
					Spec: porchapi.PackageRevisionSpec{
						Lifecycle: porchapi.PackageRevisionLifecycleDraft,
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      pkgRevName,
					}}).Build(),
		},
		"Reject proposed": {
			wantErr: false,
			output:  pkgRevName + " rejected\n",
			fc: fake.NewClientBuilder().WithScheme(scheme).
				WithObjects([]client.Object{&porchapi.PackageRevision{
					TypeMeta: metav1.TypeMeta{
						Kind:       "PackageRevision",
						APIVersion: porchapi.SchemeGroupVersion.Identifier(),
					},
					Spec: porchapi.PackageRevisionSpec{
						Lifecycle: porchapi.PackageRevisionLifecycleProposed,
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      pkgRevName,
					}}}...).Build(),
		},
	}

	for tn := range testCases {
		tc := testCases[tn]
		t.Run(tn, func(t *testing.T) {

			cmd := &cobra.Command{}
			o := os.Stdout
			e := os.Stderr
			read, write, _ := os.Pipe()
			os.Stdout = write
			os.Stderr = write

			r := &runner{
				ctx: context.Background(),
				cfg: &genericclioptions.ConfigFlags{
					Namespace: &ns,
				},
				client:  tc.fc,
				Command: cmd,
			}
			go func() {
				defer write.Close()
				err := r.runE(cmd, []string{pkgRevName})
				if err != nil && !tc.wantErr {
					t.Errorf("unexpected error: %v", err)
				}
			}()
			out, _ := io.ReadAll(read)
			os.Stdout = o
			os.Stderr = e

			if diff := cmp.Diff(string(tc.output), string(out)); diff != "" {
				t.Errorf("Unexpected result (-want, +got): %s", diff)
			}
		})
	}
}
