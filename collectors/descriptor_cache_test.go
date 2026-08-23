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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/api/monitoring/v3"
	"google.golang.org/api/option"
)

const testMetricsTypePrefix = "custom.googleapis.com"

type recordingDescriptorCache struct {
	lookups int
	stores  int
	data    []*monitoring.MetricDescriptor
}

func (r *recordingDescriptorCache) Lookup(prefix string) []*monitoring.MetricDescriptor {
	r.lookups++
	return r.data
}

func (r *recordingDescriptorCache) Store(prefix string, data []*monitoring.MetricDescriptor) {
	r.stores++
}

type noopCounterStore struct{}

func (noopCounterStore) Increment(*monitoring.MetricDescriptor, *ConstMetric) {}
func (noopCounterStore) ListMetrics(string) []*ConstMetric                    { return nil }

type noopHistogramStore struct{}

func (noopHistogramStore) Increment(*monitoring.MetricDescriptor, *HistogramMetric) {}
func (noopHistogramStore) ListMetrics(string) []*HistogramMetric                    { return nil }

func newCacheTestCollector(t *testing.T, handler http.HandlerFunc, cache DescriptorCache) *MonitoringCollector {
	t.Helper()

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	service, err := monitoring.NewService(context.Background(), option.WithEndpoint(ts.URL), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("error creating monitoring service: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	collector, err := NewMonitoringCollector("test-project", service, MonitoringCollectorOptions{
		MetricTypePrefixes: []string{testMetricsTypePrefix},
		RequestInterval:    5 * time.Minute,
		DescriptorCache:    cache,
	}, logger, noopCounterStore{}, noopHistogramStore{})
	if err != nil {
		t.Fatalf("error creating collector: %v", err)
	}
	return collector
}

// descriptorPagesHandler serves the given descriptor pages in order, then empty time series.
// A nil page fails with a non-retryable 400.
func descriptorPagesHandler(t *testing.T, pages []*monitoring.ListMetricDescriptorsResponse) http.HandlerFunc {
	t.Helper()

	pageIndex := 0
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "metricDescriptors"):
			if pageIndex >= len(pages) {
				t.Errorf("unexpected metric descriptors list call, page %d", pageIndex)
				http.NotFound(w, r)
				return
			}
			page := pages[pageIndex]
			pageIndex++
			if page == nil {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"error":{"code":400,"message":"bad request"}}`)
				return
			}
			if err := json.NewEncoder(w).Encode(page); err != nil {
				t.Errorf("error encoding page: %v", err)
			}
		case strings.Contains(r.URL.Path, "timeSeries"):
			fmt.Fprint(w, "{}")
		default:
			http.NotFound(w, r)
		}
	}
}

func scrape(c *MonitoringCollector) error {
	ch := make(chan prometheus.Metric, 128)
	defer close(ch)
	return c.reportMonitoringMetrics(ch, time.Now())
}

func TestInjectedDescriptorCacheIsUsed(t *testing.T) {
	cache := &recordingDescriptorCache{
		data: []*monitoring.MetricDescriptor{{Type: testMetricsTypePrefix + "/cached"}},
	}
	handler := func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "metricDescriptors") {
			t.Errorf("unexpected metric descriptors list call: %s", r.URL.Path)
		}
		fmt.Fprint(w, "{}")
	}
	collector := newCacheTestCollector(t, handler, cache)

	if err := scrape(collector); err != nil {
		t.Errorf("expected success, got %v", err)
	}
	if cache.lookups != 1 {
		t.Errorf("expected 1 cache lookup, got %d", cache.lookups)
	}
	if cache.stores != 0 {
		t.Errorf("expected no cache stores, got %d", cache.stores)
	}
}

func TestDescriptorCacheStoredOnFullListing(t *testing.T) {
	cache := NewInMemoryDescriptorCache(time.Hour)
	pages := []*monitoring.ListMetricDescriptorsResponse{
		{
			MetricDescriptors: []*monitoring.MetricDescriptor{{Type: testMetricsTypePrefix + "/a"}},
			NextPageToken:     "page-2",
		},
		{
			MetricDescriptors: []*monitoring.MetricDescriptor{{Type: testMetricsTypePrefix + "/b"}},
		},
	}
	collector := newCacheTestCollector(t, descriptorPagesHandler(t, pages), cache)

	if err := scrape(collector); err != nil {
		t.Errorf("expected success, got %v", err)
	}
	cached := cache.Lookup(testMetricsTypePrefix)
	if len(cached) != 2 {
		t.Errorf("expected 2 cached descriptors, got %d", len(cached))
	}
}

func TestDescriptorCacheNotStoredOnPartialListing(t *testing.T) {
	cache := NewInMemoryDescriptorCache(time.Hour)
	pages := []*monitoring.ListMetricDescriptorsResponse{
		{
			MetricDescriptors: []*monitoring.MetricDescriptor{{Type: testMetricsTypePrefix + "/a"}},
			NextPageToken:     "page-2",
		},
		nil,
	}
	collector := newCacheTestCollector(t, descriptorPagesHandler(t, pages), cache)

	if err := scrape(collector); err == nil {
		t.Error("expected scrape error, got nil")
	}
	if cached := cache.Lookup(testMetricsTypePrefix); cached != nil {
		t.Errorf("expected nothing cached after partial listing, got %d descriptors", len(cached))
	}
}
