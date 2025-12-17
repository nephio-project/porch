// Copyright 2024 The kpt and Nephio Authors
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

package propose

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	porchapiv1alpha1 "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func createScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()

	for _, api := range (runtime.SchemeBuilder{
		porchapiv1alpha1.AddToScheme,
	}) {
		if err := api(scheme); err != nil {
			return nil, err
		}
	}
	return scheme, nil
}

func TestCmd(t *testing.T) {
	pkgRevName := "test-fjdos9u2nfe2f32"
	ns := "ns"
	var scheme, err = createScheme()
	if err != nil {
		t.Fatalf("error creating scheme: %v", err)
	}
	testCases := map[string]struct {
		lc      porchapiv1alpha1.PackageRevisionLifecycle
		output  string
		wantErr bool
	}{
		"Package already proposed": {
			output: pkgRevName + " is already proposed\n",
			lc:     porchapiv1alpha1.PackageRevisionLifecycleProposed,
		},
		"Propose package": {
			output: pkgRevName + " proposed\n",
			lc:     porchapiv1alpha1.PackageRevisionLifecycleDraft,
		},
		"Cannot propose package": {
			output:  pkgRevName + " failed (cannot propose Published package)\n",
			lc:      porchapiv1alpha1.PackageRevisionLifecyclePublished,
			wantErr: true,
		},
	}

	for tn := range testCases {
		tc := testCases[tn]
		t.Run(tn, func(t *testing.T) {
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(&porchapiv1alpha1.PackageRevision{
					TypeMeta: metav1.TypeMeta{
						Kind:       "PackageRevision",
						APIVersion: porchapiv1alpha1.SchemeGroupVersion.Identifier(),
					},
					Spec: porchapiv1alpha1.PackageRevisionSpec{
						Lifecycle: tc.lc,
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      pkgRevName,
					},
				}).Build()

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
