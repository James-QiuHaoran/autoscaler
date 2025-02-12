/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	vpa_model "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/recommender/model"
	"k8s.io/autoscaler/vertical-pod-autoscaler/pkg/recommender/util"
)

var (
	timeLayout       = "2006-01-02 15:04:05"
	testTimestamp, _ = time.Parse(timeLayout, "2017-04-18 17:35:05")

	TestRequest = vpa_model.Resources{
		vpa_model.ResourceCPU:    vpa_model.CPUAmountFromCores(2.3),
		vpa_model.ResourceMemory: vpa_model.MemoryAmountFromBytes(5e8),
	}
)

const (
	kb = 1024
	mb = 1024 * kb
)

func newUsageSample(timestamp time.Time, usage int64, resource vpa_model.ResourceName) *vpa_model.ContainerUsageSample {
	return &vpa_model.ContainerUsageSample{
		MeasureStart: timestamp,
		Usage:        vpa_model.ResourceAmount(usage),
		Request:      TestRequest[resource],
		Resource:     resource,
	}
}

type ContainerTest struct {
	mockCPUHistogram        *util.MockHistogram
	mockMemoryHistogram     *util.MockHistogram
	aggregateContainerState *vpa_model.AggregateContainerState
	container               *ContainerState
}

func newContainerTest() ContainerTest {
	mockCPUHistogram := new(util.MockHistogram)
	mockMemoryHistogram := new(util.MockHistogram)
	aggregateContainerState := &vpa_model.AggregateContainerState{
		AggregateCPUUsage:    mockCPUHistogram,
		AggregateMemoryPeaks: mockMemoryHistogram,
	}
	container := &ContainerState{
		Request:    TestRequest,
		aggregator: aggregateContainerState,
	}
	return ContainerTest{
		mockCPUHistogram:        mockCPUHistogram,
		mockMemoryHistogram:     mockMemoryHistogram,
		aggregateContainerState: aggregateContainerState,
		container:               container,
	}
}

// Add 6 usage samples (3 valid, 3 invalid) to a container. Verifies that for
// valid samples the CPU measures are aggregated in the CPU histogram and
// the memory measures are aggregated in the memory peaks sliding window.
// Verifies that invalid samples (out-of-order or negative usage) are ignored.
func TestAggregateContainerUsageSamples(t *testing.T) {
	test := newContainerTest()
	c := test.container
	memoryAggregationInterval := vpa_model.GetAggregationsConfig().MemoryAggregationInterval
	// Verify that CPU measures are added to the CPU histogram.
	// The weight should be equal to the current request.
	timeStep := memoryAggregationInterval / 2
	test.mockCPUHistogram.On("AddSample", 3.14, 2.3, testTimestamp)
	test.mockCPUHistogram.On("AddSample", 6.28, 2.3, testTimestamp.Add(timeStep))
	test.mockCPUHistogram.On("AddSample", 1.57, 2.3, testTimestamp.Add(2*timeStep))
	// Verify that memory peaks are added to the memory peaks histogram.
	memoryAggregationWindowEnd := testTimestamp.Add(memoryAggregationInterval)
	test.mockMemoryHistogram.On("AddSample", 5.0, 1.0, memoryAggregationWindowEnd)
	test.mockMemoryHistogram.On("SubtractSample", 5.0, 1.0, memoryAggregationWindowEnd)
	test.mockMemoryHistogram.On("AddSample", 10.0, 1.0, memoryAggregationWindowEnd)
	memoryAggregationWindowEnd = memoryAggregationWindowEnd.Add(memoryAggregationInterval)
	test.mockMemoryHistogram.On("AddSample", 2.0, 1.0, memoryAggregationWindowEnd)

	// Add three CPU and memory usage samples.
	assert.True(t, c.AddSample(newUsageSample(
		testTimestamp, 3140, vpa_model.ResourceCPU)))
	assert.True(t, c.AddSample(newUsageSample(
		testTimestamp, 5, vpa_model.ResourceMemory)))

	assert.True(t, c.AddSample(newUsageSample(
		testTimestamp.Add(timeStep), 6280, vpa_model.ResourceCPU)))
	assert.True(t, c.AddSample(newUsageSample(
		testTimestamp.Add(timeStep), 10, vpa_model.ResourceMemory)))

	assert.True(t, c.AddSample(newUsageSample(
		testTimestamp.Add(2*timeStep), 1570, vpa_model.ResourceCPU)))
	assert.True(t, c.AddSample(newUsageSample(
		testTimestamp.Add(2*timeStep), 2, vpa_model.ResourceMemory)))

	// Discard invalid samples.
	assert.False(t, c.AddSample(newUsageSample( // Out of order sample.
		testTimestamp.Add(2*timeStep), 1000, vpa_model.ResourceCPU)))
	assert.False(t, c.AddSample(newUsageSample( // Negative CPU usage.
		testTimestamp.Add(4*timeStep), -1000, vpa_model.ResourceCPU)))
	assert.False(t, c.AddSample(newUsageSample( // Negative memory usage.
		testTimestamp.Add(4*timeStep), -1000, vpa_model.ResourceMemory)))
}

