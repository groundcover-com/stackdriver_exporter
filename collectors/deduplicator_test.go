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
	"os"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricDeduplicator_CheckAndMark(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dedup := NewMetricDeduplicator(logger)

	fqName := "test_metric"
	labelKeys := []string{"label1", "label2"}
	labelValues := []string{"value1", "value2"}
	ts := time.Now()

	// First call should not be a duplicate
	isDuplicate := dedup.CheckAndMark(fqName, labelKeys, labelValues, ts)
	assert.False(t, isDuplicate, "First call should not be a duplicate")

	// Second call with same parameters should be a duplicate
	isDuplicate = dedup.CheckAndMark(fqName, labelKeys, labelValues, ts)
	assert.True(t, isDuplicate, "Second call with same parameters should be a duplicate")

	// Call with different timestamp should not be a duplicate
	ts2 := ts.Add(time.Second)
	isDuplicate = dedup.CheckAndMark(fqName, labelKeys, labelValues, ts2)
	assert.False(t, isDuplicate, "Call with different timestamp should not be a duplicate")

	// Call with different label values should not be a duplicate
	labelValues2 := []string{"value1", "different_value"}
	isDuplicate = dedup.CheckAndMark(fqName, labelKeys, labelValues2, ts)
	assert.False(t, isDuplicate, "Call with different label values should not be a duplicate")

	// Call with different metric name should not be a duplicate
	fqName2 := "different_metric"
	isDuplicate = dedup.CheckAndMark(fqName2, labelKeys, labelValues, ts)
	assert.False(t, isDuplicate, "Call with different metric name should not be a duplicate")
}

func TestMetricDeduplicator_LabelOrdering(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dedup := NewMetricDeduplicator(logger)

	fqName := "test_metric"
	ts := time.Now()

	// Test that label order doesn't matter for deduplication
	labelKeys1 := []string{"b", "a", "c"}
	labelValues1 := []string{"val_b", "val_a", "val_c"}

	labelKeys2 := []string{"a", "b", "c"}
	labelValues2 := []string{"val_a", "val_b", "val_c"}

	// First call
	isDuplicate := dedup.CheckAndMark(fqName, labelKeys1, labelValues1, ts)
	assert.False(t, isDuplicate, "First call should not be a duplicate")

	// Second call with same labels but different order should be a duplicate
	isDuplicate = dedup.CheckAndMark(fqName, labelKeys2, labelValues2, ts)
	assert.True(t, isDuplicate, "Same labels in different order should be detected as duplicate")
}

func TestMetricDeduplicator_EmptyLabels(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dedup := NewMetricDeduplicator(logger)

	fqName := "test_metric"
	ts := time.Now()

	// Test with empty labels
	isDuplicate := dedup.CheckAndMark(fqName, []string{}, []string{}, ts)
	assert.False(t, isDuplicate, "First call with empty labels should not be a duplicate")

	isDuplicate = dedup.CheckAndMark(fqName, []string{}, []string{}, ts)
	assert.True(t, isDuplicate, "Second call with empty labels should be a duplicate")

	// Test with nil labels
	isDuplicate = dedup.CheckAndMark(fqName, nil, nil, ts)
	assert.True(t, isDuplicate, "Call with nil labels should be same as empty labels")
}

