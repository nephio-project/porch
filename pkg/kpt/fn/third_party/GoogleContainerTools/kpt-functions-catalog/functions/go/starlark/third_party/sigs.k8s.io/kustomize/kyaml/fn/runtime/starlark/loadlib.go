/*
 Copyright 2025 The Nephio Authors.

 Licensed under the Apache License, Version 2.0 (the "License");
 You may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package starlark

import (
	"github.com/nephio-project/porch/pkg/kpt/fn/third_party/GoogleContainerTools/kpt-functions-catalog/functions/go/starlark/krmfn"

	"github.com/qri-io/starlib/bsoup"
	"github.com/qri-io/starlib/encoding/base64"
	"github.com/qri-io/starlib/encoding/csv"
	"github.com/qri-io/starlib/encoding/json"
	"github.com/qri-io/starlib/encoding/yaml"
	"github.com/qri-io/starlib/geo"
	"github.com/qri-io/starlib/hash"
	"github.com/qri-io/starlib/html"
	"github.com/qri-io/starlib/http"
	"github.com/qri-io/starlib/math"
	"github.com/qri-io/starlib/re"
	"github.com/qri-io/starlib/time"
	"github.com/qri-io/starlib/xlsx"
	"github.com/qri-io/starlib/zipfile"
	"go.starlark.net/starlark"
)

// load loads starlark libraries from https://github.com/qri-io/starlib#packages and from
// our own custom libraries.
func load(_ *starlark.Thread, module string) (starlark.StringDict, error) {
	switch module {
	case bsoup.ModuleName:
		return bsoup.LoadModule()
	case base64.ModuleName:
		return base64.LoadModule()
	case csv.ModuleName:
		return csv.LoadModule()
	case json.ModuleName:
		return starlark.StringDict{"json": json.Module}, nil
	case yaml.ModuleName:
		return yaml.LoadModule()
	case geo.ModuleName:
		return geo.LoadModule()
	case hash.ModuleName:
		return hash.LoadModule()
	case html.ModuleName:
		return html.LoadModule()
	case http.ModuleName:
		return http.LoadModule()
	case math.ModuleName:
		return starlark.StringDict{"math": math.Module}, nil
	case re.ModuleName:
		return re.LoadModule()
	case time.ModuleName:
		return starlark.StringDict{"time": time.Module}, nil
	case xlsx.ModuleName:
		return xlsx.LoadModule()
	case zipfile.ModuleName:
		return zipfile.LoadModule()
	case krmfn.ModuleName:
		return starlark.StringDict{"krmfn": krmfn.Module}, nil
	}
	return nil, nil
}
