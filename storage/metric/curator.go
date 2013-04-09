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

package metric

import (
	"code.google.com/p/goprotobuf/proto"
	"fmt"
	"github.com/prometheus/prometheus/coding"
	"github.com/prometheus/prometheus/model"
	dto "github.com/prometheus/prometheus/model/generated"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/raw"
	"github.com/prometheus/prometheus/storage/raw/leveldb"
	"strings"
	"time"
)

var (
	_ = fmt.Sprint
)

// curator is responsible for effectuating a given curation policy across the
// stored samples on-disk.  This is useful to compact sparse sample values into
// single sample entities to reduce keyspace load on the datastore.
type curator struct {
	// stop functions as a channel that when empty allows the curator to operate.
	// The moment a value is ingested inside of it, the curator goes into drain
	// mode.
	stop chan bool
	// samples is the on-disk metric store that is scanned for compaction
	// candidates.
	samples raw.Persistence
	// watermarks is the on-disk store that is scanned for high watermarks for
	// given metrics.
	watermarks raw.Persistence
	// recencyThreshold represents the most recent time up to which values will be
	// curated.
	recencyThreshold time.Duration
	// groupingQuantity represents the number of samples below which encountered
	// samples will be dismembered and reaggregated into larger groups.
	groupingQuantity uint32
	// curationState is the on-disk store where the curation remarks are made for
	// how much progress has been made.
	curationState raw.Persistence
}

// newCurator builds a new curator for the given LevelDB databases.
func newCurator(recencyThreshold time.Duration, groupingQuantity uint32, curationState, samples, watermarks raw.Persistence) curator {
	return curator{
		curationState:    curationState,
		groupingQuantity: groupingQuantity,
		recencyThreshold: recencyThreshold,
		samples:          samples,
		stop:             make(chan bool),
		watermarks:       watermarks,
	}
}

// run facilitates the curation lifecycle.
func (c curator) run(instant time.Time) (err error) {
	ldb := c.samples.(*leveldb.LevelDBPersistence)
	iterator := ldb.NewIterator(true)
	defer iterator.Close()
	diskFrontier, err := newDiskFrontier(iterator)
	if err != nil {
		panic(err)
	}
	if diskFrontier == nil {
		return
	}

	curationContext := curationContext{
		curationState:    c.curationState,
		groupSize:        c.groupingQuantity,
		olderThan:        instant.Add(-1 * c.recencyThreshold),
		recencyThreshold: c.recencyThreshold,
		stop:             c.stop,
	}
	decoder := watermarkDecoder{}
	filter := watermarkFilter{
		curationContext: curationContext,
	}
	operator := watermarkOperator{
		curationContext: curationContext,
		samples:         c.samples,
		diskFrontier:    *diskFrontier,
		iterator:        iterator,
	}

	_, err = c.watermarks.ForEach(decoder, filter, operator)

	return
}

// drain instructs the curator to stop at the next convenient moment as to not
// introduce data inconsistencies.
func (c curator) drain() {
	if len(c.stop) == 0 {
		c.stop <- true
	}
}

// watermarkDecoder converts (dto.Fingerprint, dto.MetricHighWatermark) doubles
// into (model.Fingerprint, model.Watermark) doubles.
type watermarkDecoder struct{}

func (w watermarkDecoder) DecodeKey(in interface{}) (out interface{}, err error) {
	var (
		key   = &dto.Fingerprint{}
		bytes = in.([]byte)
	)

	err = proto.Unmarshal(bytes, key)
	if err != nil {
		panic(err)
	}

	out = model.NewFingerprintFromRowKey(*key.Signature)

	return
}

func (w watermarkDecoder) DecodeValue(in interface{}) (out interface{}, err error) {
	var (
		dto   = &dto.MetricHighWatermark{}
		bytes = in.([]byte)
	)

	err = proto.Unmarshal(bytes, dto)
	if err != nil {
		panic(err)
	}

	out = model.NewWatermarkFromHighWatermarkDTO(dto)

	return
}

type curationContext struct {
	// curationState is the table of CurationKey to CurationValues that rema
	// far along the curation process has gone for a given metric fingerprint.
	curationState raw.Persistence
	// groupSize refers to the target groupSize from the curator.
	groupSize uint32
	// recencyThreshold refers to the target recencyThreshold from the curator.
	recencyThreshold time.Duration
	// olderThan functions as the cutoff when scanning curator.samples for
	// uncurated samples to compact.  The operator scans forward in the samples
	// until olderThan is reached and then stops operation for samples that occur
	// after it.
	olderThan time.Time
	// stop, when non-empty, instructs the filter to stop operation.
	stop chan bool
}

// watermarkFilter determines whether to include or exclude candidate
// values from the curation process by virtue of how old the high watermark is.
type watermarkFilter struct {
	curationContext
}

func (w watermarkFilter) shouldStop() bool {
	return len(w.stop) != 0
}

