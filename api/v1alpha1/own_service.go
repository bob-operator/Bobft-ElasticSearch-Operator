package v1alpha1

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"net"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"
)

type OwnService struct {
	Spec     corev1.ServiceSpec `json:"spec"`
}

type OwnResourcesServiceStatus struct {
	Type            corev1.ServiceType      `json:"type,omitempty"`
	ClusterIP       string              `json:"clusterIP,omitempty"`
	Ports           []ServicePortStatus `json:"ports,omitempty"`
	SessionAffinity corev1.ServiceAffinity  `json:"sessionAffinity,omitempty"`
}
type ServicePortStatus struct {
	Ports corev1.ServicePort `json:"ports,omitempty"`
	Health bool `json:"health,omitempty"`
}

type OwnResourcesEndpointStatus struct {
	PodName  string `json:"podName"`
	PodIP    string `json:"podIP"`
	NodeName string `json:"nodeName"`
}

func (ownService *OwnService) MakeOwnResource(instance *Elasticsearch, logger logr.Logger, scheme *runtime.Scheme) (interface{}, interface{}, error) {

	service, err:= ownService.MakeOwnService(instance,logger,scheme)
	if err != nil {
		return nil, nil, err
	}
	headlessservice, err:= ownService.MakeOwnHeadlessService(instance,logger,scheme)
	if err != nil {
		return nil, nil, err
	}

	return service, headlessservice , nil
}

// Check if the ownService already exists
func (ownService *OwnService) OwnServiceExist(instance *Elasticsearch, client client.Client, logger logr.Logger) (bool, interface{}, error) {

	found := &corev1.Service{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		msg := fmt.Sprintf("Service %s/%s found, but with error", instance.Namespace, instance.Name)
		logger.Error(err, msg)
		return true, found, err
	}
	return true, found, nil
}

func (ownService *OwnService) OwnHeadlessServiceExist(instance *Elasticsearch, client client.Client, logger logr.Logger) (bool, interface{}, error) {

	found := &corev1.Service{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: instance.Name + "-headless-service", Namespace: instance.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		msg := fmt.Sprintf("Service %s/%s found, but with error", instance.Namespace, instance.Name)
		logger.Error(err, msg)
		return true, found, err
	}
	return true, found, nil
}

// 更新own Service的status，以及own Service对应的Endpoint状态也一起在这里处理
func (ownService *OwnService) UpdateOwnResourceStatus(instance *Elasticsearch, client client.Client, logger logr.Logger) (*Elasticsearch, error) {

	// 更新Service status
	found := &corev1.Service{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, found)
	if err != nil {
		return instance, err
	}

	var portsStatus []ServicePortStatus
	for _, port := range found.Spec.Ports {
		health := true
		checkPort := port.Port
		addr := found.Spec.ClusterIP
		sock := fmt.Sprintf("%s:%d", addr, checkPort)
		proto := string(port.Protocol)
		_, err := net.DialTimeout(proto, sock, time.Duration(100)*time.Millisecond)
		if err != nil {
			health = false
		}

		portStatus := ServicePortStatus{
			Ports: 		 port,
			Health:      health,
		}
		portsStatus = append(portsStatus, portStatus)
	}

	serviceStatus := OwnResourcesServiceStatus{
		Type:      found.Spec.Type,
		ClusterIP: found.Spec.ClusterIP,
		Ports:     portsStatus,
	}
	instance.Status.OwnResourcesStatus.Service = serviceStatus

	foundEndpoint := &corev1.Endpoints{}
	err = client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, foundEndpoint)
	if err != nil {
		return instance, err
	}

	if foundEndpoint.Subsets != nil && foundEndpoint.Subsets[0].Addresses != nil {
		var endpointsStatus []OwnResourcesEndpointStatus
		for _, ep := range foundEndpoint.Subsets[0].Addresses {
			endpointStatus := OwnResourcesEndpointStatus{
				PodName:  ep.Hostname,
				PodIP:    ep.IP,
				NodeName: *ep.NodeName,
			}
			endpointsStatus = append(endpointsStatus, endpointStatus)
		}
		instance.Status.OwnResourcesStatus.Endpoint = endpointsStatus
	}
	instance.Status.LastUpdateTime = metav1.Now()

	return instance, nil
}

