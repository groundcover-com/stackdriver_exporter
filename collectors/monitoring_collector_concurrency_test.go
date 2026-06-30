// Copyright 2020 The Prometheus Authors
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
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/monitoring/v3"
	"google.golang.org/api/option"
)

type noopCounterStore struct{}

func (noopCounterStore) Increment(*monitoring.MetricDescriptor, *ConstMetric) {}
func (noopCounterStore) ListMetrics(string) []*ConstMetric                    { return nil }

type noopHistogramStore struct{}

func (noopHistogramStore) Increment(*monitoring.MetricDescriptor, *HistogramMetric) {}
func (noopHistogramStore) ListMetrics(string) []*HistogramMetric                    { return nil }

// TestReportMonitoringMetrics_TimeSeriesListConcurrency verifies that the inner
// projects.timeSeries.list fan-out is bounded by TimeSeriesListConcurrency.
func TestReportMonitoringMetrics_TimeSeriesListConcurrency(t *testing.T) {
	const numDescriptors = 8
	const limit = 2

	var inFlight, maxInFlight, total int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/metricDescriptors"):
			var sb strings.Builder
			sb.WriteString(`{"metricDescriptors":[`)
			for i := 0; i < numDescriptors; i++ {
				if i > 0 {
					sb.WriteString(",")
				}
				fmt.Fprintf(&sb, `{"type":"compute.googleapis.com/m%d","metricKind":"GAUGE","valueType":"DOUBLE"}`, i)
			}
			sb.WriteString(`]}`)
			w.Write([]byte(sb.String()))
		case strings.HasSuffix(r.URL.Path, "/timeSeries"):
			atomic.AddInt64(&total, 1)
			cur := atomic.AddInt64(&inFlight, 1)
			for {
				old := atomic.LoadInt64(&maxInFlight)
				if cur <= old || atomic.CompareAndSwapInt64(&maxInFlight, old, cur) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			atomic.AddInt64(&inFlight, -1)
			w.Write([]byte(`{}`))
		default:
			w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()

	svc, err := monitoring.NewService(context.Background(),
		option.WithEndpoint(server.URL),
		option.WithoutAuthentication(),
		option.WithHTTPClient(server.Client()))
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	collector, err := NewMonitoringCollector("test-project", svc, MonitoringCollectorOptions{
		MetricTypePrefixes:        []string{"compute.googleapis.com"},
		RequestInterval:           time.Minute,
		TimeSeriesListConcurrency: limit,
	}, logger, noopCounterStore{}, noopHistogramStore{})
	require.NoError(t, err)

	ch := make(chan prometheus.Metric, 1024)
	var drain sync.WaitGroup
	drain.Add(1)
	go func() {
		defer drain.Done()
		for range ch {
		}
	}()

	require.NoError(t, collector.reportMonitoringMetrics(ch, time.Now()))
	close(ch)
	drain.Wait()

	require.Equal(t, int64(numDescriptors), atomic.LoadInt64(&total), "every descriptor should be queried")
	require.LessOrEqual(t, atomic.LoadInt64(&maxInFlight), int64(limit), "concurrent timeSeries.list calls must not exceed the limit")
}
