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

package logic

import (
	"context"
	"strconv"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	mpa_types "k8s.io/autoscaler/multidimensional-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1alpha1"
	target_mock "k8s.io/autoscaler/multidimensional-pod-autoscaler/pkg/target/mock"
	"k8s.io/autoscaler/multidimensional-pod-autoscaler/pkg/updater/eviction"
	"k8s.io/autoscaler/multidimensional-pod-autoscaler/pkg/updater/priority"
	mpa_test "k8s.io/autoscaler/multidimensional-pod-autoscaler/pkg/utils/test"
	vpa_types "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/autoscaler/vertical-pod-autoscaler/pkg/utils/status"
	"k8s.io/autoscaler/vertical-pod-autoscaler/pkg/utils/test"
)

func parseLabelSelector(selector string) labels.Selector {
	labelSelector, _ := metav1.ParseToLabelSelector(selector)
	parsedSelector, _ := metav1.LabelSelectorAsSelector(labelSelector)
	return parsedSelector
}

func TestRunOnce_Mode(t *testing.T) {
	tests := []struct {
		name                  string
		updateMode            vpa_types.UpdateMode
		expectFetchCalls      bool
		expectedEvictionCount int
	}{
		{
			name:                  "with Auto mode",
			updateMode:            vpa_types.UpdateModeAuto,
			expectFetchCalls:      true,
			expectedEvictionCount: 5,
		},
		{
			name:                  "with Initial mode",
			updateMode:            vpa_types.UpdateModeInitial,
			expectFetchCalls:      false,
			expectedEvictionCount: 0,
		},
		{
			name:                  "with Off mode",
			updateMode:            vpa_types.UpdateModeOff,
			expectFetchCalls:      false,
			expectedEvictionCount: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testRunOnceBase(
				t,
				tc.updateMode,
				newFakeValidator(true),
				tc.expectFetchCalls,
				tc.expectedEvictionCount,
			)
		})
	}
}

func TestRunOnce_Status(t *testing.T) {
	tests := []struct {
		name                  string
		statusValidator       status.Validator
		expectFetchCalls      bool
		expectedEvictionCount int
	}{
		{
			name:                  "with valid status",
			statusValidator:       newFakeValidator(true),
			expectFetchCalls:      true,
			expectedEvictionCount: 5,
		},
		{
			name:                  "with invalid status",
			statusValidator:       newFakeValidator(false),
			expectFetchCalls:      false,
			expectedEvictionCount: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testRunOnceBase(
				t,
				vpa_types.UpdateModeAuto,
				tc.statusValidator,
				tc.expectFetchCalls,
				tc.expectedEvictionCount,
			)
		})
	}
}

func testRunOnceBase(
	t *testing.T,
	updateMode vpa_types.UpdateMode,
	statusValidator status.Validator,
	expectFetchCalls bool,
	expectedEvictionCount int,
) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	replicas := int32(5)
	livePods := 5
	labels := map[string]string{"app": "testingApp"}
	selector := parseLabelSelector("app = testingApp")
	containerName := "container1"
	rc := apiv1.ReplicationController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rc",
			Namespace: "default",
		},
		Spec: apiv1.ReplicationControllerSpec{
			Replicas: &replicas,
		},
	}
	pods := make([]*apiv1.Pod, livePods)
	eviction := &test.PodsEvictionRestrictionMock{}

	for i := range pods {
		pods[i] = test.Pod().WithName("test_"+strconv.Itoa(i)).
			AddContainer(test.BuildTestContainer(containerName, "1", "100M")).
			WithCreator(&rc.ObjectMeta, &rc.TypeMeta).
			Get()

		pods[i].Labels = labels
		eviction.On("CanEvict", pods[i]).Return(true)
		eviction.On("Evict", pods[i], nil).Return(nil)
	}

	factory := &fakeEvictFactory{eviction}
	mpaLister := &mpa_test.MultidimPodAutoscalerListerMock{}

	podLister := &test.PodListerMock{}
	podLister.On("List").Return(pods, nil)

	mpaObj := mpa_test.MultidimPodAutoscaler().
		WithContainer(containerName).
		WithTarget("2", "200M").
		WithMinAllowed("1", "100M").
		WithMaxAllowed("3", "1G").
		Get()
	mpaObj.Spec.Policy = &mpa_types.PodUpdatePolicy{UpdateMode: &updateMode}
	mpaLister.On("List").Return([]*mpa_types.MultidimPodAutoscaler{mpaObj}, nil).Once()

	mockSelectorFetcher := target_mock.NewMockMpaTargetSelectorFetcher(ctrl)

	updater := &updater{
		mpaLister:                    mpaLister,
		podLister:                    podLister,
		evictionFactory:              factory,
		evictionRateLimiter:          rate.NewLimiter(rate.Inf, 0),
		recommendationProcessor:      &mpa_test.FakeRecommendationProcessor{},
		selectorFetcher:              mockSelectorFetcher,
		useAdmissionControllerStatus: true,
		statusValidator:              statusValidator,
		priorityProcessor:            priority.NewProcessor(),
	}

	if expectFetchCalls {
		mockSelectorFetcher.EXPECT().Fetch(gomock.Eq(mpaObj)).Return(selector, nil)
	}
	updater.RunOnce(context.Background())
	eviction.AssertNumberOfCalls(t, "Evict", expectedEvictionCount)
}

func TestRunOnceNotingToProcess(t *testing.T) {
	eviction := &test.PodsEvictionRestrictionMock{}
	factory := &fakeEvictFactory{eviction}
	mpaLister := &mpa_test.MultidimPodAutoscalerListerMock{}
	podLister := &test.PodListerMock{}
	mpaLister.On("List").Return(nil, nil).Once()

	updater := &updater{
		mpaLister:                    mpaLister,
		podLister:                    podLister,
		evictionFactory:              factory,
		evictionRateLimiter:          rate.NewLimiter(rate.Inf, 0),
		recommendationProcessor:      &mpa_test.FakeRecommendationProcessor{},
		useAdmissionControllerStatus: true,
		statusValidator:              newFakeValidator(true),
	}
	updater.RunOnce(context.Background())
}

func TestGetRateLimiter(t *testing.T) {
	cases := []struct {
		rateLimit       float64
		rateLimitBurst  int
		expectedLimiter *rate.Limiter
	}{
		{0.0, 1, rate.NewLimiter(rate.Inf, 0)},
		{-1.0, 2, rate.NewLimiter(rate.Inf, 0)},
		{10.0, 3, rate.NewLimiter(rate.Limit(10), 3)},
	}
	for _, tc := range cases {
		limiter := getRateLimiter(tc.rateLimit, tc.rateLimitBurst)
		assert.Equal(t, tc.expectedLimiter.Burst(), limiter.Burst())
		assert.InDelta(t, float64(tc.expectedLimiter.Limit()), float64(limiter.Limit()), 1e-6)
	}
}

type fakeEvictFactory struct {
	evict eviction.PodsEvictionRestriction
}

func (f fakeEvictFactory) NewPodsEvictionRestriction(pods []*apiv1.Pod, mpa *mpa_types.MultidimPodAutoscaler) eviction.PodsEvictionRestriction {
	return f.evict
}

type fakeValidator struct {
	isValid bool
}

func newFakeValidator(isValid bool) status.Validator {
	return &fakeValidator{isValid}
}

func (f *fakeValidator) IsStatusValid(statusTimeout time.Duration) (bool, error) {
	return f.isValid, nil
}
