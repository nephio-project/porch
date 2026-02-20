package api

import (
	suiteutils "github.com/nephio-project/porch/test/e2e/suiteutils"
	"github.com/stretchr/testify/assert"
)

func (t *PorchSuite) TestMetricsEndpoint() {
	porchServerShouldHaveRegexList := []string{
		"go_*",
		"http_server_*",
		"http_client_*",
		"rest_client_*",
		"process_*",
		"rpc_client_*",
	}
	porchControllerShouldHaveRegexList := []string{
		"controller_*",
		"go_*",
	}
	porchFunctionRunnerShouldHaveRegexList := []string{
		"go_*",
		"rpc_server_*",
		// "rpc_client_*", //There is no way to force both function runners to have at least one connection, so no metrics
	}
	porchWrapperServerShouldHaveRegexList := []string{
		"go_*",
		"rpc_server_*",
	}

	//This is needed to ensure that there is at least one wrapper-server instance.

	resources := t.setupFunctionTestPackage("git-fn-distroless", "test-fn-redis-bucket", "test-description", TestPackageSetupOptions{
		UpstreamRef: "redis-bucket/v1",
		UpstreamDir: "redis-bucket",
	})

	resources.Spec.Resources["configmap.yaml"] = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: kptfile.kpt.dev
data:
  name: bucket-namespace
`

	t.AddMutator(resources, t.KrmFunctionsRegistry+"/"+setNamespaceImage, suiteutils.WithConfigPath("configmap.yaml"))
	t.UpdateF(resources)

	collectionResults, err := t.CollectMetricsFromPods()
	if err != nil {
		t.Fatalf("failed to collect metrics from pods: %v", err)
	}

	for _, regex := range porchServerShouldHaveRegexList {
		t.Regexp(regex, collectionResults.PorchServerMetrics, "porch server metrics should contain %q", regex)
	}

	for _, regex := range porchControllerShouldHaveRegexList {
		assert.Regexp(t.T(), regex, collectionResults.PorchControllerMetrics, "porch controller metrics should contain %q", regex)
	}

	for _, regex := range porchFunctionRunnerShouldHaveRegexList {
		assert.Regexp(t.T(), regex, collectionResults.PorchFunctionRunnerMetrics, "porch function runner metrics should contain %q", regex)
	}
	for _, regex := range porchWrapperServerShouldHaveRegexList {
		assert.Regexp(t.T(), regex, collectionResults.PorchWrapperServerMetrics, "porch wrapper server metrics should contain %q", regex)
	}
}
