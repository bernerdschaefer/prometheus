// Copyright 2012 Prometheus Team
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

package metric

import (
	"github.com/matttproud/prometheus/model"
)

type MetricPersistence interface {
	Close() error

	AppendSample(sample *model.Sample) error

	GetLabelNames() ([]string, error)
	GetLabelPairs() ([]model.LabelPairs, error)
	GetMetrics() ([]model.LabelPairs, error)

	GetMetricFingerprintsForLabelPairs(labelSets []*model.LabelPairs) ([]*model.Fingerprint, error)
	GetFingerprintLabelPairs(fingerprint model.Fingerprint) (model.LabelPairs, error)

	RecordFingerprintWatermark(sample *model.Sample) error
}