func TestMetricDeduplicator_Metrics(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dedup := NewMetricDeduplicator(logger)

	// Register metrics with a test registry
	registry := prometheus.NewRegistry()
	registry.MustRegister(dedup)

	fqName := "test_metric"
	labelKeys := []string{"label1"}
	labelValues := []string{"value1"}
	ts := time.Now()

	// Initial state - no metrics yet
	checksCount := testutil.ToFloat64(dedup.checksTotal)
	duplicatesCount := testutil.ToFloat64(dedup.duplicatesTotal)
	uniqueCount := testutil.ToFloat64(dedup.uniqueMetricsGauge)

	assert.Equal(t, float64(0), checksCount, "Initial checks count should be 0")
	assert.Equal(t, float64(0), duplicatesCount, "Initial duplicates count should be 0")
	assert.Equal(t, float64(0), uniqueCount, "Initial unique count should be 0")

	// First call - should increment checks and unique metrics
	dedup.CheckAndMark(fqName, labelKeys, labelValues, ts)

	checksCount = testutil.ToFloat64(dedup.checksTotal)
	duplicatesCount = testutil.ToFloat64(dedup.duplicatesTotal)
	uniqueCount = testutil.ToFloat64(dedup.uniqueMetricsGauge)

	assert.Equal(t, float64(1), checksCount, "Checks count should be 1 after first call")
	assert.Equal(t, float64(0), duplicatesCount, "Duplicates count should still be 0")
	assert.Equal(t, float64(1), uniqueCount, "Unique count should be 1")

	// Second call - should increment checks and duplicates
	dedup.CheckAndMark(fqName, labelKeys, labelValues, ts)

	checksCount = testutil.ToFloat64(dedup.checksTotal)
	duplicatesCount = testutil.ToFloat64(dedup.duplicatesTotal)
	uniqueCount = testutil.ToFloat64(dedup.uniqueMetricsGauge)

	assert.Equal(t, float64(2), checksCount, "Checks count should be 2 after second call")
	assert.Equal(t, float64(1), duplicatesCount, "Duplicates count should be 1")
	assert.Equal(t, float64(1), uniqueCount, "Unique count should still be 1")

	// Third call with different timestamp - should increment checks and unique metrics
	ts2 := ts.Add(time.Second)
	dedup.CheckAndMark(fqName, labelKeys, labelValues, ts2)

	checksCount = testutil.ToFloat64(dedup.checksTotal)
	duplicatesCount = testutil.ToFloat64(dedup.duplicatesTotal)
	uniqueCount = testutil.ToFloat64(dedup.uniqueMetricsGauge)

	assert.Equal(t, float64(3), checksCount, "Checks count should be 3 after third call")
	assert.Equal(t, float64(1), duplicatesCount, "Duplicates count should still be 1")
	assert.Equal(t, float64(2), uniqueCount, "Unique count should be 2")
}

func TestMetricDeduplicator_ConcurrentAccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dedup := NewMetricDeduplicator(logger)

	const numGoroutines = 10
	const numCallsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Launch multiple goroutines that call CheckAndMark concurrently
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()

			fqName := "test_metric"
			labelKeys := []string{"goroutine_id"}
			labelValues := []string{string(rune('0' + goroutineID))} // Convert int to string
			ts := time.Now()

			for j := 0; j < numCallsPerGoroutine; j++ {
				// Each goroutine calls with the same parameters multiple times
				// First call should not be duplicate, subsequent calls should be
				isDuplicate := dedup.CheckAndMark(fqName, labelKeys, labelValues, ts)

				if j == 0 {
					assert.False(t, isDuplicate, "First call from goroutine %d should not be duplicate", goroutineID)
				} else {
					assert.True(t, isDuplicate, "Call %d from goroutine %d should be duplicate", j, goroutineID)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify final metrics
	checksCount := testutil.ToFloat64(dedup.checksTotal)
	duplicatesCount := testutil.ToFloat64(dedup.duplicatesTotal)
	uniqueCount := testutil.ToFloat64(dedup.uniqueMetricsGauge)

	expectedChecks := float64(numGoroutines * numCallsPerGoroutine)
	expectedDuplicates := float64(numGoroutines * (numCallsPerGoroutine - 1)) // All calls except first per goroutine
	expectedUnique := float64(numGoroutines)                                  // One unique metric per goroutine

	assert.Equal(t, expectedChecks, checksCount, "Total checks should match expected")
	assert.Equal(t, expectedDuplicates, duplicatesCount, "Total duplicates should match expected")
	assert.Equal(t, expectedUnique, uniqueCount, "Total unique metrics should match expected")
}

func TestMetricDeduplicator_PrometheusIntegration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dedup := NewMetricDeduplicator(logger)

	// Test Describe method
	ch := make(chan *prometheus.Desc, 10)
	dedup.Describe(ch)
	close(ch)

	descriptions := make([]*prometheus.Desc, 0)
	for desc := range ch {
		descriptions = append(descriptions, desc)
	}

	require.Len(t, descriptions, 3, "Should have exactly 3 metric descriptions")

	// Test Collect method
	metricCh := make(chan prometheus.Metric, 10)
	dedup.Collect(metricCh)
	close(metricCh)

	metrics := make([]prometheus.Metric, 0)
	for metric := range metricCh {
		metrics = append(metrics, metric)
	}

	require.Len(t, metrics, 3, "Should have exactly 3 metrics")
}

