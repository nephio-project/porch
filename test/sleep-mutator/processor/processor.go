package processor

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"sigs.k8s.io/kustomize/kyaml/fn/framework"
)

type SleepProcessor struct{}

func (p *SleepProcessor) Process(rl *framework.ResourceList) error {
	sleep := 10
	config := rl.FunctionConfig
	if config != nil && config.GetKind() == "ConfigMap" {
		data := config.GetDataMap()
		if data == nil {
			err := fmt.Errorf("Couldn't parse FunctionConfig's data field")
			return err
		}
		raw, ok := data["sleepSeconds"]
		if ok {
			raw = strings.TrimSpace(raw)
			if parsed, err := strconv.Atoi(raw); err == nil {
				sleep = parsed
			} else {
				return fmt.Errorf("couldn't parse sleepSeconds field of functionConfig: %w", err)
			}
		}
	}
	fmt.Fprintf(os.Stderr, "Sleeping for %d seconds...\n", sleep)
	time.Sleep(time.Duration(sleep) * time.Second)
	fmt.Fprintln(os.Stderr, "Sleep completed.")
	return nil
}
