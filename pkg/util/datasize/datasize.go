// Copyright 2026 The kpt Authors
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

// Package datasize provides named units, parsing and formatting for sizes of
// data in bytes, and a flag value that accepts sizes such as "6MB".
package datasize

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Units of data size. They are powers of 1024, so 6*MB is 6*1024*1024 bytes.
// They are untyped constants, so they can be used with any integer type.
const (
	KB = 1 << 10
	MB = 1 << 20
	GB = 1 << 30
)

// units maps the unit names accepted by Parse to their size in bytes.
var units = map[string]int64{
	"":    1,
	"B":   1,
	"KB":  KB,
	"KIB": KB,
	"MB":  MB,
	"MIB": MB,
	"GB":  GB,
	"GIB": GB,
}

// Parse parses a data size such as "6MB", "512KB" or "6291456" and returns it
// in bytes. A number without a unit is a number of bytes. The units are B, KB,
// MB and GB, and KiB, MiB and GiB are accepted as other names for KB, MB and GB.
// Units are powers of 1024 and are not case sensitive. Fractions and negative
// sizes are not accepted.
func Parse(s string) (int64, error) {
	trimmed := strings.TrimSpace(s)
	digits := 0
	for digits < len(trimmed) && trimmed[digits] >= '0' && trimmed[digits] <= '9' {
		digits++
	}
	if digits == 0 {
		return 0, fmt.Errorf("invalid data size %q: must start with a number", s)
	}
	number, err := strconv.ParseInt(trimmed[:digits], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid data size %q: %w", s, err)
	}
	unitName := strings.TrimSpace(trimmed[digits:])
	unit, ok := units[strings.ToUpper(unitName)]
	if !ok {
		return 0, fmt.Errorf("invalid data size %q: unknown unit %q, must be one of B, KB, MB, GB", s, unitName)
	}
	if number > math.MaxInt64/unit {
		return 0, fmt.Errorf("invalid data size %q: value is too large", s)
	}
	return number * unit, nil
}

// Format returns the size in the largest unit that represents it exactly, for
// example "6MB" for 6291456 and "1500B" for 1500. Parse reads the result back
// as the same size. A negative size is not a valid data size, and it is
// returned as a plain number.
func Format(size int64) string {
	switch {
	case size < 0:
		return strconv.FormatInt(size, 10)
	case size == 0:
		return "0B"
	case size%GB == 0:
		return fmt.Sprintf("%dGB", size/GB)
	case size%MB == 0:
		return fmt.Sprintf("%dMB", size/MB)
	case size%KB == 0:
		return fmt.Sprintf("%dKB", size/KB)
	default:
		return fmt.Sprintf("%dB", size)
	}
}

// Integer is the constraint for the types of the variables that a Value can set.
type Integer interface {
	~int | ~int64
}

// Value is a flag value for a data size. It implements the Value interfaces of
// both the flag and the pflag packages, and stores the size in bytes in an
// integer variable. It accepts everything that Parse accepts. It also accepts
// every number that the integer flags of the flag and pflag packages accept,
// with the same meaning, so a flag that used to take a number of bytes keeps
// working when it is changed to a Value.
type Value[T Integer] struct {
	target *T
}

// NewValue returns a Value that stores the size in target. It sets target to
// defaultSize, in the same way as flag.IntVar.
func NewValue[T Integer](target *T, defaultSize T) *Value[T] {
	*target = defaultSize
	return &Value[T]{target: target}
}

// Set parses s and stores the size in the target variable.
func (v *Value[T]) Set(s string) error {
	// Read a plain number in the same way as the integer flags do, so that
	// values such as "0x600000" or "-1" keep their meaning.
	size, err := strconv.ParseInt(s, 0, 64)
	if err != nil {
		size, err = Parse(s)
		if err != nil {
			return err
		}
	}
	if int64(T(size)) != size {
		return fmt.Errorf("invalid data size %q: value is too large", s)
	}
	*v.target = T(size)
	return nil
}

// String returns the current size in the format of Format.
func (v *Value[T]) String() string {
	// the flag package calls String on a zero Value to find out if the
	// default value of a flag is the zero value
	if v == nil || v.target == nil {
		return ""
	}
	return Format(int64(*v.target))
}

// Get returns the current size in the type of the target variable. It makes
// Value a flag.Getter, as the integer flags are.
func (v *Value[T]) Get() any {
	return *v.target
}

// Type returns the name of the type of the flag for the pflag package.
func (v *Value[T]) Type() string {
	return "size"
}
