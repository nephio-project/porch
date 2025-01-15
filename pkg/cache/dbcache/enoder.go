// Copyright 2024 The Nephio Authors
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

package dbcache

import (
	"fmt"

	"github.com/fxamacker/cbor/v2"
	"gopkg.in/yaml.v2"
)

type PackageResourceEncoding string

const (
	PackageResourceEncodingYAML PackageResourceEncoding = "yaml"
	PackageResourceEncodingCBOR PackageResourceEncoding = "cbor"
)

type encoder struct {
	encoding PackageResourceEncoding
}

func (e encoder) EncodePackage(resources map[string]string) ([]byte, error) {
	if resources == nil {
		resources = make(map[string]string)
	}

	switch e.encoding {
	case PackageResourceEncodingYAML:
		return yaml.Marshal(resources)

	case PackageResourceEncodingCBOR:
		return cbor.Marshal(resources)

	default:
		return nil, fmt.Errorf("encoding format not supported: %s", e.encoding)
	}
}

func (e encoder) DecodePackage(rawResources []byte) (map[string]string, error) {
	resources := make(map[string]string)

	switch e.encoding {
	case PackageResourceEncodingYAML:
		if err := yaml.Unmarshal(rawResources, resources); err == nil {
			return resources, nil
		} else {
			return nil, err
		}

	case PackageResourceEncodingCBOR:
		if err := cbor.Unmarshal(rawResources, &resources); err == nil {
			return resources, nil
		} else {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("ehcoding format not supported: %s", e.encoding)
	}
}
