package main

import (
	"fmt"
	"os"

	"github.com/kptdev/kpt-functions-catalog/functions/go/sleep/processor"
	"sigs.k8s.io/kustomize/kyaml/fn/framework/command"
)

func main() {
	cmd := command.Build(&processor.SleepProcessor{}, command.StandaloneEnabled, false)
	cmd.Short = "Sleep function simulates a delay for testing purposes."
	cmd.Long = "The sleep function reads `sleepSeconds` from FunctionConfig and delays execution. Useful for simulating latency in pipelines."
	cmd.Example = `apiVersion: v1
kind: ConfigMap
metadata:
  name: sleep-config
data:
  sleepSeconds: "5"`

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "\n\nERROR: %s\n", err)
		os.Exit(1)
	}
}