func TestMetricDeduplicator_SliceReuse(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dedup := NewMetricDeduplicator(logger)

	fqName := "test_metric"
	ts := time.Now()

	// Test with different numbers of labels to ensure slice reuse works correctly
	testCases := []struct {
		name        string
		labelKeys   []string
		labelValues []string
	}{
		{
			name:        "small_labels",
			labelKeys:   []string{"a", "b"},
			labelValues: []string{"1", "2"},
		},
		{
			name:        "medium_labels",
			labelKeys:   []string{"a", "b", "c", "d", "e"},
			labelValues: []string{"1", "2", "3", "4", "5"},
		},
		{
			name:        "large_labels",
			labelKeys:   []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
			labelValues: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"},
		},
		{
			name:        "back_to_small",
			labelKeys:   []string{"x", "y"},
			labelValues: []string{"1", "2"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// First call should not be duplicate
			isDuplicate := dedup.CheckAndMark(fqName+"_"+tc.name, tc.labelKeys, tc.labelValues, ts)
			assert.False(t, isDuplicate, "First call should not be duplicate")

			// Second call should be duplicate
			isDuplicate = dedup.CheckAndMark(fqName+"_"+tc.name, tc.labelKeys, tc.labelValues, ts)
			assert.True(t, isDuplicate, "Second call should be duplicate")
		})
	}

	// Verify that the slice capacity grew appropriately and is being reused
	// The slice should have grown to accommodate the largest test case
	assert.GreaterOrEqual(t, cap(dedup.indicesSlice), 10, "Slice capacity should have grown to accommodate largest test case")
}

func TestMetricDeduplicator_Reset(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dedup := NewMetricDeduplicator(logger)

	fqName := "test_metric"
	labelKeys := []string{"label1", "label2"}
	labelValues := []string{"value1", "value2"}
	ts := time.Now()

	// First iteration: Add some metrics
	isDuplicate := dedup.CheckAndMark(fqName, labelKeys, labelValues, ts)
	assert.False(t, isDuplicate, "First call should not be a duplicate")

	// Verify it's now marked as seen
	isDuplicate = dedup.CheckAndMark(fqName, labelKeys, labelValues, ts)
	assert.True(t, isDuplicate, "Second call should be a duplicate")

	// Add another metric with different timestamp
	ts2 := ts.Add(time.Second)
	isDuplicate = dedup.CheckAndMark(fqName, labelKeys, labelValues, ts2)
	assert.False(t, isDuplicate, "Different timestamp should not be a duplicate")

	// Verify the unique metrics gauge shows we have 2 unique signatures
	uniqueCount := testutil.ToFloat64(dedup.uniqueMetricsGauge)
	assert.Equal(t, float64(2), uniqueCount, "Should have 2 unique metrics before reset")

	// Reset the deduplicator (simulating new iteration)
	dedup.Reset()

	// Verify the unique metrics gauge is reset to 0
	uniqueCount = testutil.ToFloat64(dedup.uniqueMetricsGauge)
	assert.Equal(t, float64(0), uniqueCount, "Should have 0 unique metrics after reset")

	// After reset, the same metrics should not be considered duplicates
	isDuplicate = dedup.CheckAndMark(fqName, labelKeys, labelValues, ts)
	assert.False(t, isDuplicate, "After reset, previously seen metric should not be a duplicate")

	isDuplicate = dedup.CheckAndMark(fqName, labelKeys, labelValues, ts2)
	assert.False(t, isDuplicate, "After reset, previously seen metric with different timestamp should not be a duplicate")

	// But within the same iteration (after reset), duplicates should still be detected
	isDuplicate = dedup.CheckAndMark(fqName, labelKeys, labelValues, ts)
	assert.True(t, isDuplicate, "Within same iteration after reset, duplicate should be detected")

	// Verify unique count is updated correctly after reset
	uniqueCount = testutil.ToFloat64(dedup.uniqueMetricsGauge)
	assert.Equal(t, float64(2), uniqueCount, "Should have 2 unique metrics after adding them back")
}

