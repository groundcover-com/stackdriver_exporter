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

	"github.com/stretchr/testify/assert"
)

func TestMonitoringCollector_AddOrOverride(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	collector := &MonitoringCollector{
		logger: logger,
	}

	tests := []struct {
		name           string
		initialKeys    []string
		initialValues  []string
		addKey         string
		addValue       string
		override       bool
		expectedKeys   []string
		expectedValues []string
		description    string
	}{
		{
			name:           "add_new_key_override_false",
			initialKeys:    []string{"existing"},
			initialValues:  []string{"value1"},
			addKey:         "new_key",
			addValue:       "new_value",
			override:       false,
			expectedKeys:   []string{"existing", "new_key"},
			expectedValues: []string{"value1", "new_value"},
			description:    "Should add new key when it doesn't exist, regardless of override flag",
		},
		{
			name:           "add_new_key_override_true",
			initialKeys:    []string{"existing"},
			initialValues:  []string{"value1"},
			addKey:         "new_key",
			addValue:       "new_value",
			override:       true,
			expectedKeys:   []string{"existing", "new_key"},
			expectedValues: []string{"value1", "new_value"},
			description:    "Should add new key when it doesn't exist, regardless of override flag",
		},
		{
			name:           "existing_key_override_false",
			initialKeys:    []string{"existing", "key2"},
			initialValues:  []string{"original_value", "value2"},
			addKey:         "existing",
			addValue:       "new_value",
			override:       false,
			expectedKeys:   []string{"existing", "key2"},
			expectedValues: []string{"original_value", "value2"},
			description:    "Should not override existing key when override is false",
		},
		{
			name:           "existing_key_override_true",
			initialKeys:    []string{"existing", "key2"},
			initialValues:  []string{"original_value", "value2"},
			addKey:         "existing",
			addValue:       "new_value",
			override:       true,
			expectedKeys:   []string{"existing", "key2"},
			expectedValues: []string{"new_value", "value2"},
			description:    "Should override existing key when override is true",
		},
		{
			name:           "empty_initial_slices",
			initialKeys:    []string{},
			initialValues:  []string{},
			addKey:         "first_key",
			addValue:       "first_value",
			override:       false,
			expectedKeys:   []string{"first_key"},
			expectedValues: []string{"first_value"},
			description:    "Should add to empty slices",
		},
		{
			name:           "override_first_key",
			initialKeys:    []string{"target", "other"},
			initialValues:  []string{"original", "other_value"},
			addKey:         "target",
			addValue:       "overridden",
			override:       true,
			expectedKeys:   []string{"target", "other"},
			expectedValues: []string{"overridden", "other_value"},
			description:    "Should override first key correctly",
		},
		{
			name:           "override_middle_key",
			initialKeys:    []string{"first", "target", "last"},
			initialValues:  []string{"first_value", "original", "last_value"},
			addKey:         "target",
			addValue:       "overridden",
			override:       true,
			expectedKeys:   []string{"first", "target", "last"},
			expectedValues: []string{"first_value", "overridden", "last_value"},
			description:    "Should override middle key correctly",
		},
		{
			name:           "override_last_key",
			initialKeys:    []string{"first", "second", "target"},
			initialValues:  []string{"first_value", "second_value", "original"},
			addKey:         "target",
			addValue:       "overridden",
			override:       true,
			expectedKeys:   []string{"first", "second", "target"},
			expectedValues: []string{"first_value", "second_value", "overridden"},
			description:    "Should override last key correctly",
		},
		{
			name:           "case_sensitive_keys",
			initialKeys:    []string{"Key", "KEY"},
			initialValues:  []string{"value1", "value2"},
			addKey:         "key",
			addValue:       "value3",
			override:       false,
			expectedKeys:   []string{"Key", "KEY", "key"},
			expectedValues: []string{"value1", "value2", "value3"},
			description:    "Should treat keys as case sensitive",
		},
		{
			name:           "empty_key_and_value",
			initialKeys:    []string{"existing"},
			initialValues:  []string{"value1"},
			addKey:         "",
			addValue:       "",
			override:       false,
			expectedKeys:   []string{"existing", ""},
			expectedValues: []string{"value1", ""},
			description:    "Should handle empty key and value",
		},
		{
			name:           "special_characters",
			initialKeys:    []string{"normal"},
			initialValues:  []string{"value1"},
			addKey:         "key.with-special_chars@domain.com",
			addValue:       "special/value:with|symbols",
			override:       false,
			expectedKeys:   []string{"normal", "key.with-special_chars@domain.com"},
			expectedValues: []string{"value1", "special/value:with|symbols"},
			description:    "Should handle special characters in keys and values",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create copies of the slices to avoid modifying the test data
			labelKeys := make([]string, len(tt.initialKeys))
			labelValues := make([]string, len(tt.initialValues))
			copy(labelKeys, tt.initialKeys)
			copy(labelValues, tt.initialValues)

			// Call the function under test
			collector.addOrOverrideLabels(&labelKeys, &labelValues, tt.addKey, tt.addValue, tt.override)

			// Verify results
			assert.Equal(t, tt.expectedKeys, labelKeys, tt.description+" - keys mismatch")
			assert.Equal(t, tt.expectedValues, labelValues, tt.description+" - values mismatch")
			assert.Equal(t, len(labelKeys), len(labelValues), "Keys and values length should match")
		})
	}
}

