package processor

import (
	"testing"
	"time"

	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func TestSleepProcessor_DefaultSleep(t *testing.T) {
	start := time.Now()
	p := &SleepProcessor{}
	rl := &framework.ResourceList{
		FunctionConfig: nil,
	}
	err := p.Process(rl)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	duration := time.Since(start)
	if duration < 10*time.Second {
		t.Errorf("expected at least 10 seconds sleep, got %v", duration)
	}
}

func TestSleepProcessor_CustomSleep(t *testing.T) {
	start := time.Now()
	node, err := yaml.Parse(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  sleepSeconds: "2"
`)
	if err != nil {
		t.Fatalf("failed to parse yaml: %v", err)
	}

	p := &SleepProcessor{}
	rl := &framework.ResourceList{
		FunctionConfig: node,
	}
	err = p.Process(rl)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	duration := time.Since(start)
	if duration < 2*time.Second {
		t.Errorf("expected at least 2 seconds sleep, got %v", duration)
	}
}
