// Copyright 2026 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package collectors

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/api/monitoring/v3"
)

// flakyHandler serves the Monitoring API, failing the first failures calls to the
// endpoint containing failPath with the given HTTP status.
type flakyHandler struct {
	failPath   string
	failStatus int
	failures   int

	mu    sync.Mutex
	calls map[string]int
}

func (h *flakyHandler) count(path string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls[path]
}

func (h *flakyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	endpoint := "other"
	switch {
	case strings.Contains(r.URL.Path, "metricDescriptors"):
		endpoint = "metricDescriptors"
	case strings.Contains(r.URL.Path, "timeSeries"):
		endpoint = "timeSeries"
	}

	h.mu.Lock()
	if h.calls == nil {
		h.calls = map[string]int{}
	}
	h.calls[endpoint]++
	seen := h.calls[endpoint]
	h.mu.Unlock()

	if endpoint == h.failPath && seen <= h.failures {
		w.WriteHeader(h.failStatus)
		fmt.Fprintf(w, `{"error":{"code":%d,"message":"transient"}}`, h.failStatus)
		return
	}

	if endpoint == "metricDescriptors" {
		if err := json.NewEncoder(w).Encode(&monitoring.ListMetricDescriptorsResponse{
			MetricDescriptors: []*monitoring.MetricDescriptor{{
				Type:       testMetricsTypePrefix + "/flaky",
				MetricKind: "GAUGE",
				ValueType:  "DOUBLE",
			}},
		}); err != nil {
			panic(err)
		}
		return
	}
	fmt.Fprint(w, "{}")
}

func TestScrapeRetriesTransientAPIErrors(t *testing.T) {
	origBase, origMax := retryBaseBackoff, retryMaxBackoff
	retryBaseBackoff, retryMaxBackoff = time.Millisecond, 5*time.Millisecond
	defer func() { retryBaseBackoff, retryMaxBackoff = origBase, origMax }()

	for _, tc := range []struct {
		name       string
		failPath   string
		failStatus int
		failures   int
		wantErr    bool
		wantCalls  int
	}{
		{"descriptors recover from 503", "metricDescriptors", http.StatusServiceUnavailable, 2, false, 3},
		{"descriptors recover from 429", "metricDescriptors", http.StatusTooManyRequests, 1, false, 2},
		{"descriptors give up after max attempts", "metricDescriptors", http.StatusServiceUnavailable, retryMaxAttempts, true, retryMaxAttempts},
		{"descriptors do not retry 400", "metricDescriptors", http.StatusBadRequest, 1, true, 1},
		{"time series recover from 503", "timeSeries", http.StatusServiceUnavailable, 2, false, 3},
		{"time series give up after max attempts", "timeSeries", http.StatusServiceUnavailable, retryMaxAttempts, true, retryMaxAttempts},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := &flakyHandler{failPath: tc.failPath, failStatus: tc.failStatus, failures: tc.failures}
			collector := newCacheTestCollector(t, handler.ServeHTTP, NewInMemoryDescriptorCache(time.Hour))

			err := scrape(collector)
			if tc.wantErr && err == nil {
				t.Error("expected scrape error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected scrape success, got %v", err)
			}
			if got := handler.count(tc.failPath); got != tc.wantCalls {
				t.Errorf("expected %d %s calls, got %d", tc.wantCalls, tc.failPath, got)
			}
		})
	}
}
