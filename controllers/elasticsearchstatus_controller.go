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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"net"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strconv"
	"time"

	elasticv1alpha1 "Bobft-ElasticSearch-Operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ElasticsearchStatusReconciler reconciles a ElasticsearchStatus object
type ElasticsearchStatusReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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
// the ElasticsearchStatus object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *ElasticsearchStatusReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

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

	errUS := r.getOwnResourcesExistAndUpdateStatus(instance,r.Client,log)
	if errUS != nil {
		log.Error(errUS,"getOwnResourcesExist!!!")
	}


	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ElasticsearchStatusReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&elasticv1alpha1.Elasticsearch{}).
		Complete(r)
}

func LabelsSelectorForElasticStatus(name string) *metav1.LabelSelector{
	return &metav1.LabelSelector{
		MatchLabels:      map[string]string{"app": "elasticsearch", "instance": name},
		MatchExpressions: nil,
	}
}

func (r *ElasticsearchStatusReconciler) getOwnResourcesExistAndUpdateStatus(instance *elasticv1alpha1.Elasticsearch, client client.Client, logger logr.Logger) (error) {

	oldInstance := instance.DeepCopy()

	foundCM := &corev1.ConfigMap{}
	errCM := client.Get(context.TODO(), types.NamespacedName{Name: instance.Name + "-cm", Namespace: instance.Namespace}, foundCM)
	if errCM != nil {
		if errors.IsNotFound(errCM) {
			return errCM
		}
		msg := fmt.Sprintf("ConfigMap %s/%s found, but with error", instance.Namespace, instance.Name)
		logger.Error(errCM, msg)
		return errCM
	}

	foundSVC := &corev1.Service{}
	errSVC := client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, foundSVC)
	if errSVC != nil {
		if errors.IsNotFound(errSVC) {
			return errSVC
		}
		msg := fmt.Sprintf("Service %s/%s found, but with error", instance.Namespace, instance.Name)
		logger.Error(errSVC, msg)
		return errSVC
	}

	foundHLSVC := &corev1.Service{}
	errHLSVC := client.Get(context.TODO(), types.NamespacedName{Name: instance.Name + "-headless-service", Namespace: instance.Namespace}, foundHLSVC)
	if errHLSVC != nil {
		if errors.IsNotFound(errHLSVC) {
			return errHLSVC
		}
		msg := fmt.Sprintf("Service %s/%s found, but with error", instance.Namespace, instance.Name)
		logger.Error(errHLSVC, msg)
		return errHLSVC
	}

	foundSTS := &appsv1.StatefulSet{}
	errSTS := client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, foundSTS)
	if errSTS != nil {
		if errors.IsNotFound(errSTS) {
			return errHLSVC
		}
		msg := fmt.Sprintf("StatefulSet %s/%s found, but with error", instance.Namespace, instance.Name)
		logger.Error(errSTS, msg)
		return errHLSVC
	}

	foundPVCStatus := []corev1.PersistentVolumeClaimStatus{}
	for i:=0; i < int(instance.Spec.Size); i++ {
		foundPVC := &corev1.PersistentVolumeClaim{}
		errorsPVC := client.Get(context.TODO(), types.NamespacedName{Name: instance.Name + "-data-" + instance.Name + "-" + strconv.Itoa(i), Namespace: instance.Namespace}, foundPVC)
		if errorsPVC != nil {
			if errors.IsNotFound(errorsPVC) {
				return errorsPVC
			}
			msg := fmt.Sprintf("PersistentVolumeClaim %s/%s found, but with error", instance.Namespace, instance.Name)
			logger.Error(errHLSVC, msg)
			return errHLSVC
		}
		foundPVCStatus = append(foundPVCStatus,foundPVC.Status)
	}

	instance.Status.AvailableReplicas = foundSTS.Status.AvailableReplicas
	instance.Status.LastUpdateTime = metav1.Now()
	instance.Status.OwnResourcesStatus.Service.Type = foundSVC.Spec.Type
	instance.Status.OwnResourcesStatus.Service.ClusterIP = foundSVC.Spec.ClusterIP
	instance.Status.OwnResourcesStatus.Service.SessionAffinity = foundSVC.Spec.SessionAffinity
	var portsStatus []elasticv1alpha1.ServicePortStatus
	for _, port := range foundSVC.Spec.Ports {
		health := true
		checkPort := port.Port
		addr := foundSVC.Spec.ClusterIP
		sock := fmt.Sprintf("%s:%d", addr, checkPort)
		proto := string(port.Protocol)
		_, err := net.DialTimeout(proto, sock, time.Duration(100)*time.Millisecond)
		if err != nil {
			health = false
		}

		portStatus := elasticv1alpha1.ServicePortStatus{
			Ports:		 port,
			Health:      health,
		}
		portsStatus = append(portsStatus, portStatus)
	}
	instance.Status.OwnResourcesStatus.Service.Ports = portsStatus
	foundEP := &corev1.Endpoints{}
	errEP := client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, foundEP)
	if errEP != nil {
		if errors.IsNotFound(errEP) {
			return errEP
		}
		msg := fmt.Sprintf("EndPoints %s/%s found, but with error", instance.Namespace, instance.Name)
		logger.Error(errEP, msg)
		return errEP
	}
	endpoints := []elasticv1alpha1.OwnResourcesEndpointStatus{}
	for _,valueA := range foundEP.Subsets{
		endpoint := elasticv1alpha1.OwnResourcesEndpointStatus{}
		for _,valueB := range valueA.Addresses{
			endpoint.NodeName = *valueB.NodeName
			endpoint.PodIP = valueB.IP
			endpoint.PodName = valueB.TargetRef.Name
			endpoints = append(endpoints,endpoint)
		}
	}
	instance.Status.OwnResourcesStatus.Endpoint = endpoints
	instance.Status.OwnResourcesStatus.PVC = foundPVCStatus

	if oldInstance == nil {
		err := r.Status().Update(context.Background(), instance);
		if err != nil {
			logger.Error(err, "unable to update ElasitcSearch status")
			return err
		}
	}
	if oldInstance != nil && !reflect.DeepEqual(oldInstance.Status, instance.Status) {
		err := r.Status().Update(context.Background(), instance);
		if err != nil {
			logger.Error(err, "unable to update ElasitcSearch status")
			return err
		}
	}

	return nil
}