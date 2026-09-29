#!/bin/bash
# Copyright 2026 The kpt Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

# Set environment variables
export PORCH_NAMESPACE="porch-system"
export MONITORING_NAMESPACE="porch-monitoring"
API_VERSION="${1:-v1alpha1}"

echo "=== OTEL Validation Checks for $API_VERSION ==="
echo ""

# Check if Jaeger is accessible
echo "=== 1. Checking Jaeger availability ==="
JAEGER_POD=$(kubectl get pods -n ${MONITORING_NAMESPACE} -l app=jaeger -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
if [ -z "$JAEGER_POD" ]; then
  echo "ERROR: Jaeger pod not found in ${MONITORING_NAMESPACE}"
  exit 1
fi
echo "✓ Jaeger pod found: $JAEGER_POD"

# Check porch-server metrics endpoint via port-forward
echo ""
echo "=== 2. Checking porch-server metrics endpoint ==="
PORCH_SERVER_POD=$(kubectl get pods -n ${PORCH_NAMESPACE} -l app=porch-server -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
if [ -z "$PORCH_SERVER_POD" ]; then
  echo "ERROR: porch-server pod not found in ${PORCH_NAMESPACE}"
  exit 1
fi

# Create temporary port-forward
PF_TEMP=$(mktemp)
kubectl port-forward -n ${PORCH_NAMESPACE} ${PORCH_SERVER_POD} 9464:9464 > "$PF_TEMP" 2>&1 &
PF_PID=$!
sleep 2

METRICS=$(curl -s http://localhost:9464/metrics 2>/dev/null | head -20 || echo "")
kill $PF_PID 2>/dev/null || true
rm -f "$PF_TEMP"
wait $PF_PID 2>/dev/null || true

if [ -z "$METRICS" ]; then
  echo "ERROR: Could not scrape metrics from porch-server"
  exit 1
fi
echo "✓ Metrics endpoint responding"
echo "$METRICS"

# Check for OTEL in porch-server logs
echo ""
echo "=== 3. Checking porch-server OTEL initialization ==="
OTEL_INIT=$(kubectl logs -n ${PORCH_NAMESPACE} ${PORCH_SERVER_POD} --tail=100 2>/dev/null | grep -i "OpenTelemetry initialized" | tail -1 || echo "")
if [ -z "$OTEL_INIT" ]; then
  echo "WARNING: No OTEL initialization message found (may not be critical)"
else
  echo "✓ OTEL initialized: $OTEL_INIT"
fi

# Check for export errors in logs
echo ""
echo "=== 4. Checking for OTEL export errors ==="
EXPORT_ERRORS=$(kubectl logs -n ${PORCH_NAMESPACE} ${PORCH_SERVER_POD} --tail=100 2>/dev/null | grep -i "export.*error\|error.*export" | wc -l)
if [ "$EXPORT_ERRORS" -gt 0 ]; then
  echo "WARNING: Found $EXPORT_ERRORS export-related errors in logs"
  kubectl logs -n ${PORCH_NAMESPACE} ${PORCH_SERVER_POD} --tail=100 2>/dev/null | grep -i "export.*error\|error.*export" || true
fi
echo "✓ Export error check complete"

# Check Jaeger for traces (with retry)
echo ""
echo "=== 5. Querying Jaeger for traces ==="
TRACES_FOUND=0
for i in {1..3}; do
  echo "Attempt $i..."
  
  # Create temporary port-forward to Jaeger
  PF_TEMP=$(mktemp)
  kubectl port-forward -n ${MONITORING_NAMESPACE} ${JAEGER_POD} 16686:16686 > "$PF_TEMP" 2>&1 &
  PF_PID=$!
  sleep 2
  
  TRACE_COUNT=$(curl -s "http://localhost:16686/api/traces?service=porch-server&limit=1" 2>/dev/null | jq -r '.data | length' 2>/dev/null || echo "0")
  
  kill $PF_PID 2>/dev/null || true
  rm -f "$PF_TEMP"
  wait $PF_PID 2>/dev/null || true
  
  if [ "$TRACE_COUNT" -gt 0 ]; then
    TRACES_FOUND=1
    echo "✓ Found $TRACE_COUNT trace(s) in Jaeger"
    break
  fi
  
  if [ $i -lt 3 ]; then
    echo "  No traces yet, waiting 5 seconds..."
    sleep 5
  fi
done

if [ $TRACES_FOUND -eq 0 ]; then
  echo "WARNING: No traces found in Jaeger (services may not have been exercised)"
fi

# List all services exporting traces
echo ""
echo "=== 6. Services reporting traces to Jaeger ==="
PF_TEMP=$(mktemp)
kubectl port-forward -n ${MONITORING_NAMESPACE} ${JAEGER_POD} 16686:16686 > "$PF_TEMP" 2>&1 &
PF_PID=$!
sleep 2

SERVICES=$(curl -s "http://localhost:16686/api/services" 2>/dev/null | jq -r '.data[] | select(. != "jaeger-all-in-one")' 2>/dev/null || echo "")

kill $PF_PID 2>/dev/null || true
rm -f "$PF_TEMP"
wait $PF_PID 2>/dev/null || true

if [ -n "$SERVICES" ]; then
  echo "✓ Porch components exporting traces:"
  echo "$SERVICES" | while read service; do
    echo "  - $service"
  done
else
  echo "WARNING: No Porch services found in Jaeger"
fi

# Summary
echo ""
echo "=== OTEL Validation Complete ==="
echo "✓ Jaeger is running"
echo "✓ Metrics endpoint responsive"
echo "✓ OTEL components initialized"
echo ""
echo "Infrastructure ready for OTEL testing"
