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

package reject

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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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
	scheme.AddKnownTypes(porchapiv1alpha1.SchemeGroupVersion, &porchapiv1alpha1.PackageRevision{})
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
		output     string
		wantErr    bool
		fakeclient client.WithWatch
	}{
		"Package not found in ns": {
			output:  pkgRevName + " failed (packagerevisions.porch.kpt.dev \"" + pkgRevName + "\" not found)\n",
			wantErr: true,
			fakeclient: fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(&porchapiv1alpha1.PackageRevision{
					TypeMeta: metav1.TypeMeta{
						Kind:       "PackageRevision",
						APIVersion: porchapiv1alpha1.SchemeGroupVersion.Identifier(),
					},
					Spec: porchapiv1alpha1.PackageRevisionSpec{
						Lifecycle:      porchapiv1alpha1.PackageRevisionLifecycleProposed,
						RepositoryName: repoName,
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "dummy",
						Name:      pkgRevName,
					}}).Build(),
		},
		"Reject deletion-proposed package": {
			output: pkgRevName + " no longer proposed for deletion\n",
			fakeclient: fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(&porchapiv1alpha1.PackageRevision{
					TypeMeta: metav1.TypeMeta{
						Kind:       "PackageRevision",
						APIVersion: porchapiv1alpha1.SchemeGroupVersion.Identifier(),
					},
					Spec: porchapiv1alpha1.PackageRevisionSpec{
						Lifecycle:      porchapiv1alpha1.PackageRevisionLifecycleDeletionProposed,
						RepositoryName: repoName,
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      pkgRevName,
					}}).Build(),
		},
		"Reject draft package": {
			wantErr: true,
			output:  pkgRevName + " failed (cannot reject " + pkgRevName + " with lifecycle 'Draft')\n",
			fakeclient: fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(&porchapiv1alpha1.PackageRevision{
					TypeMeta: metav1.TypeMeta{
						Kind:       "PackageRevision",
						APIVersion: porchapiv1alpha1.SchemeGroupVersion.Identifier(),
					},
					Spec: porchapiv1alpha1.PackageRevisionSpec{
						Lifecycle:      porchapiv1alpha1.PackageRevisionLifecycleDraft,
						RepositoryName: repoName,
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      pkgRevName,
					}}).Build(),
		},
		"Reject published package": {
			wantErr: true,
			output:  pkgRevName + " failed (cannot reject " + pkgRevName + " with lifecycle 'Published')\n",
			fakeclient: fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(&porchapiv1alpha1.PackageRevision{
					TypeMeta: metav1.TypeMeta{
						Kind:       "PackageRevision",
						APIVersion: porchapiv1alpha1.SchemeGroupVersion.Identifier(),
					},
					Spec: porchapiv1alpha1.PackageRevisionSpec{
						Lifecycle:      porchapiv1alpha1.PackageRevisionLifecyclePublished,
						RepositoryName: repoName,
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      pkgRevName,
					}}).Build(),
		},
		"Reject proposed package": {
			output: pkgRevName + " no longer proposed for approval\n",
			fakeclient: fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
				//fake subresourceupdate
				SubResourceUpdate: func(ctx context.Context, client client.Client, subResourceName string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
					return nil
				},
			}).WithScheme(scheme).
				WithRuntimeObjects(&porchapiv1alpha1.PackageRevision{
					TypeMeta: metav1.TypeMeta{
						Kind:       "PackageRevision",
						APIVersion: porchapiv1alpha1.SchemeGroupVersion.Identifier(),
					},
					Spec: porchapiv1alpha1.PackageRevisionSpec{
						Lifecycle:      porchapiv1alpha1.PackageRevisionLifecycleProposed,
						RepositoryName: repoName,
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      pkgRevName,
					}}).Build(),
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
				client:  tc.fakeclient,
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
