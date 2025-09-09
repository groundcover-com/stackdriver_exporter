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
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/api/googleapi"
)

func TestMonitoringCollector_AddSystemLabels(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create a minimal MonitoringCollector for testing
	collector := &MonitoringCollector{
		logger: logger,
	}

	tests := []struct {
		name                string
		systemLabelsJSON    string
		initialLabelKeys    []string
		initialLabelValues  []string
		expectedLabelKeys   []string
		expectedLabelValues []string
	}{
		{
			name:                "empty_system_labels",
			systemLabelsJSON:    `{}`,
			initialLabelKeys:    []string{"existing_key"},
			initialLabelValues:  []string{"existing_value"},
			expectedLabelKeys:   []string{"existing_key"},
			expectedLabelValues: []string{"existing_value"},
		},
		{
			name:                "single_system_label",
			systemLabelsJSON:    `{"system_key": "system_value"}`,
			initialLabelKeys:    []string{"existing_key"},
			initialLabelValues:  []string{"existing_value"},
			expectedLabelKeys:   []string{"existing_key", "system_key"},
			expectedLabelValues: []string{"existing_value", "system_value"},
		},
		{
			name:                "multiple_system_labels",
			systemLabelsJSON:    `{"region": "us-central1", "zone": "us-central1-a", "instance_id": "12345"}`,
			initialLabelKeys:    []string{"metric_type"},
			initialLabelValues:  []string{"cpu_usage"},
			expectedLabelKeys:   []string{"metric_type", "region", "zone", "instance_id"},
			expectedLabelValues: []string{"cpu_usage", "us-central1", "us-central1-a", "12345"},
		},
		{
			name:                "duplicate_key_ignored",
			systemLabelsJSON:    `{"existing_key": "system_value", "new_key": "new_value"}`,
			initialLabelKeys:    []string{"existing_key"},
			initialLabelValues:  []string{"existing_value"},
			expectedLabelKeys:   []string{"existing_key", "new_key"},
			expectedLabelValues: []string{"existing_value", "new_value"},
		},
		{
			name:                "empty_initial_labels",
			systemLabelsJSON:    `{"cluster": "prod-cluster", "namespace": "default"}`,
			initialLabelKeys:    []string{},
			initialLabelValues:  []string{},
			expectedLabelKeys:   []string{"cluster", "namespace"},
			expectedLabelValues: []string{"prod-cluster", "default"},
		},
		{
			name:                "numeric_values",
			systemLabelsJSON:    `{"port": "8080", "replicas": "3", "version": "1.2.3"}`,
			initialLabelKeys:    []string{"app"},
			initialLabelValues:  []string{"myapp"},
			expectedLabelKeys:   []string{"app", "port", "replicas", "version"},
			expectedLabelValues: []string{"myapp", "8080", "3", "1.2.3"},
		},
		{
			name:                "boolean_values",
			systemLabelsJSON:    `{"enabled": "true", "debug": "false"}`,
			initialLabelKeys:    []string{"service"},
			initialLabelValues:  []string{"api"},
			expectedLabelKeys:   []string{"service", "enabled", "debug"},
			expectedLabelValues: []string{"api", "true", "false"},
		},
		{
			name:                "special_characters",
			systemLabelsJSON:    `{"env-type": "prod", "app_name": "my-service", "version.tag": "v1.0"}`,
			initialLabelKeys:    []string{},
			initialLabelValues:  []string{},
			expectedLabelKeys:   []string{"env-type", "app_name", "version.tag"},
			expectedLabelValues: []string{"prod", "my-service", "v1.0"},
		},
		{
			name:                "nested_json_as_string",
			systemLabelsJSON:    `{"config": "{\"key\": \"value\"}", "simple": "text"}`,
			initialLabelKeys:    []string{},
			initialLabelValues:  []string{},
			expectedLabelKeys:   []string{"config", "simple"},
			expectedLabelValues: []string{"{\"key\": \"value\"}", "text"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create copies of the initial slices to avoid modifying the test data
			labelKeys := make([]string, len(tt.initialLabelKeys))
			labelValues := make([]string, len(tt.initialLabelValues))
			copy(labelKeys, tt.initialLabelKeys)
			copy(labelValues, tt.initialLabelValues)

			// Create googleapi.RawMessage from JSON string
			rawMessage := googleapi.RawMessage(tt.systemLabelsJSON)

			// Call the method under test
			collector.addSystemLabels(rawMessage, &labelKeys, &labelValues)

			// Verify the results
			assert.Equal(t, tt.expectedLabelKeys, labelKeys, "Label keys should match expected")
			assert.Equal(t, tt.expectedLabelValues, labelValues, "Label values should match expected")
			assert.Equal(t, len(labelKeys), len(labelValues), "Label keys and values should have the same length")
		})
	}
}