func TestRecordOOMIncreasedByBumpUp(t *testing.T) {
	test := newContainerTest()
	memoryAggregationWindowEnd := testTimestamp.Add(vpa_model.GetAggregationsConfig().MemoryAggregationInterval)
	// Bump Up factor is 20%.
	test.mockMemoryHistogram.On("AddSample", 1200.0*mb, 1.0, memoryAggregationWindowEnd)

	assert.NoError(t, test.container.RecordOOM(testTimestamp, vpa_model.ResourceAmount(1000*mb)))
}

func TestRecordOOMDontRunAway(t *testing.T) {
	test := newContainerTest()
	memoryAggregationWindowEnd := testTimestamp.Add(vpa_model.GetAggregationsConfig().MemoryAggregationInterval)

	// Bump Up factor is 20%.
	test.mockMemoryHistogram.On("AddSample", 1200.0*mb, 1.0, memoryAggregationWindowEnd)
	assert.NoError(t, test.container.RecordOOM(testTimestamp, vpa_model.ResourceAmount(1000*mb)))

	// new smaller OOMs don't influence the sample value (oomPeak)
	assert.NoError(t, test.container.RecordOOM(testTimestamp, vpa_model.ResourceAmount(999*mb)))
	assert.NoError(t, test.container.RecordOOM(testTimestamp, vpa_model.ResourceAmount(999*mb)))

	test.mockMemoryHistogram.On("SubtractSample", 1200.0*mb, 1.0, memoryAggregationWindowEnd)
	test.mockMemoryHistogram.On("AddSample", 2400.0*mb, 1.0, memoryAggregationWindowEnd)
	// a larger OOM should increase the sample value
	assert.NoError(t, test.container.RecordOOM(testTimestamp, vpa_model.ResourceAmount(2000*mb)))
}

func TestRecordOOMIncreasedByMin(t *testing.T) {
	test := newContainerTest()
	memoryAggregationWindowEnd := testTimestamp.Add(vpa_model.GetAggregationsConfig().MemoryAggregationInterval)
	// Min grow by 100Mb.
	test.mockMemoryHistogram.On("AddSample", 101.0*mb, 1.0, memoryAggregationWindowEnd)

	assert.NoError(t, test.container.RecordOOM(testTimestamp, vpa_model.ResourceAmount(1*mb)))
}

func TestRecordOOMMaxedWithKnownSample(t *testing.T) {
	test := newContainerTest()
	memoryAggregationWindowEnd := testTimestamp.Add(vpa_model.GetAggregationsConfig().MemoryAggregationInterval)

	test.mockMemoryHistogram.On("AddSample", 3000.0*mb, 1.0, memoryAggregationWindowEnd)
	assert.True(t, test.container.AddSample(newUsageSample(testTimestamp, 3000*mb, vpa_model.ResourceMemory)))

	// Last known sample is higher than request, so it is taken.
	test.mockMemoryHistogram.On("SubtractSample", 3000.0*mb, 1.0, memoryAggregationWindowEnd)
	test.mockMemoryHistogram.On("AddSample", 3600.0*mb, 1.0, memoryAggregationWindowEnd)

	assert.NoError(t, test.container.RecordOOM(testTimestamp, vpa_model.ResourceAmount(1000*mb)))
}

func TestRecordOOMDiscardsOldSample(t *testing.T) {
	test := newContainerTest()
	memoryAggregationWindowEnd := testTimestamp.Add(vpa_model.GetAggregationsConfig().MemoryAggregationInterval)

	test.mockMemoryHistogram.On("AddSample", 1000.0*mb, 1.0, memoryAggregationWindowEnd)
	assert.True(t, test.container.AddSample(newUsageSample(testTimestamp, 1000*mb, vpa_model.ResourceMemory)))

	// OOM is stale, mem not changed.
	assert.Error(t, test.container.RecordOOM(testTimestamp.Add(-30*time.Hour), vpa_model.ResourceAmount(1000*mb)))
}

func TestRecordOOMInNewWindow(t *testing.T) {
	test := newContainerTest()
	memoryAggregationInterval := vpa_model.GetAggregationsConfig().MemoryAggregationInterval
	memoryAggregationWindowEnd := testTimestamp.Add(memoryAggregationInterval)

	test.mockMemoryHistogram.On("AddSample", 2000.0*mb, 1.0, memoryAggregationWindowEnd)
	assert.True(t, test.container.AddSample(newUsageSample(testTimestamp, 2000*mb, vpa_model.ResourceMemory)))

	memoryAggregationWindowEnd = memoryAggregationWindowEnd.Add(2 * memoryAggregationInterval)
	test.mockMemoryHistogram.On("AddSample", 2400.0*mb, 1.0, memoryAggregationWindowEnd)
	assert.NoError(t, test.container.RecordOOM(testTimestamp.Add(2*memoryAggregationInterval), vpa_model.ResourceAmount(1000*mb)))
}
