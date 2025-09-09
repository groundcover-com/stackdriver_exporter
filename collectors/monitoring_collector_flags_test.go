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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/googleapi"
)

func TestMonitoringCollectorOptions_ConfigurationFlags(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	tests := []struct {
		name               string
		enableSystemLabels bool
		userLabelsOverride bool
	}{
		{
			name:               "all_features_enabled",
			enableSystemLabels: true,
			userLabelsOverride: true,
		},
		{
			name:               "all_features_disabled",
			enableSystemLabels: false,
			userLabelsOverride: false,
		},
		{
			name:               "only_system_labels_enabled",
			enableSystemLabels: true,
			userLabelsOverride: false,
		},
		{
			name:               "only_user_override_enabled",
			enableSystemLabels: false,
			userLabelsOverride: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := MonitoringCollectorOptions{
				MetricTypePrefixes: []string{"compute.googleapis.com"},
				RequestInterval:    time.Minute,
				EnableSystemLabels: tt.enableSystemLabels,
				UserLabelsOverride: tt.userLabelsOverride,
			}

			// Create a mock monitoring service (nil is fine for this test)
			collector, err := NewMonitoringCollector(
				"test-project",
				nil, // monitoring service not needed for this test
				opts,
				logger,
				nil, // counter store
				nil, // histogram store
			)

			require.NoError(t, err)
			assert.Equal(t, tt.enableSystemLabels, collector.enableSystemLabels)
			assert.Equal(t, tt.userLabelsOverride, collector.userLabelsOverride)
		})
	}
}

func TestMonitoringCollector_LabelProcessingOrder(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	tests := []struct {
		name               string
		enableSystemLabels bool
		userLabelsOverride bool
		unitLabel          string
		metricLabels       map[string]string
		resourceLabels     map[string]string
		userLabels         map[string]string
		systemLabelsJSON   string
		expectedKeys       []string
		expectedValues     []string
		description        string
	}{
		{
			name:               "system_labels_disabled",
			enableSystemLabels: false,
			userLabelsOverride: false,
			unitLabel:          "bytes",
			metricLabels:       map[string]string{"metric_key": "metric_value"},
			resourceLabels:     map[string]string{"resource_key": "resource_value"},
			userLabels:         map[string]string{"user_key": "user_value"},
			systemLabelsJSON:   `{"system_key": "system_value"}`,
			expectedKeys:       []string{"unit", "metric_key", "resource_key", "user_key"},
			expectedValues:     []string{"bytes", "metric_value", "resource_value", "user_value"},
			description:        "System labels should be ignored when disabled",
		},
		{
			name:               "system_labels_enabled_no_override",
			enableSystemLabels: true,
			userLabelsOverride: false,
			unitLabel:          "count",
			metricLabels:       map[string]string{"metric_key": "metric_value"},
			resourceLabels:     map[string]string{"resource_key": "resource_value"},
			userLabels:         map[string]string{"user_key": "user_value"},
			systemLabelsJSON:   `{"system_key": "system_value"}`,
			expectedKeys:       []string{"unit", "metric_key", "resource_key", "system_key", "user_key"},
			expectedValues:     []string{"count", "metric_value", "resource_value", "system_value", "user_value"},
			description:        "System labels should be added first when no override (system labels take precedence)",
		},
		{
			name:               "user_override_enabled",
			enableSystemLabels: true,
			userLabelsOverride: true,
			unitLabel:          "seconds",
			metricLabels:       map[string]string{"metric_key": "metric_value"},
			resourceLabels:     map[string]string{"resource_key": "resource_value"},
			userLabels:         map[string]string{"conflict_key": "user_value"},
			systemLabelsJSON:   `{"conflict_key": "system_value", "system_only": "system_only_value"}`,
			expectedKeys:       []string{"unit", "metric_key", "resource_key", "conflict_key", "system_only"},
			expectedValues:     []string{"seconds", "metric_value", "resource_value", "user_value", "system_only_value"},
			description:        "User labels should override system labels when enabled",
		},
		{
			name:               "no_user_override_conflict",
			enableSystemLabels: true,
			userLabelsOverride: false,
			unitLabel:          "percent",
			metricLabels:       map[string]string{"metric_key": "metric_value"},
			resourceLabels:     map[string]string{"resource_key": "resource_value"},
			userLabels:         map[string]string{"conflict_key": "user_value"},
			systemLabelsJSON:   `{"conflict_key": "system_value", "system_only": "system_only_value"}`,
			expectedKeys:       []string{"unit", "metric_key", "resource_key", "conflict_key", "system_only"},
			expectedValues:     []string{"percent", "metric_value", "resource_value", "system_value", "system_only_value"},
			description:        "System labels should win over user labels when no override (system labels take precedence)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create collector with test configuration
			collector := &MonitoringCollector{
				logger:             logger,
				enableSystemLabels: tt.enableSystemLabels,
				userLabelsOverride: tt.userLabelsOverride,
			}

			// Simulate the label processing logic from reportTimeSeriesMetrics
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

			// Add user labels and system labels based on configuration
			if collector.userLabelsOverride {
				// Add user labels first, then system labels (user labels take precedence)
				for key, value := range tt.userLabels {
					if !collector.keyExists(labelKeys, key) {
						labelKeys = append(labelKeys, key)
						labelValues = append(labelValues, value)
					}
				}
				if collector.enableSystemLabels {
					rawMessage := googleapi.RawMessage(tt.systemLabelsJSON)
					collector.addSystemLabels(rawMessage, &labelKeys, &labelValues)
				}
			} else {
				// Add system labels first, then user labels (system labels take precedence)
				if collector.enableSystemLabels {
					rawMessage := googleapi.RawMessage(tt.systemLabelsJSON)
					collector.addSystemLabels(rawMessage, &labelKeys, &labelValues)
				}
				for key, value := range tt.userLabels {
					if !collector.keyExists(labelKeys, key) {
						labelKeys = append(labelKeys, key)
						labelValues = append(labelValues, value)
					}
				}
			}

			// Verify results
			assert.Equal(t, tt.expectedKeys, labelKeys, tt.description+" - keys mismatch")
			assert.Equal(t, tt.expectedValues, labelValues, tt.description+" - values mismatch")
			assert.Equal(t, len(labelKeys), len(labelValues), "Keys and values length should match")
		})
	}
}