func TestMonitoringCollector_AddSystemLabels_InvalidJSON(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	collector := &MonitoringCollector{
		logger: logger,
	}

	tests := []struct {
		name             string
		systemLabelsJSON string
		initialKeys      []string
		initialValues    []string
		expectChange     bool // Whether we expect the labels to change
	}{
		{
			name:             "invalid_json",
			systemLabelsJSON: `{invalid json}`,
			initialKeys:      []string{"existing"},
			initialValues:    []string{"value"},
			expectChange:     false, // Invalid JSON should not change labels
		},
		{
			name:             "empty_string",
			systemLabelsJSON: ``,
			initialKeys:      []string{"existing"},
			initialValues:    []string{"value"},
			expectChange:     false, // Empty string should not change labels
		},
		{
			name:             "null_json",
			systemLabelsJSON: `null`,
			initialKeys:      []string{"existing"},
			initialValues:    []string{"value"},
			expectChange:     false, // Early exit for null JSON
		},
		{
			name:             "array_json",
			systemLabelsJSON: `["not", "an", "object"]`,
			initialKeys:      []string{"existing"},
			initialValues:    []string{"value"},
			expectChange:     false, // Early exit for non-object JSON
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labelKeys := make([]string, len(tt.initialKeys))
			labelValues := make([]string, len(tt.initialValues))
			copy(labelKeys, tt.initialKeys)
			copy(labelValues, tt.initialValues)

			originalKeyCount := len(labelKeys)
			originalValueCount := len(labelValues)

			rawMessage := googleapi.RawMessage(tt.systemLabelsJSON)

			// Should not panic
			assert.NotPanics(t, func() {
				collector.addSystemLabels(rawMessage, &labelKeys, &labelValues)
			})

			if tt.expectChange {
				// For cases where we expect changes
				assert.GreaterOrEqual(t, len(labelKeys), originalKeyCount, "Label keys should have at least original count")
				assert.GreaterOrEqual(t, len(labelValues), originalValueCount, "Label values should have at least original count")
				assert.Equal(t, len(labelKeys), len(labelValues), "Label keys and values should have the same length")
			} else {
				// For cases with early exit, labels should remain unchanged
				assert.Equal(t, tt.initialKeys, labelKeys, "Label keys should remain unchanged with early exit")
				assert.Equal(t, tt.initialValues, labelValues, "Label values should remain unchanged with early exit")
			}
		})
	}
}

func TestMonitoringCollector_AddSystemLabels_EdgeCases(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	collector := &MonitoringCollector{
		logger: logger,
	}

	t.Run("nil_slices", func(t *testing.T) {
		var labelKeys *[]string
		var labelValues *[]string

		rawMessage := googleapi.RawMessage(`{"key": "value"}`)

		// This will panic because the current implementation doesn't handle nil pointers
		// This test documents the current behavior - in practice, this should never happen
		// as the calling code always passes valid slice pointers
		assert.Panics(t, func() {
			collector.addSystemLabels(rawMessage, labelKeys, labelValues)
		}, "addSystemLabels should panic with nil slice pointers")
	})

	t.Run("empty_values", func(t *testing.T) {
		labelKeys := []string{"existing"}
		labelValues := []string{"value"}

		rawMessage := googleapi.RawMessage(`{"empty": "", "whitespace": "   ", "zero": "0"}`)

		collector.addSystemLabels(rawMessage, &labelKeys, &labelValues)

		expectedKeys := []string{"existing", "empty", "whitespace", "zero"}
		expectedValues := []string{"value", "", "   ", "0"}

		assert.Equal(t, expectedKeys, labelKeys)
		assert.Equal(t, expectedValues, labelValues)
	})

	t.Run("unicode_characters", func(t *testing.T) {
		labelKeys := []string{}
		labelValues := []string{}

		rawMessage := googleapi.RawMessage(`{"emoji": "🚀", "chinese": "你好", "arabic": "مرحبا"}`)

		collector.addSystemLabels(rawMessage, &labelKeys, &labelValues)

		expectedKeys := []string{"emoji", "chinese", "arabic"}
		expectedValues := []string{"🚀", "你好", "مرحبا"}

		assert.Equal(t, expectedKeys, labelKeys)
		assert.Equal(t, expectedValues, labelValues)
	})

	t.Run("very_long_values", func(t *testing.T) {
		labelKeys := []string{}
		labelValues := []string{}

		longValue := string(make([]byte, 1000)) // Create a very long string
		for i := range longValue {
			longValue = longValue[:i] + "a" + longValue[i+1:]
		}

		rawMessage := googleapi.RawMessage(`{"long_key": "` + longValue + `"}`)

		collector.addSystemLabels(rawMessage, &labelKeys, &labelValues)

		expectedKeys := []string{"long_key"}
		expectedValues := []string{longValue}

		assert.Equal(t, expectedKeys, labelKeys)
		assert.Equal(t, expectedValues, labelValues)
	})
}