func (c curationContext) getCurationState(fingerprint model.Fingerprint) (remark model.CurationRemark, err error) {
	curationKey := &dto.CurationKey{
		Fingerprint:      fingerprint.ToDTO(),
		MinimumGroupSize: proto.Uint32(c.groupSize),
		OlderThan:        proto.Int64(int64(c.recencyThreshold)),
	}
	curationValue := &dto.CurationValue{}

	rawCurationValue, err := c.curationState.Get(coding.NewProtocolBuffer(curationKey))
	if err != nil {
		panic(err)
	}

	err = proto.Unmarshal(rawCurationValue, curationValue)
	if err != nil {
		panic(err)
	}

	remark = model.NewCurationRemarkFromDTO(curationValue)

	return
}

func (w watermarkFilter) Filter(key, value interface{}) (result storage.FilterResult) {
	defer func() {
		labels := map[string]string{
			"group_size":        fmt.Sprint(w.groupSize),
			"recency_threshold": fmt.Sprint(w.recencyThreshold),
			"result":            strings.ToLower(result.String()),
		}

		curationFilterOperations.Increment(labels)
	}()

	if w.shouldStop() {
		result = storage.STOP
		return
	}

	fingerprint := key.(model.Fingerprint)
	curationRemark, err := w.getCurationState(fingerprint)
	if err != nil {
		return
	}
	if !curationRemark.OlderThan(w.olderThan) {
		result = storage.SKIP
		return
	}
	watermark := value.(model.Watermark)
	if !curationRemark.OlderThan(watermark.Time) {
		result = storage.SKIP
		return
	}
	curationConsistent, err := w.curationConsistent(fingerprint, watermark)
	if err != nil {
		panic(err)
	}
	if curationConsistent {
		result = storage.SKIP
		return
	}

	result = storage.ACCEPT

	return
}

// watermarkOperator scans over the curator.samples table for metrics whose
// high watermark has been determined to be allowable for curation.  This type
// is individually responsible for compaction.
//
// The scanning starts from CurationRemark.LastCompletionTimestamp and goes
// forward until the stop point or end of the series is reached.
type watermarkOperator struct {
	curationContext
	samples      raw.Persistence
	diskFrontier diskFrontier
	iterator     leveldb.Iterator
}

func (w watermarkOperator) Operate(key, _ interface{}) (oErr *storage.OperatorError) {
	fingerprint := key.(model.Fingerprint)
	seriesFrontier, err := newSeriesFrontier(fingerprint, w.diskFrontier, w.iterator)
	if err != nil {
		panic(err)
	}
	if seriesFrontier == nil {
		return
	}
	curationState, err := w.getCurationState(fingerprint)
	if err != nil {
		panic(err)
	}
	var seekTime time.Time
	switch {
	case seriesFrontier.After(curationState.LastCompletionTimestamp):
		seekTime = seriesFrontier.firstSupertime
	case !seriesFrontier.InSafeSeekRange(curationState.LastCompletionTimestamp):
		seekTime = seriesFrontier.lastSupertime
	default:
		seekTime = curationState.LastCompletionTimestamp
	}
	_ = seekTime
	// timeSeek := &dto.SampleKey{
	// 	Fingerprint: fingerprint.ToDTO(),
	// 	Timestamp:   indexable.EncodeTime(seekTime),
	// }

	curationKey := &dto.CurationKey{
		Fingerprint:      key.(model.Fingerprint).ToDTO(),
		OlderThan:        proto.Int64(int64(w.recencyThreshold)),
		MinimumGroupSize: proto.Uint32(w.groupSize),
	}
	curationValue := &dto.CurationValue{
		LastCompletionTimestamp: proto.Int64(int64(w.olderThan.Unix())),
	}

	e := w.curationState.Put(coding.NewProtocolBuffer(curationKey), coding.NewProtocolBuffer(curationValue))
	if e != nil {
		panic(e)
	}

	return
}

// hasBeenCurated answers true if the provided Fingerprint has been curated in
// in the past.
func (w watermarkOperator) hasBeenCurated(f model.Fingerprint) (curated bool, err error) {
	curationKey := &dto.CurationKey{
		Fingerprint:      f.ToDTO(),
		OlderThan:        proto.Int64(int64(w.recencyThreshold)),
		MinimumGroupSize: proto.Uint32(w.groupSize),
	}

	curated, err = w.curationState.Has(coding.NewProtocolBuffer(curationKey))

	return
}

// curationConsistent determines whether the given metric is in a dirty state
// and needs curation.
func (w watermarkFilter) curationConsistent(f model.Fingerprint, watermark model.Watermark) (consistent bool, err error) {
	curationRemark, err := w.getCurationState(f)
	if err != nil {
		panic(err)
	}
	if !curationRemark.OlderThan(watermark.Time) {
		consistent = true
	}

	return
}
