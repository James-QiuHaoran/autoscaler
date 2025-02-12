/*
Copyright 2018 The Kubernetes Authors.

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

package checkpoint

import (
	"context"
	"fmt"
	"sort"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	mpa_types "k8s.io/autoscaler/multidimensional-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1alpha1"
	"k8s.io/autoscaler/multidimensional-pod-autoscaler/pkg/recommender/model"
	api_util "k8s.io/autoscaler/multidimensional-pod-autoscaler/pkg/utils/mpa"
	vpa_types "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	vpa_api "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned/typed/autoscaling.k8s.io/v1"
	vpa_model "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/recommender/model"
	"k8s.io/klog/v2"
)

// CheckpointWriter persistently stores aggregated historical usage of containers
// controlled by MPA objects. This state can be restored to initialize the model after restart.
type CheckpointWriter interface {
	// StoreCheckpoints writes at least minCheckpoints if there are more checkpoints to write.
	// Checkpoints are written until ctx permits or all checkpoints are written.
	StoreCheckpoints(ctx context.Context, now time.Time, minCheckpoints int) error
}

type checkpointWriter struct {
	vpaCheckpointClient vpa_api.VerticalPodAutoscalerCheckpointsGetter
	cluster             *model.ClusterState
}

// NewCheckpointWriter returns new instance of a CheckpointWriter
func NewCheckpointWriter(cluster *model.ClusterState, vpaCheckpointClient vpa_api.VerticalPodAutoscalerCheckpointsGetter) CheckpointWriter {
	return &checkpointWriter{
		vpaCheckpointClient: vpaCheckpointClient,
		cluster:             cluster,
	}
}

func isFetchingHistory(mpa *model.Mpa) bool {
	condition, found := mpa.Conditions[mpa_types.FetchingHistory]
	if !found {
		return false
	}
	return condition.Status == v1.ConditionTrue
}

func getMpasToCheckpoint(clusterMpas map[model.MpaID]*model.Mpa) []*model.Mpa {
	mpas := make([]*model.Mpa, 0, len(clusterMpas))
	for _, mpa := range clusterMpas {
		if isFetchingHistory(mpa) {
			klog.V(3).Infof("MPA %s/%s is loading history, skipping checkpoints", mpa.ID.Namespace, mpa.ID.MpaName)
			continue
		}
		mpas = append(mpas, mpa)
	}
	sort.Slice(mpas, func(i, j int) bool {
		return mpas[i].CheckpointWritten.Before(mpas[j].CheckpointWritten)
	})
	return mpas
}

func (writer *checkpointWriter) StoreCheckpoints(ctx context.Context, now time.Time, minCheckpoints int) error {
	mpas := getMpasToCheckpoint(writer.cluster.Mpas)
	for _, mpa := range mpas {

		// Draining ctx.Done() channel. ctx.Err() will be checked if timeout occurred, but minCheckpoints have
		// to be written before return from this function.
		select {
		case <-ctx.Done():
		default:
		}

		if ctx.Err() != nil && minCheckpoints <= 0 {
			return ctx.Err()
		}

		aggregateContainerStateMap := buildAggregateContainerStateMap(mpa, writer.cluster, now)
		for container, aggregatedContainerState := range aggregateContainerStateMap {
			containerCheckpoint, err := aggregatedContainerState.SaveToCheckpoint()
			if err != nil {
				klog.Errorf("Cannot serialize checkpoint for mpa %v container %v. Reason: %+v", mpa.ID.MpaName, container, err)
				continue
			}
			checkpointName := fmt.Sprintf("%s-%s", mpa.ID.MpaName, container)
			vpaCheckpoint := vpa_types.VerticalPodAutoscalerCheckpoint{
				ObjectMeta: metav1.ObjectMeta{Name: checkpointName},
				Spec: vpa_types.VerticalPodAutoscalerCheckpointSpec{
					ContainerName: container,
					VPAObjectName: mpa.ID.MpaName,
				},
				Status: *containerCheckpoint,
			}
			err = api_util.CreateOrUpdateMpaCheckpoint(writer.vpaCheckpointClient.VerticalPodAutoscalerCheckpoints(mpa.ID.Namespace), &vpaCheckpoint)
			if err != nil {
				klog.Errorf("Cannot save MPA %s/%s checkpoint for %s. Reason: %+v",
					mpa.ID.Namespace, vpaCheckpoint.Spec.VPAObjectName, vpaCheckpoint.Spec.ContainerName, err)
			} else {
				klog.V(3).Infof("Saved MPA %s/%s checkpoint for %s",
					mpa.ID.Namespace, vpaCheckpoint.Spec.VPAObjectName, vpaCheckpoint.Spec.ContainerName)
				mpa.CheckpointWritten = now
			}
			minCheckpoints--
		}
	}
	return nil
}

// Build the AggregateContainerState for the purpose of the checkpoint. This is an aggregation of state of all
// containers that belong to pods matched by the MPA.
// Note however that we exclude the most recent memory peak for each container (see below).
func buildAggregateContainerStateMap(mpa *model.Mpa, cluster *model.ClusterState, now time.Time) map[string]*vpa_model.AggregateContainerState {
	aggregateContainerStateMap := mpa.AggregateStateByContainerName()
	// Note: the memory peak from the current (ongoing) aggregation interval is not included in the
	// checkpoint to avoid having multiple peaks in the same interval after the state is restored from
	// the checkpoint. Therefore we are extracting the current peak from all containers.
	// TODO: Avoid the nested loop over all containers for each MPA.
	for _, pod := range cluster.Pods {
		for containerName, container := range pod.Containers {
			aggregateKey := cluster.MakeAggregateStateKey(pod, containerName)
			if mpa.UsesAggregation(aggregateKey) {
				if aggregateContainerState, exists := aggregateContainerStateMap[containerName]; exists {
					subtractCurrentContainerMemoryPeak(aggregateContainerState, container, now)
				}
			}
		}
	}
	return aggregateContainerStateMap
}

func subtractCurrentContainerMemoryPeak(a *vpa_model.AggregateContainerState, container *model.ContainerState, now time.Time) {
	if now.Before(container.WindowEnd) {
		a.AggregateMemoryPeaks.SubtractSample(vpa_model.BytesFromMemoryAmount(container.GetMaxMemoryPeak()), 1.0, container.WindowEnd)
	}
}
