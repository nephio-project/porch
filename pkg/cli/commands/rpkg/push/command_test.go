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

package push

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/kpt/printer"
	fakeprint "github.com/nephio-project/porch/pkg/kpt/printer/fake"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCmd(t *testing.T) {
	pkgRevName := "test-fjdos9u2nfe2f32"
	ns := "ns"
	pkgDir := "testdata/test-fjdos9u2nfe2f32"

	scheme, err := createScheme()
	if err != nil {
		t.Fatalf("error creating scheme: %v", err)
	}

	testCases := map[string]struct {
		output  string
		args    []string
		wantErr bool
	}{
		"Push package": {
			output: pkgRevName + " pushed\n",
			args:   []string{pkgRevName, pkgDir},
		},
		"Args not provided": {
			wantErr: true,
		},
		"Pkgdir not provided": {
			args:    []string{pkgRevName},
			wantErr: true,
		},
		".KptRevisionMetadata missing": {
			args:    []string{pkgRevName, "dummy"},
			wantErr: true,
		},
	}

	for tn := range testCases {
		tc := testCases[tn]
		t.Run(tn, func(t *testing.T) {
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(&porchapi.PackageRevisionResources{
					ObjectMeta: metav1.ObjectMeta{
						Name:      pkgRevName,
						Namespace: ns,
					},
				}).
				Build()
			output := &bytes.Buffer{}
			o := os.Stdout
			e := os.Stderr
			read, write, _ := os.Pipe()
			os.Stdout = write
			os.Stderr = write
			ctx := fakeprint.CtxWithPrinter(output, output)
			r := &runner{
				ctx: ctx,
				cfg: &genericclioptions.ConfigFlags{
					Namespace: &ns,
				},
				client:  c,
				printer: printer.FromContextOrDie(ctx),
			}
			cmd := &cobra.Command{}

			go func() {
				defer write.Close()
				err := r.runE(cmd, tc.args)
				if err != nil && !tc.wantErr {
					t.Errorf("unexpected error: %v", err)
				}
			}()
			io.Copy(output, read)
			os.Stdout = o
			os.Stderr = e

			if diff := cmp.Diff(string(tc.output), output.String()); diff != "" {
				t.Errorf("Unexpected result (-want, +got): %s", diff)
			}
		})
	}
}
