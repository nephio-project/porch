// Copyright 2025 The Nephio Authors
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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUtil(t *testing.T) {
	dbSQL := dbSQL{}

	err := dbSQL.Close()
	assert.Equal(t, "cannot close database, database is not initialized", err.Error())

	_, err = dbSQL.Exec("")
	assert.Equal(t, "cannot execute query on database, database is not initialized", err.Error())

	_, err = dbSQL.Query("")
	assert.Equal(t, "cannot query database, database is not initialized", err.Error())

	nilRow := dbSQL.QueryRow("")
	assert.Nil(t, nilRow)
}
