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
)

func BenchmarkHashLabels(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	dedup := NewMetricDeduplicator(logger, "test_project")
	fqName := "benchmark_metric"
	keys := []string{"region", "zone", "instance", "project", "service", "method", "version"}
	vals := []string{"us-central1", "us-central1-a", "instance-1", "my-project", "api-service", "get", "v1"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dedup.hashLabels(fqName, keys, vals)
	}
}
