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
	// 根据ElasticSearch的指定，生成相应的各类own_resources资源对象，用作创建或更新
	//MakeOwnResource(instance *elasticv1alpha1.Elasticsearch, logger logr.Logger, scheme *runtime.Scheme) (interface{}, error)

	// 判断此资源是否已存在
	//OwnResourceExist(instance *elasticv1alpha1.Elasticsearch, client client.Client, logger logr.Logger) (bool, interface{}, error)

	// 获取ElasticSearch对应的own_resource的状态，用来填充ElasticSearch的status字段
	//UpdateOwnResourceStatus(instance *elasticv1alpha1.Elasticsearch, client client.Client, logger logr.Logger) (*elasticv1alpha1.Elasticsearch, error)

	// 创建/更新获取ElasticSearch对应的own_resource的状态对应的own_resource资源
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

	// Get操作,获取 Unit object
	instance := &elasticv1alpha1.Elasticsearch{}

	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
	}

	// Create or Update操作
	// 根据ElasticSearch.Spec 生成ElasticSearch关联的所有own_resource
	ownResources, err := r.getOwnResources(instance)
	if err != nil {
		msg := fmt.Sprintf("%s %s Reconciler.getOwnResource() function error", instance.Namespace, instance.Name)
		log.Error(err,msg)
		return ctrl.Result{}, err
	}

	// 3.2 判断各own resource 是否存在，不存在则创建，存在则判断spec是否有变化，有变化则更新
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

// 根据Unit.Spec生成其所有的own resource
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

	// 将关联的资源(svc/cm/pvc)加入ownResources中
	if instance.Spec.Service != nil {
		ownResources = append(ownResources, instance.Spec.Service)
	}
	if instance.Spec.Config != nil {
		ownResources = append(ownResources, instance.Spec.Config)
	}

	return ownResources, nil
}