func TestMonitoringCollector_KeyExists(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	collector := &MonitoringCollector{
		logger: logger,
	}

	tests := []struct {
		name      string
		labelKeys []string
		searchKey string
		expected  bool
	}{
		{
			name:      "key_exists",
			labelKeys: []string{"key1", "key2", "key3"},
			searchKey: "key2",
			expected:  true,
		},
		{
			name:      "key_not_exists",
			labelKeys: []string{"key1", "key2", "key3"},
			searchKey: "key4",
			expected:  false,
		},
		{
			name:      "empty_slice",
			labelKeys: []string{},
			searchKey: "key1",
			expected:  false,
		},
		{
			name:      "empty_search_key",
			labelKeys: []string{"key1", "", "key3"},
			searchKey: "",
			expected:  true,
		},
		{
			name:      "case_sensitive",
			labelKeys: []string{"Key1", "KEY2", "key3"},
			searchKey: "key1",
			expected:  false,
		},
		{
			name:      "duplicate_keys",
			labelKeys: []string{"key1", "key2", "key1"},
			searchKey: "key1",
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collector.keyExists(tt.labelKeys, tt.searchKey)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMonitoringCollector_AllLabelSources_Integration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	collector := &MonitoringCollector{
		logger: logger,
	}

	tests := []struct {
		name                string
		unitLabel           string
		metricLabels        map[string]string
		resourceLabels      map[string]string
		userLabels          map[string]string
		systemLabelsJSON    string
		expectedLabelKeys   []string
		expectedLabelValues []string
	}{
		{
			name:      "all_label_sources",
			unitLabel: "bytes",
			metricLabels: map[string]string{
				"instance_name": "web-server-1",
				"method":        "GET",
			},
			resourceLabels: map[string]string{
				"project_id": "my-project",
				"zone":       "us-central1-a",
			},
			userLabels: map[string]string{
				"environment": "production",
				"team":        "backend",
			},
			systemLabelsJSON: `{
				"cluster": "prod-cluster",
				"namespace": "default"
			}`,
			expectedLabelKeys: []string{
				"unit",
				"instance_name", "method",
				"project_id", "zone",
				"environment", "team",
				"cluster", "namespace",
			},
			expectedLabelValues: []string{
				"bytes",
				"web-server-1", "GET",
				"my-project", "us-central1-a",
				"production", "backend",
				"prod-cluster", "default",
			},
		},
		{
			name:      "overlapping_labels_first_wins",
			unitLabel: "count",
			metricLabels: map[string]string{
				"region": "us-west1", // This should win over resource label
				"app":    "frontend",
			},
			resourceLabels: map[string]string{
				"region":     "us-east1", // Should be ignored due to duplicate
				"project_id": "test-project",
			},
			userLabels: map[string]string{
				"app":     "backend", // Should be ignored due to duplicate
				"version": "v1.2.3",
			},
			systemLabelsJSON: `{
				"version": "v2.0.0",
				"cluster": "test-cluster"
			}`,
			expectedLabelKeys: []string{
				"unit",
				"region", "app",
				"project_id",
				"version",
				"cluster",
			},
			expectedLabelValues: []string{
				"count",
				"us-west1", "frontend", // Metric labels win
				"test-project",
				"v1.2.3", // User label wins over system label
				"test-cluster",
			},
		},
		{
			name:             "empty_sources",
			unitLabel:        "",
			metricLabels:     map[string]string{},
			resourceLabels:   map[string]string{},
			userLabels:       map[string]string{},
			systemLabelsJSON: `{}`,
			expectedLabelKeys: []string{
				"unit",
			},
			expectedLabelValues: []string{
				"",
			},
		},
		{
			name:           "only_system_labels",
			unitLabel:      "seconds",
			metricLabels:   map[string]string{},
			resourceLabels: map[string]string{},
			userLabels:     map[string]string{},
			systemLabelsJSON: `{
				"pod": "my-pod-12345",
				"container": "app-container",
				"node": "node-1"
			}`,
			expectedLabelKeys: []string{
				"unit",
				"pod", "container", "node",
			},
			expectedLabelValues: []string{
				"seconds",
				"my-pod-12345", "app-container", "node-1",
			},
		},
		{
			name:      "special_characters_and_unicode",
			unitLabel: "requests/sec",
			metricLabels: map[string]string{
				"service-name": "my-service",
				"env_type":     "prod",
			},
			resourceLabels: map[string]string{
				"resource.type": "gce_instance",
			},
			userLabels: map[string]string{
				"owner":       "team-α",
				"cost.center": "engineering",
			},
			systemLabelsJSON: `{
				"k8s.pod.name": "app-pod-🚀",
				"version.tag": "v1.0-beta"
			}`,
			expectedLabelKeys: []string{
				"unit",
				"service-name", "env_type",
				"resource.type",
				"owner", "cost.center",
				"k8s.pod.name", "version.tag",
			},
			expectedLabelValues: []string{
				"requests/sec",
				"my-service", "prod",
				"gce_instance",
				"team-α", "engineering",
				"app-pod-🚀", "v1.0-beta",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Start with unit label (this is always first)
			labelKeys := []string{"unit"}
			labelValues := []string{tt.unitLabel}

			// Add metric labels
			for key, value := range tt.metricLabels {
				if !collector.keyExists(labelKeys, key) {
					labelKeys = append(labelKeys, key)
					labelValues = append(labelValues, value)
				}
			}

			// Add resource labels
			for key, value := range tt.resourceLabels {
				if !collector.keyExists(labelKeys, key) {
					labelKeys = append(labelKeys, key)
					labelValues = append(labelValues, value)
				}
			}

			// Add user labels
			for key, value := range tt.userLabels {
				if !collector.keyExists(labelKeys, key) {
					labelKeys = append(labelKeys, key)
					labelValues = append(labelValues, value)
				}
			}

			// Add system labels
			rawMessage := googleapi.RawMessage(tt.systemLabelsJSON)
			collector.addSystemLabels(rawMessage, &labelKeys, &labelValues)

			sort.Strings(tt.expectedLabelValues)
			sort.Strings(labelValues)
			sort.Strings(tt.expectedLabelKeys)
			sort.Strings(labelKeys)
			// Verify the results
			assert.Equal(t, tt.expectedLabelKeys, labelKeys, "Label keys should match expected")
			assert.Equal(t, tt.expectedLabelValues, labelValues, "Label values should match expected")
			assert.Equal(t, len(labelKeys), len(labelValues), "Label keys and values should have the same length")

			// Verify no duplicate keys exist
			keySet := make(map[string]bool)
			for _, key := range labelKeys {
				assert.False(t, keySet[key], "Duplicate key found: %s", key)
				keySet[key] = true
			}
		})
	}
}

func BenchmarkMonitoringCollector_AddSystemLabels(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	collector := &MonitoringCollector{
		logger: logger,
	}

	// Test with realistic system labels
	systemLabelsJSON := `{
		"region": "us-central1",
		"zone": "us-central1-a", 
		"instance_id": "1234567890",
		"cluster": "prod-cluster",
		"namespace": "default",
		"pod": "my-app-12345",
		"container": "my-container"
	}`

	rawMessage := googleapi.RawMessage(systemLabelsJSON)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		labelKeys := []string{"metric_type", "unit"}
		labelValues := []string{"cpu_usage", "percent"}

		collector.addSystemLabels(rawMessage, &labelKeys, &labelValues)
	}
}
