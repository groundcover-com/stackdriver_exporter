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
func NewMetricDeduplicator(logger *slog.Logger) *MetricDeduplicator {
	if logger == nil {
		logger = slog.Default()
	}

	duplicatesTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "stackdriver",
		Subsystem: "deduplicator",
		Name:      "duplicates_total",
		Help:      "Total number of duplicate metrics detected and dropped.",
	})

	checksTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "stackdriver",
		Subsystem: "deduplicator",
		Name:      "checks_total",
		Help:      "Total number of deduplication checks performed.",
	})

	uniqueMetricsGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "stackdriver",
		Subsystem: "deduplicator",
		Name:      "unique_metrics",
		Help:      "Current number of unique metrics being tracked.",
	})

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
// If seen before, it returns true (duplicate detected).
// This method is thread-safe.
func (d *MetricDeduplicator) CheckAndMark(name string, labelKeys, labelValues []string, ts time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.checksTotal.Inc()

	signature := d.hashLabelsTimestamp(name, labelKeys, labelValues, ts)

	if _, exists := d.sentSignatures[signature]; exists {
		d.duplicatesTotal.Inc()

		// Log duplicate detection at debug level only
		d.logger.Debug("duplicate metric detected",
			"metric", name,
			"timestamp", ts.Format(time.RFC3339Nano),
			"signature", signature,
		)

		return true // Duplicate detected
	}

	d.sentSignatures[signature] = struct{}{} // Mark as seen
	d.uniqueMetricsGauge.Set(float64(len(d.sentSignatures)))

	return false // Not a duplicate
}

func (d *MetricDeduplicator) RevertMark(fqName string, labelKeys, labelValues []string, ts time.Time) {
	signature := d.hashLabelsTimestamp(fqName, labelKeys, labelValues, ts)
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.sentSignatures, signature)
	d.uniqueMetricsGauge.Set(float64(len(d.sentSignatures)))
}

// hashLabelsTimestamp calculates a hash based on FQName, sorted labels, and timestamp.
func (d *MetricDeduplicator) hashLabelsTimestamp(fqName string, labelKeys, labelValues []string, ts time.Time) uint64 {
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

	// Add timestamp
	tsNano := ts.UnixNano()
	h = hash.AddUint64(h, uint64(tsNano))

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
