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

package datasize

import (
	"flag"
	"io"
	"math"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	testCases := map[string]struct {
		input    string
		expected int64
	}{
		"number of bytes":       {input: "6291456", expected: 6291456},
		"zero":                  {input: "0", expected: 0},
		"bytes":                 {input: "1500B", expected: 1500},
		"kilobytes":             {input: "512KB", expected: 512 * 1024},
		"megabytes":             {input: "6MB", expected: 6 * 1024 * 1024},
		"gigabytes":             {input: "2GB", expected: 2 * 1024 * 1024 * 1024},
		"binary unit names":     {input: "512KiB", expected: 512 * 1024},
		"lower case unit":       {input: "6mb", expected: 6 * 1024 * 1024},
		"space before the unit": {input: "6 MB", expected: 6 * 1024 * 1024},
		"surrounding spaces":    {input: " 6MB ", expected: 6 * 1024 * 1024},
		"largest size":          {input: "9223372036854775807", expected: math.MaxInt64},
	}
	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			actual, err := Parse(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestParseErrors(t *testing.T) {
	testCases := map[string]struct {
		input       string
		expectedErr string
	}{
		"empty":               {input: "", expectedErr: "must start with a number"},
		"unit only":           {input: "MB", expectedErr: "must start with a number"},
		"negative":            {input: "-6MB", expectedErr: "must start with a number"},
		"fraction":            {input: "1.5MB", expectedErr: `unknown unit ".5MB"`},
		"unknown unit":        {input: "6TB", expectedErr: `unknown unit "TB"`},
		"number too large":    {input: "9223372036854775808", expectedErr: "value out of range"},
		"too large with unit": {input: "9007199254740992GB", expectedErr: "value is too large"},
	}
	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			_, err := Parse(tc.input)
			assert.ErrorContains(t, err, tc.expectedErr)
		})
	}
}

func TestFormat(t *testing.T) {
	testCases := map[string]struct {
		size     int64
		expected string
	}{
		"zero":                {size: 0, expected: "0B"},
		"bytes":               {size: 1500, expected: "1500B"},
		"kilobytes":           {size: 512 * KB, expected: "512KB"},
		"megabytes":           {size: 6 * MB, expected: "6MB"},
		"gigabytes":           {size: 2 * GB, expected: "2GB"},
		"not a whole unit":    {size: 6*MB + 1, expected: "6291457B"},
		"whole smaller units": {size: 1536 * KB, expected: "1536KB"},
	}
	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			actual := Format(tc.size)
			assert.Equal(t, tc.expected, actual)

			parsed, err := Parse(actual)
			require.NoError(t, err)
			assert.Equal(t, tc.size, parsed)
		})
	}
}

func TestFormatNegative(t *testing.T) {
	assert.Equal(t, "-1", Format(-1))
	assert.Equal(t, "-1024", Format(-KB))
}

// TestValueKeepsIntegerFlagSyntax checks that a Value reads every number that
// the integer flags accept, with the same result.
func TestValueKeepsIntegerFlagSyntax(t *testing.T) {
	for _, input := range []string{"6291456", "0", "-1", "+6291456", "0x600000", "010000000", "0b1000", "6_291_456"} {
		t.Run(input, func(t *testing.T) {
			var fromInt64Flag, fromValue int64
			intFlags := flag.NewFlagSet("test", flag.ContinueOnError)
			intFlags.Int64Var(&fromInt64Flag, "max-size", 0, "maximum size")
			require.NoError(t, intFlags.Parse([]string{"--max-size=" + input}))

			sizeFlags := flag.NewFlagSet("test", flag.ContinueOnError)
			sizeFlags.Var(NewValue(&fromValue, 0), "max-size", "maximum size")
			require.NoError(t, sizeFlags.Parse([]string{"--max-size=" + input}))

			assert.Equal(t, fromInt64Flag, fromValue)
		})
	}
}

func TestValueSetError(t *testing.T) {
	size := 6 * MB
	value := NewValue(&size, size)

	assert.ErrorContains(t, value.Set("-6MB"), "must start with a number")
	assert.ErrorContains(t, value.Set("6TB"), `unknown unit "TB"`)
	// the size is not changed by a value that is not valid
	assert.Equal(t, 6*MB, size)
}

func TestValueGet(t *testing.T) {
	var size int64
	var getter flag.Getter = NewValue(&size, int64(512*KB))
	assert.Equal(t, int64(512*KB), getter.Get())
}

func TestValueWithFlagPackage(t *testing.T) {
	testCases := map[string]struct {
		args        []string
		expected    int
		expectedErr string
	}{
		"default":         {args: nil, expected: 6 * MB},
		"size with unit":  {args: []string{"--max-size=10MB"}, expected: 10 * MB},
		"number of bytes": {args: []string{"--max-size=10485760"}, expected: 10 * MB},
		"invalid size":    {args: []string{"--max-size=big"}, expectedErr: `invalid data size "big"`},
	}
	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			var size int
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			fs.Var(NewValue(&size, 6*MB), "max-size", "maximum size")

			err := fs.Parse(tc.args)
			if tc.expectedErr != "" {
				assert.ErrorContains(t, err, tc.expectedErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expected, size)
		})
	}
}

func TestValueWithPflagPackage(t *testing.T) {
	var size int64
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.Var(NewValue(&size, int64(512*KB)), "max-size", "maximum size")

	assert.Equal(t, "512KB", fs.Lookup("max-size").DefValue)
	assert.Equal(t, int64(512*KB), size)

	require.NoError(t, fs.Parse([]string{"--max-size=2MB"}))
	assert.Equal(t, int64(2*MB), size)
	assert.Equal(t, "2MB", fs.Lookup("max-size").Value.String())
}

func TestValueZeroValue(t *testing.T) {
	// the flag package calls String on a zero Value when it prints the defaults
	assert.Equal(t, "", (&Value[int]{}).String())
	assert.Equal(t, "size", (&Value[int]{}).Type())
}
