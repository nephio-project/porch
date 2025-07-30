// Copyright 2019 The kpt and Nephio Authors
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

package cmdutil

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
)

const (
	StackTraceOnErrors  = "COBRA_STACK_TRACE_ON_ERRORS"
	trueString          = "true"
	Stdout              = "stdout"
	Unwrap              = "unwrap"
	FunctionsCatalogURL = "https://catalog.kpt.dev/catalog-v2.json"
)

// FixDocs replaces instances of old with new in the docs for c
func FixDocs(old, new string, c *cobra.Command) {
	c.Use = strings.ReplaceAll(c.Use, old, new)
	c.Short = strings.ReplaceAll(c.Short, old, new)
	c.Long = strings.ReplaceAll(c.Long, old, new)
	c.Example = strings.ReplaceAll(c.Example, old, new)
}

func PrintErrorStacktrace() bool {
	e := os.Getenv(StackTraceOnErrors)
	if StackOnError || e == trueString || e == "1" {
		return true
	}
	return false
}

// StackOnError if true, will print a stack trace on failure.
var StackOnError bool

// WriteFnOutput writes the output resources of function commands to provided destination
func WriteFnOutput(dest, content string, fromStdin bool, w io.Writer) error {
	r := strings.NewReader(content)
	switch dest {
	case Stdout:
		// if user specified dest is "stdout" directly write the content as it is already wrapped
		_, err := w.Write([]byte(content))
		if err != nil {
			return fmt.Errorf("failed to write content to stdout: %w", err)
		}
		return nil
	case Unwrap:
		// if user specified dest is "unwrap", write the unwrapped content to the provided writer
		return WriteToOutput(r, w, "")
	case "":
		if fromStdin {
			// if user didn't specify dest, and if input is from STDIN, write the wrapped content provided writer
			// this is same as "stdout" input above
			_, err := w.Write([]byte(content))
			if err != nil {
				return fmt.Errorf("failed to write content from stdin: %w", err)
			}
			return nil
		}
	default:
		// this means user specified a directory as dest, write the content to dest directory
		return WriteToOutput(r, nil, dest)
	}
	return nil
}

// WriteToOutput reads the input from r and writes the output to either w or outDir
func WriteToOutput(r io.Reader, w io.Writer, outDir string) error {
	var outputs []kio.Writer
	if outDir != "" {
		err := os.MkdirAll(outDir, 0750)
		if err != nil {
			return fmt.Errorf("failed to create output directory %q: %w", outDir, err)
		}
		outputs = []kio.Writer{&kio.LocalPackageWriter{PackagePath: outDir}}
	} else {
		outputs = []kio.Writer{&kio.ByteWriter{
			Writer: w,
			ClearAnnotations: []string{kioutil.IndexAnnotation, kioutil.PathAnnotation,
				kioutil.LegacyIndexAnnotation, kioutil.LegacyPathAnnotation}}, // nolint:staticcheck
		}
	}

	err := kio.Pipeline{
		Inputs:  []kio.Reader{&kio.ByteReader{Reader: r, PreserveSeqIndent: true, WrapBareSeqNode: true}},
		Outputs: outputs}.Execute()
	if err != nil {
		return fmt.Errorf("failed to execute kio pipeline: %w", err)
	}
	return nil
}

// CheckDirectoryNotPresent returns error if the directory already exists
func CheckDirectoryNotPresent(outDir string) error {
	_, err := os.Stat(outDir)
	if err == nil || os.IsExist(err) {
		return fmt.Errorf("directory %q already exists, please delete the directory and retry", outDir)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check directory %q: %w", outDir, err)
	}
	return nil
}

func GetKeywordsFromFlag(cmd *cobra.Command) []string {
	flagVal := cmd.Flag("keywords").Value.String()
	flagVal = strings.TrimPrefix(flagVal, "[")
	flagVal = strings.TrimSuffix(flagVal, "]")
	splitted := strings.Split(flagVal, ",")
	var trimmed []string
	for _, val := range splitted {
		if strings.TrimSpace(val) == "" {
			continue
		}
		trimmed = append(trimmed, strings.TrimSpace(val))
	}
	return trimmed
}
