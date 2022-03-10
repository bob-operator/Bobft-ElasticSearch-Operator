/*
Copyright 2022.

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

package controllers

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logs "sigs.k8s.io/controller-runtime/pkg/log"

	elasticv1alpha1 "Bobft-ElasticSearch-Operator/api/v1alpha1"
)

// ElasticsearchReconciler reconciles a Elasticsearch object
type ElasticsearchReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

type OwnResource interface {

	ApplyOwnResource(instance *elasticv1alpha1.Elasticsearch, client client.Client, logger logr.Logger, scheme *runtime.Scheme) error

}

//+kubebuilder:rbac:groups=elastic.bobfintech.com,resources=elasticsearches,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=elastic.bobfintech.com,resources=elasticsearches/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=elastic.bobfintech.com,resources=elasticsearches/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulSet,verbs=get;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=service,verbs=get;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=endpoint,verbs=get
// +kubebuilder:rbac:groups=core,resources=persistentVolumeClaimStatus,verbs=get;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmap,verbs=get;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Elasticsearch object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *ElasticsearchReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logs.FromContext(ctx)

	instance := &elasticv1alpha1.Elasticsearch{}

	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
	}

	ownResources, err := r.getOwnResources(instance)
	if err != nil {
		msg := fmt.Sprintf("%s %s Reconciler.getOwnResource() function error", instance.Namespace, instance.Name)
		log.Error(err,msg)
		return ctrl.Result{}, err
	}

	for _, ownResource := range ownResources {
		if err = ownResource.ApplyOwnResource(instance, r.Client, log, r.Scheme); err != nil {
			log.Error(err,"ApplyOwnResource Failed!!!")
		}
	}
	
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ElasticsearchReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&elasticv1alpha1.Elasticsearch{}).
		Complete(r)
}

func LabelsSelectorForElastic(name string) *metav1.LabelSelector{
	
	return &metav1.LabelSelector{
		MatchLabels:      map[string]string{"app": "elasticsearch", "instance": name},
		MatchExpressions: nil,
	}

}

func (r *ElasticsearchReconciler) getOwnResources(instance *elasticv1alpha1.Elasticsearch) ([]OwnResource, error) {
	var ownResources []OwnResource
	
	ownStatefulSet := &elasticv1alpha1.OwnStatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &instance.Spec.Size,
			Selector:    LabelsSelectorForElastic(instance.Name),
			ServiceName: instance.Name + "headless-serivce",
		},
	}

	ownResources = append(ownResources, ownStatefulSet)

	if instance.Spec.Service != nil {
		ownResources = append(ownResources, instance.Spec.Service)
	}
	if instance.Spec.Config != nil {
		ownResources = append(ownResources, instance.Spec.Config)
	}

	return ownResources, nil
}