func (ownService *OwnService) ApplyOwnResource(instance *Elasticsearch, client client.Client, logger logr.Logger, scheme *runtime.Scheme) error {

	existA, foundA, err := ownService.OwnServiceExist(instance, client, logger)
	if err != nil {
		return err
	}
	existB, foundB, err := ownService.OwnHeadlessServiceExist(instance, client, logger)
	if err != nil {
		return err
	}

	newService, err := ownService.MakeOwnService(instance, logger, scheme)
	if err != nil {
		return err
	}

	newHeadlessService, err := ownService.MakeOwnHeadlessService(instance, logger, scheme)
	if err != nil {
		return err
	}

	if !existA {
		// if Service not exist，then create it
		msg := fmt.Sprintf("Service %s/%s not found, create it!", newService.Namespace, newService.Name)
		logger.Info(msg)
		err := client.Create(context.TODO(), newService);
		if err != nil {
			return err
		}
	} else {
		foundServiceA := foundA.(*corev1.Service)
		newService.Spec.ClusterIP = foundServiceA.Spec.ClusterIP
		newService.Spec.SessionAffinity = foundServiceA.Spec.SessionAffinity
		// if Service exist with change，then try to update it
		if !reflect.DeepEqual(newService.Spec, foundServiceA.Spec) {
			msg := fmt.Sprintf("Updating Service %s/%s", newService.Namespace, newService.Name)
			logger.Info(msg)
			err := client.Update(context.TODO(), newService);
			if err != nil {
				return err
			}
		}
	}
	if !existB {
		// if Service not exist，then create it
		msg := fmt.Sprintf("HeadLessService %s/%s not found, create it!", newService.Namespace, newService.Name)
		logger.Info(msg)
		err := client.Create(context.TODO(), newHeadlessService);
		if err != nil {
			return err
		}
	} else {
		foundServiceB := foundB.(*corev1.Service)

		newHeadlessService.Spec.ClusterIP = foundServiceB.Spec.ClusterIP
		newHeadlessService.Spec.SessionAffinity = foundServiceB.Spec.SessionAffinity

		if !reflect.DeepEqual(newHeadlessService.Spec, foundServiceB.Spec) {
			msg := fmt.Sprintf("Updating HeadLessService %s/%s", newHeadlessService.Namespace, newHeadlessService.Name)
			logger.Info(msg)
			err := client.Update(context.TODO(), newHeadlessService);
			if err != nil {
				return err
			}
		}
	}

	return nil

}



func (ownService *OwnService) MakeOwnService(instance *Elasticsearch, logger logr.Logger, scheme *runtime.Scheme) (*corev1.Service, error) {

	// new a Service object
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    labelsSelectorForElastic(instance.Name).MatchLabels},
		Spec: ownService.Spec,
	}
	svc.Spec.Selector = labelsSelectorForElastic(instance.Name).MatchLabels

	if err := controllerutil.SetControllerReference(instance, svc, scheme); err != nil {
		msg := fmt.Sprintf("set controllerReference for Service %s/%s failed", instance.Namespace, instance.Name)
		logger.Error(err, msg)
		return nil, err
	}

	return svc, nil

}

func (ownService *OwnService) MakeOwnHeadlessService(instance *Elasticsearch, logger logr.Logger, scheme *runtime.Scheme) (*corev1.Service, error) {

	// new a Service object
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-headless-service",
			Namespace: instance.Namespace,
			Labels:    labelsSelectorForElastic(instance.Name).MatchLabels},
		Spec: ownService.Spec,
	}
	svc.Spec.Selector = labelsSelectorForElastic(instance.Name).MatchLabels
	svc.Spec.Type = "ClusterIP"
	svc.Spec.ClusterIP = "None"

	if err := controllerutil.SetControllerReference(instance, svc, scheme); err != nil {
		msg := fmt.Sprintf("set controllerReference for Service %s/%s failed", instance.Namespace, instance.Name)
		logger.Error(err, msg)
		return nil, err
	}

	return svc, nil

}