package processor

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type SleepProcessor struct{}

func (p *SleepProcessor) Process(rl *framework.ResourceList) error {
	sleep := 10
	if rl.FunctionConfig != nil && rl.FunctionConfig.GetKind() == "ConfigMap" {
		node := rl.FunctionConfig
		valueNode, err := node.Pipe(yaml.Lookup("data", "sleepSeconds"))
		if err == nil && valueNode != nil {
			raw, err := valueNode.String()
			if err == nil {
				raw = strings.TrimSpace(raw)
				if parsed, err := strconv.Atoi(raw); err == nil {
					sleep = parsed
				}
			}
		}
	}
	fmt.Fprintf(os.Stderr, "Sleeping for %d seconds...\n", sleep)
	time.Sleep(time.Duration(sleep) * time.Second)
	fmt.Fprintln(os.Stderr, "Sleep completed.")
	return nil
}
