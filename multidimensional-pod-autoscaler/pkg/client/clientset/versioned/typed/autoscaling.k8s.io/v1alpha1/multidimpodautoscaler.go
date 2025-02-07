/*
Copyright The Kubernetes Authors.

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

// Code generated by client-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	v1alpha1 "k8s.io/autoscaler/multidimensional-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1alpha1"
	scheme "k8s.io/autoscaler/multidimensional-pod-autoscaler/pkg/client/clientset/versioned/scheme"
	rest "k8s.io/client-go/rest"
)

// MultidimPodAutoscalersGetter has a method to return a MultidimPodAutoscalerInterface.
// A group's client should implement this interface.
type MultidimPodAutoscalersGetter interface {
	MultidimPodAutoscalers(namespace string) MultidimPodAutoscalerInterface
}

// MultidimPodAutoscalerInterface has methods to work with MultidimPodAutoscaler resources.
type MultidimPodAutoscalerInterface interface {
	Create(ctx context.Context, multidimPodAutoscaler *v1alpha1.MultidimPodAutoscaler, opts v1.CreateOptions) (*v1alpha1.MultidimPodAutoscaler, error)
	Update(ctx context.Context, multidimPodAutoscaler *v1alpha1.MultidimPodAutoscaler, opts v1.UpdateOptions) (*v1alpha1.MultidimPodAutoscaler, error)
	UpdateStatus(ctx context.Context, multidimPodAutoscaler *v1alpha1.MultidimPodAutoscaler, opts v1.UpdateOptions) (*v1alpha1.MultidimPodAutoscaler, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1alpha1.MultidimPodAutoscaler, error)
	List(ctx context.Context, opts v1.ListOptions) (*v1alpha1.MultidimPodAutoscalerList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.MultidimPodAutoscaler, err error)
	MultidimPodAutoscalerExpansion
}

// multidimPodAutoscalers implements MultidimPodAutoscalerInterface
type multidimPodAutoscalers struct {
	client rest.Interface
	ns     string
}

// newMultidimPodAutoscalers returns a MultidimPodAutoscalers
func newMultidimPodAutoscalers(c *AutoscalingV1alpha1Client, namespace string) *multidimPodAutoscalers {
	return &multidimPodAutoscalers{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the multidimPodAutoscaler, and returns the corresponding multidimPodAutoscaler object, and an error if there is any.
func (c *multidimPodAutoscalers) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.MultidimPodAutoscaler, err error) {
	result = &v1alpha1.MultidimPodAutoscaler{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("multidimpodautoscalers").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of MultidimPodAutoscalers that match those selectors.
func (c *multidimPodAutoscalers) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.MultidimPodAutoscalerList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1alpha1.MultidimPodAutoscalerList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("multidimpodautoscalers").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested multidimPodAutoscalers.
func (c *multidimPodAutoscalers) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("multidimpodautoscalers").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx)
}

// Create takes the representation of a multidimPodAutoscaler and creates it.  Returns the server's representation of the multidimPodAutoscaler, and an error, if there is any.
func (c *multidimPodAutoscalers) Create(ctx context.Context, multidimPodAutoscaler *v1alpha1.MultidimPodAutoscaler, opts v1.CreateOptions) (result *v1alpha1.MultidimPodAutoscaler, err error) {
	result = &v1alpha1.MultidimPodAutoscaler{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("multidimpodautoscalers").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(multidimPodAutoscaler).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a multidimPodAutoscaler and updates it. Returns the server's representation of the multidimPodAutoscaler, and an error, if there is any.
func (c *multidimPodAutoscalers) Update(ctx context.Context, multidimPodAutoscaler *v1alpha1.MultidimPodAutoscaler, opts v1.UpdateOptions) (result *v1alpha1.MultidimPodAutoscaler, err error) {
	result = &v1alpha1.MultidimPodAutoscaler{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("multidimpodautoscalers").
		Name(multidimPodAutoscaler.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(multidimPodAutoscaler).
		Do(ctx).
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *multidimPodAutoscalers) UpdateStatus(ctx context.Context, multidimPodAutoscaler *v1alpha1.MultidimPodAutoscaler, opts v1.UpdateOptions) (result *v1alpha1.MultidimPodAutoscaler, err error) {
	result = &v1alpha1.MultidimPodAutoscaler{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("multidimpodautoscalers").
		Name(multidimPodAutoscaler.Name).
		SubResource("status").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(multidimPodAutoscaler).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the multidimPodAutoscaler and deletes it. Returns an error if one occurs.
func (c *multidimPodAutoscalers) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("multidimpodautoscalers").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *multidimPodAutoscalers) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("multidimpodautoscalers").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched multidimPodAutoscaler.
func (c *multidimPodAutoscalers) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.MultidimPodAutoscaler, err error) {
	result = &v1alpha1.MultidimPodAutoscaler{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("multidimpodautoscalers").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}
