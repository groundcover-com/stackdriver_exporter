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
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/prometheus-community/stackdriver_exporter/hash"
	"github.com/prometheus/client_golang/prometheus"
)

// MetricDeduplicator helps prevent sending duplicate metrics to Prometheus.
// It tracks signatures of metrics that have already been sent.
type MetricDeduplicator struct {
	mu             sync.Mutex // Protects all fields below
	sentSignatures map[uint64]struct{}
	logger         *slog.Logger

	// Prometheus metrics
	duplicatesTotal    prometheus.Counter
	checksTotal        prometheus.Counter
	uniqueMetricsGauge prometheus.Gauge
}

// NewMetricDeduplicator creates a new MetricDeduplicator.
func NewMetricDeduplicator(logger *slog.Logger, projectID string) *MetricDeduplicator {
	if logger == nil {
		logger = slog.Default()
	}

	duplicatesTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "stackdriver",
		Subsystem: "deduplicator",
		Name:      "duplicates_total",
		Help:      "Total number of duplicate metrics detected and dropped.",
	}, []string{"project_id"}).WithLabelValues(projectID)

	checksTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "stackdriver",
		Subsystem: "deduplicator",
		Name:      "checks_total",
		Help:      "Total number of deduplication checks performed.",
	}, []string{"project_id"}).WithLabelValues(projectID)

	uniqueMetricsGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "stackdriver",
		Subsystem: "deduplicator",
		Name:      "unique_metrics",
		Help:      "Current number of unique metrics being tracked.",
	}, []string{"project_id"}).WithLabelValues(projectID)

	return &MetricDeduplicator{
		sentSignatures:     make(map[uint64]struct{}),
		logger:             logger.With("component", "deduplicator"),
		duplicatesTotal:    duplicatesTotal,
		checksTotal:        checksTotal,
		uniqueMetricsGauge: uniqueMetricsGauge,
	}
}

// CheckAndMark checks if a metric signature has been seen before.
// If not seen, it marks it as seen and returns false (not a duplicate).
// If seen before, returns true (duplicate detected).
// We keep the first occurrence and drop all subsequent ones.
// This method is thread-safe.
func (d *MetricDeduplicator) CheckAndMark(name string, labelKeys, labelValues []string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.checksTotal.Inc()

	signature := d.hashLabels(name, labelKeys, labelValues)

	if _, exists := d.sentSignatures[signature]; exists {
		d.duplicatesTotal.Inc()
		return true // Duplicate detected - drop it
	}

	d.sentSignatures[signature] = struct{}{} // Mark as seen
	d.uniqueMetricsGauge.Set(float64(len(d.sentSignatures)))

	return false // Not a duplicate
}

func (d *MetricDeduplicator) RevertMark(fqName string, labelKeys, labelValues []string, ts time.Time) {
	signature := d.hashLabels(fqName, labelKeys, labelValues)
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.sentSignatures, signature)
	d.uniqueMetricsGauge.Set(float64(len(d.sentSignatures)))
}

// hashLabels calculates a hash based on FQName and sorted labels.
func (d *MetricDeduplicator) hashLabels(fqName string, labelKeys, labelValues []string) uint64 {
	h := hash.New()
	h = hash.Add(h, fqName)
	h = hash.AddByte(h, hash.SeparatorByte)

	if len(labelKeys) > 0 {
		// Create indices [0, 1, 2, ...]
		indices := make([]int, len(labelKeys))
		for i := range indices {
			indices[i] = i
		}

		// Sort indices by their label keys
		sort.Slice(indices, func(i, j int) bool {
			return labelKeys[indices[i]] < labelKeys[indices[j]]
		})

		// Hash labels in sorted order
		for _, idx := range indices {
			h = hash.Add(h, labelKeys[idx])
			h = hash.AddByte(h, hash.SeparatorByte)
			if idx < len(labelValues) {
				h = hash.Add(h, labelValues[idx])
			}
			h = hash.AddByte(h, hash.SeparatorByte)
		}
	}

	return h
}

// Describe implements prometheus.Collector interface.
func (d *MetricDeduplicator) Describe(ch chan<- *prometheus.Desc) {
	d.duplicatesTotal.Describe(ch)
	d.checksTotal.Describe(ch)
	d.uniqueMetricsGauge.Describe(ch)
}

// Collect implements prometheus.Collector interface.
func (d *MetricDeduplicator) Collect(ch chan<- prometheus.Metric) {
	d.duplicatesTotal.Collect(ch)
	d.checksTotal.Collect(ch)
	d.uniqueMetricsGauge.Collect(ch)
}

func (d *MetricDeduplicator) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.sentSignatures = make(map[uint64]struct{})
	d.uniqueMetricsGauge.Set(0)
}
