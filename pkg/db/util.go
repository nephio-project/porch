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

package db

import (
	"bytes"
	"os/user"
	"strings"

	"github.com/google/uuid"
	"github.com/nephio-project/porch/api/porch/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
)

func getCurrentUser() string {
	currentUser, err := user.Current()
	if err == nil {
		return currentUser.Username
	} else {
		return "undefined"
	}
}

func generateUid(prefix string, kubeNs string, kubeName string) types.UID {
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
