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

package server

import (
	"os"
	"testing"
	"time"

	porchv1alpha1 "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/apiserver"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	genericoptions "k8s.io/apiserver/pkg/server/options"
)

func TestAddFlags(t *testing.T) {
	versions := schema.GroupVersions{
		porchv1alpha1.SchemeGroupVersion,
	}
	o := PorchServerOptions{
		RecommendedOptions: genericoptions.NewRecommendedOptions(
			defaultEtcdPathPrefix,
			apiserver.Codecs.LegacyCodec(versions...),
		),
	}
	o.AddFlags(&pflag.FlagSet{})
	if o.RepoSyncFrequency < 5*time.Minute {
		t.Fatalf("AddFlags(): repo-sync-frequency cannot be less that 5 minutes.")
	}
}

func TestValidate(t *testing.T) {
	opts := NewPorchServerOptions(os.Stdout, os.Stderr)

	err := opts.Validate(nil)
	assert.True(t, err != nil)

	opts.CacheType = "CR"
	err = opts.Validate(nil)
	assert.True(t, err == nil)

	opts.CacheType = "cr"
	err = opts.Validate(nil)
	assert.True(t, err == nil)

	opts.CacheType = "DB"
	err = opts.Validate(nil)
	assert.True(t, err != nil)

	opts.CacheType = ""
	err = opts.Validate(nil)
	assert.True(t, err != nil)
}
