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

package approve

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
		lc      porchapi.PackageRevisionLifecycle
		ns      string
	}{
		"Package not found in ns": {
			wantErr: true,
			ns:      "dummy",
		},
		"Approve draft package": {
			wantErr: true,
			output:  "cannot approve " + pkgRevName + " with lifecycle 'Draft'\n",
			ns:      ns,
			lc:      porchapi.PackageRevisionLifecycleDraft,
		},
		"Approve published package": {
			wantErr: true,
			output:  "cannot approve " + pkgRevName + " with lifecycle 'Published'\n",
			ns:      ns,
			lc:      porchapi.PackageRevisionLifecyclePublished,
		},
		"Approve deletion-proposed package": {
			output: pkgRevName + " approved\n",
			ns:     ns,
			lc:     porchapi.PackageRevisionLifecycleDeletionProposed,
		},
		"Approve proposed package": {
			output: pkgRevName + " approved\n",
			ns:     ns,
			lc:     porchapi.PackageRevisionLifecycleProposed,
		},
	}

	for tn := range testCases {
		tc := testCases[tn]
		t.Run(tn, func(t *testing.T) {

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(&porchapi.PackageRevision{
					TypeMeta: metav1.TypeMeta{
						Kind:       "PackageRevision",
						APIVersion: porchapi.SchemeGroupVersion.Identifier(),
					},
					Spec: porchapi.PackageRevisionSpec{
						Lifecycle:      tc.lc,
						RepositoryName: repoName,
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: tc.ns,
						Name:      pkgRevName,
					}}).Build()

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
				client:  c,
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