func TestMonitoringCollector_AddOrOverride_Integration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	collector := &MonitoringCollector{
		logger: logger,
	}

	t.Run("multiple_operations", func(t *testing.T) {
		labelKeys := []string{"initial"}
		labelValues := []string{"initial_value"}

		// Add new key
		collector.addOrOverrideLabels(&labelKeys, &labelValues, "new1", "value1", false)
		assert.Equal(t, []string{"initial", "new1"}, labelKeys)
		assert.Equal(t, []string{"initial_value", "value1"}, labelValues)

		// Try to override without permission
		collector.addOrOverrideLabels(&labelKeys, &labelValues, "initial", "changed", false)
		assert.Equal(t, []string{"initial", "new1"}, labelKeys)
		assert.Equal(t, []string{"initial_value", "value1"}, labelValues)

		// Override with permission
		collector.addOrOverrideLabels(&labelKeys, &labelValues, "initial", "changed", true)
		assert.Equal(t, []string{"initial", "new1"}, labelKeys)
		assert.Equal(t, []string{"changed", "value1"}, labelValues)

		// Add another new key
		collector.addOrOverrideLabels(&labelKeys, &labelValues, "new2", "value2", true)
		assert.Equal(t, []string{"initial", "new1", "new2"}, labelKeys)
		assert.Equal(t, []string{"changed", "value1", "value2"}, labelValues)
	})
}

func TestMonitoringCollector_FindKeyIndex(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	collector := &MonitoringCollector{
		logger: logger,
	}

	tests := []struct {
		name          string
		keys          []string
		searchKey     string
		expectedIndex int
		description   string
	}{
		{
			name:          "key_found_first",
			keys:          []string{"target", "other", "another"},
			searchKey:     "target",
			expectedIndex: 0,
			description:   "Should find key at first position",
		},
		{
			name:          "key_found_middle",
			keys:          []string{"first", "target", "last"},
			searchKey:     "target",
			expectedIndex: 1,
			description:   "Should find key at middle position",
		},
		{
			name:          "key_found_last",
			keys:          []string{"first", "second", "target"},
			searchKey:     "target",
			expectedIndex: 2,
			description:   "Should find key at last position",
		},
		{
			name:          "key_not_found",
			keys:          []string{"first", "second", "third"},
			searchKey:     "missing",
			expectedIndex: -1,
			description:   "Should return -1 when key not found",
		},
		{
			name:          "empty_slice",
			keys:          []string{},
			searchKey:     "any",
			expectedIndex: -1,
			description:   "Should return -1 for empty slice",
		},
		{
			name:          "empty_search_key",
			keys:          []string{"", "other"},
			searchKey:     "",
			expectedIndex: 0,
			description:   "Should find empty key",
		},
		{
			name:          "case_sensitive",
			keys:          []string{"Key", "key", "KEY"},
			searchKey:     "key",
			expectedIndex: 1,
			description:   "Should be case sensitive",
		},
		{
			name:          "duplicate_keys_first_match",
			keys:          []string{"dup", "other", "dup"},
			searchKey:     "dup",
			expectedIndex: 0,
			description:   "Should return first match for duplicate keys",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collector.findKeyIndex(tt.keys, tt.searchKey)
			assert.Equal(t, tt.expectedIndex, result, tt.description)
		})
	}
}

func BenchmarkMonitoringCollector_AddOrOverride(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	collector := &MonitoringCollector{
		logger: logger,
	}

	// Setup initial data
	labelKeys := []string{"key1", "key2", "key3", "key4", "key5"}
	labelValues := []string{"val1", "val2", "val3", "val4", "val5"}

	b.Run("add_new_key", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			keys := make([]string, len(labelKeys))
			values := make([]string, len(labelValues))
			copy(keys, labelKeys)
			copy(values, labelValues)

			collector.addOrOverrideLabels(&keys, &values, "new_key", "new_value", false)
		}
	})

	b.Run("override_existing_key", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			keys := make([]string, len(labelKeys))
			values := make([]string, len(labelValues))
			copy(keys, labelKeys)
			copy(values, labelValues)

			collector.addOrOverrideLabels(&keys, &values, "key3", "new_value", true)
		}
	})

	b.Run("skip_existing_key", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			keys := make([]string, len(labelKeys))
			values := make([]string, len(labelValues))
			copy(keys, labelKeys)
			copy(values, labelValues)

			collector.addOrOverrideLabels(&keys, &values, "key3", "new_value", false)
		}
	})
}