func TestMetricDeduplicator_ResetBetweenIterations(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dedup := NewMetricDeduplicator(logger)

	// Simulate multiple scrape iterations with the same metrics
	fqName := "test_metric"
	labelKeys := []string{"service", "method"}
	labelValues := []string{"api", "get"}
	
	// Iteration 1
	ts1 := time.Now()
	
	// First metric in iteration 1
	isDuplicate := dedup.CheckAndMark(fqName, labelKeys, labelValues, ts1)
	assert.False(t, isDuplicate, "Iteration 1: First occurrence should not be duplicate")
	
	// Same metric again in iteration 1 - should be duplicate
	isDuplicate = dedup.CheckAndMark(fqName, labelKeys, labelValues, ts1)
	assert.True(t, isDuplicate, "Iteration 1: Same metric should be duplicate")

	// Check metrics before reset
	checksCount1 := testutil.ToFloat64(dedup.checksTotal)
	duplicatesCount1 := testutil.ToFloat64(dedup.duplicatesTotal)
	uniqueCount1 := testutil.ToFloat64(dedup.uniqueMetricsGauge)
	
	assert.Equal(t, float64(2), checksCount1, "Should have 2 checks in iteration 1")
	assert.Equal(t, float64(1), duplicatesCount1, "Should have 1 duplicate in iteration 1")
	assert.Equal(t, float64(1), uniqueCount1, "Should have 1 unique metric in iteration 1")

	// Reset for iteration 2 (simulating what happens in monitoring collector)
	dedup.Reset()

	// Iteration 2 - same metrics but different time
	ts2 := ts1.Add(5 * time.Minute) // New scrape interval
	
	// Same metric in iteration 2 - should NOT be duplicate (clean state)
	isDuplicate = dedup.CheckAndMark(fqName, labelKeys, labelValues, ts2)
	assert.False(t, isDuplicate, "Iteration 2: Same metric with different timestamp should not be duplicate after reset")
	
	// Same metric again in iteration 2 - should be duplicate within this iteration
	isDuplicate = dedup.CheckAndMark(fqName, labelKeys, labelValues, ts2)
	assert.True(t, isDuplicate, "Iteration 2: Same metric should be duplicate within iteration")

	// Check final metrics
	checksCount2 := testutil.ToFloat64(dedup.checksTotal)
	duplicatesCount2 := testutil.ToFloat64(dedup.duplicatesTotal)
	uniqueCount2 := testutil.ToFloat64(dedup.uniqueMetricsGauge)
	
	assert.Equal(t, float64(4), checksCount2, "Should have 4 total checks across both iterations")
	assert.Equal(t, float64(2), duplicatesCount2, "Should have 2 total duplicates across both iterations")
	assert.Equal(t, float64(1), uniqueCount2, "Should have 1 unique metric in current iteration after reset")

	// Iteration 3 - test with multiple different metrics
	dedup.Reset()
	
	ts3 := ts2.Add(5 * time.Minute)
	
	// Add multiple different metrics in iteration 3
	metrics := []struct {
		name   string
		keys   []string
		values []string
	}{
		{"metric_a", []string{"service"}, []string{"api"}},
		{"metric_b", []string{"service"}, []string{"db"}},
		{"metric_c", []string{"service", "region"}, []string{"cache", "us-east"}},
	}
	
	for _, metric := range metrics {
		isDuplicate = dedup.CheckAndMark(metric.name, metric.keys, metric.values, ts3)
		assert.False(t, isDuplicate, "Iteration 3: New metric %s should not be duplicate", metric.name)
		
		// Same metric again - should be duplicate
		isDuplicate = dedup.CheckAndMark(metric.name, metric.keys, metric.values, ts3)
		assert.True(t, isDuplicate, "Iteration 3: Repeated metric %s should be duplicate", metric.name)
	}
	
	// Final verification
	uniqueCount3 := testutil.ToFloat64(dedup.uniqueMetricsGauge)
	assert.Equal(t, float64(3), uniqueCount3, "Should have 3 unique metrics in iteration 3")
}
