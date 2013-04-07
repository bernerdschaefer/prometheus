// Copyright 2013 Prometheus Team
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

package model

import (
	"fmt"
	dto "github.com/prometheus/prometheus/model/generated"
	"time"
)

var (
	_ = fmt.Sprintf
)

// CurationRemark provides a representation of dto.CurationValue with associated
// business logic methods attached to it to enhance code readability.
type CurationRemark struct {
	LastCompletionTimestamp time.Time
}

// OlderThanLimit answers whether this CurationRemark is older than the provided
// cutOff time.
func (c CurationRemark) OlderThan(cutOff time.Time) bool {
	return c.LastCompletionTimestamp.Before(cutOff)
}

// Equal answers whether the two CurationRemarks are equivalent.
func (c CurationRemark) Equal(o CurationRemark) bool {
	fmt.Println(c, o)
	return c.LastCompletionTimestamp.Equal(o.LastCompletionTimestamp)
}

// NewCurationRemarkFromDTO builds CurationRemark from the provided
// dto.CurationValue object.
func NewCurationRemarkFromDTO(d *dto.CurationValue) CurationRemark {
	return CurationRemark{
		LastCompletionTimestamp: time.Unix(*d.LastCompletionTimestamp, 0),
	}
}

// CurationKey provides a representation of dto.CurationKey with asociated
// business logic methods attached to it to enhance code readability.
type CurationKey struct {
	Fingerprint      Fingerprint
	OlderThan        time.Duration
	MinimumGroupSize uint32
}

/// Equal answers whether the two CurationKeys are equivalent.
func (c CurationKey) Equal(o CurationKey) (equal bool) {
	switch {
	case !c.Fingerprint.Equal(o.Fingerprint):
		return
	case c.OlderThan != o.OlderThan:
		return
	case c.MinimumGroupSize != o.MinimumGroupSize:
		return
	}

	return true
}

// NewCurationKeyFromDTO vuilds CurationKey from the provided dto.CurationKey.
func NewCurationKeyFromDTO(d *dto.CurationKey) CurationKey {
	return CurationKey{
		Fingerprint:      NewFingerprintFromDTO(d.Fingerprint),
		OlderThan:        time.Duration(*d.OlderThan),
		MinimumGroupSize: *d.MinimumGroupSize,
	}
}
