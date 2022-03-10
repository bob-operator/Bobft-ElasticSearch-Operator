package v1alpha1

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strconv"
)

type OwnStatefulSet struct {
	Spec appsv1.StatefulSetSpec `json:"spec"`
}

func (ownStatefulSet *OwnStatefulSet) MakeOwnResource(instance *Elasticsearch, logger logr.Logger, scheme *runtime.Scheme) (interface{}, error) {

	ownStatefulSet.Spec.Replicas = &instance.Spec.Size
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: instance.Name,
			Namespace: instance.Namespace,
			Labels: labelsSelectorForElastic(instance.Name).MatchLabels},
		Spec:       ownStatefulSet.Spec,
	}
	sts.Spec.ServiceName = instance.Name +"-headless-service"
	sts.Spec.Template = ownStatefulSet.MakePodTemplate(instance)
	sts.Spec.VolumeClaimTemplates = ownStatefulSet.MakeVolume(instance)

	volumes := []corev1.Volume{}
	volumeConfig := corev1.Volume{}
	volumeConfig.Name = instance.Name + "-cm"
	var configmapSource *corev1.ConfigMapVolumeSource
	configmapSource = new(corev1.ConfigMapVolumeSource)
	configmapSource.Name = instance.Name + "-cm"
	volumeConfig.ConfigMap = configmapSource
	volumes = append(volumes,volumeConfig)
	sts.Spec.Template.Spec.Volumes = volumes

	if err := controllerutil.SetControllerReference(instance, sts, scheme); err != nil {
		msg := fmt.Sprintf("set controllerReference for StatefulSet %s/%s failed", instance.Namespace, instance.Name)
		logger.Error(err, msg)
		return nil, err
	}

	return sts, nil

}

func (ownStatefulSet *OwnStatefulSet) OwnResourceExist(instance *Elasticsearch, client client.Client, logger logr.Logger) (bool, interface{}, error) {

	found := &appsv1.StatefulSet{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		msg := fmt.Sprintf("StatefulSet %s/%s found, but with error", instance.Namespace, instance.Name)
		logger.Error(err, msg)
		return true, found, err
	}
	return true, found, nil

}

func (ownStatefulSet *OwnStatefulSet) UpdateOwnResourceStatus(instance *Elasticsearch, client client.Client, logger logr.Logger) (*Elasticsearch, error) {

	found := &appsv1.StatefulSet{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, found)
	if err != nil {
		return instance, err
	}

	foundPVC := &corev1.PersistentVolumeClaim{}
	pvcStatuss := []corev1.PersistentVolumeClaimStatus{}
	for i:=0; i<int(instance.Spec.Size); i++ {
		err := client.Get(context.TODO(), types.NamespacedName{Name: instance.Name + "-data" + strconv.Itoa(i), Namespace: instance.Namespace}, foundPVC)
		if err != nil {
			return nil , err
		}
		pvcStatuss = append(pvcStatuss,foundPVC.Status)
	}

	instance.Status.LastUpdateTime = metav1.Now()
	instance.Status.AvailableReplicas = found.Status.AvailableReplicas
	instance.Status.OwnResourcesStatus.PVC = pvcStatuss
	return instance, nil

}

func (ownStatefulSet *OwnStatefulSet) MakeVolume(instance *Elasticsearch) ([]corev1.PersistentVolumeClaim) {

	accessmode := corev1.PersistentVolumeAccessMode("ReadWriteMany")
	accessmodearray := []corev1.PersistentVolumeAccessMode{}
	accessmodearray = append(accessmodearray,accessmode)

	volume := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: instance.Name + "-data",
			Namespace: instance.Namespace},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: accessmodearray,
			Resources: corev1.ResourceRequirements{
				Limits:   nil,
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(instance.Spec.StorageResourceRequest),
				},
			},
			StorageClassName: instance.Spec.StorageClass,
		},
	}

	volumes := []corev1.PersistentVolumeClaim{}
	volumes = append(volumes,volume)
	return volumes

}

func (ownStatefulSet *OwnStatefulSet) MakePodTemplate(instance *Elasticsearch) (corev1.PodTemplateSpec) {

	podtemplate := corev1.PodTemplateSpec{}
	podtemplate.Labels = labelsSelectorForElastic(instance.Name).MatchLabels

	initContainers := []corev1.Container{}
	initContainer := corev1.Container{}
	initContainer.Name = "sysctl"
	var privileged *bool
	securityContext := new(corev1.SecurityContext); privileged = new(bool); *privileged = true
	securityContext.Privileged = privileged
	initContainer.SecurityContext = securityContext
	command := []string{"sh","-c","sysctl -w vm.max_map_count=262144"}
	initContainer.Command = command
	initContainer.Image = "busybox"
	initContainer.ImagePullPolicy = "IfNotPresent"
	initContainers = append(initContainers,initContainer)

	containers := []corev1.Container{}
	esContainer := corev1.Container{}
	esContainer.Image = instance.Spec.Image
	esContainer.Resources = instance.Spec.Resources

	envs := instance.Spec.Env
	jvmEnv := corev1.EnvVar{}
	jvmEnv.Name = "ES_JAVA_OPTS"
	jvmEnv.Value = instance.Spec.Jvm
	clusterNameEnv := corev1.EnvVar{}
	clusterNameEnv.Name = "POD_IP"
	clusterNameEnvVarSource := corev1.EnvVarSource{}
	var clusterNameObjectFieldSelector *corev1.ObjectFieldSelector; clusterNameObjectFieldSelector = new(corev1.ObjectFieldSelector)
	clusterNameObjectFieldSelector.APIVersion = "v1"
	clusterNameObjectFieldSelector.FieldPath = "status.podIP"
	clusterNameEnvVarSource.FieldRef = clusterNameObjectFieldSelector
	clusterNameEnv.ValueFrom = &clusterNameEnvVarSource
	podNameEnv := corev1.EnvVar{}
	podNameEnv.Name = "POD_NAME"
	podNameEnvVarSource := corev1.EnvVarSource{}
	var podNameObjectFieldSelector *corev1.ObjectFieldSelector; podNameObjectFieldSelector = new(corev1.ObjectFieldSelector)
	podNameObjectFieldSelector.APIVersion = "v1"
	podNameObjectFieldSelector.FieldPath = "metadata.name"
	podNameEnvVarSource.FieldRef = podNameObjectFieldSelector
	podNameEnv.ValueFrom = &podNameEnvVarSource
	nodeNameEnv := corev1.EnvVar{}
	nodeNameEnv.Name = "NODE_NAME"
	nodeNameEnvVarSource := corev1.EnvVarSource{}
	var nodeNameObjectFieldSelector *corev1.ObjectFieldSelector; nodeNameObjectFieldSelector = new(corev1.ObjectFieldSelector)
	nodeNameObjectFieldSelector.APIVersion = "v1"
	nodeNameObjectFieldSelector.FieldPath = "spec.nodeName"
	nodeNameEnvVarSource.FieldRef = nodeNameObjectFieldSelector
	nodeNameEnv.ValueFrom = &nodeNameEnvVarSource
	namespaceNameEnv := corev1.EnvVar{}
	namespaceNameEnv.Name = "NAMESPACE"
	namespaceNameEnvVarSource := corev1.EnvVarSource{}
	var namespaceNameObjectFieldSelector *corev1.ObjectFieldSelector; namespaceNameObjectFieldSelector = new(corev1.ObjectFieldSelector)
	namespaceNameObjectFieldSelector.APIVersion = "v1"
	namespaceNameObjectFieldSelector.FieldPath = "metadata.namespace"
	namespaceNameEnvVarSource.FieldRef = namespaceNameObjectFieldSelector
	namespaceNameEnv.ValueFrom = &namespaceNameEnvVarSource
	headlessServiceNameEnv := corev1.EnvVar{}
	headlessServiceNameEnv.Name = "HEADLESS_SERVICE_NAME"
	headlessServiceNameEnv.Value = instance.Name + "-headless-service"
	discoverySeedHostsEnv := corev1.EnvVar{}
	discoverySeedHostsEnv.Name = "discovery.seed_hosts"
	discoverySeedHostsEnv.Value = instance.Name + "-headless-service"

	envs = append(envs,jvmEnv)
	envs = append(envs,clusterNameEnv)
	envs = append(envs,podNameEnv)
	envs = append(envs,nodeNameEnv)
	envs = append(envs,namespaceNameEnv)
	envs = append(envs,headlessServiceNameEnv)
	envs = append(envs,discoverySeedHostsEnv)

	ports := []corev1.ContainerPort{}
	portA := corev1.ContainerPort{}
	portA.Name = "http"
	portA.ContainerPort = 9200
	portA.Protocol = "TCP"
	ports = append(ports,portA)
	portB := corev1.ContainerPort{}
	portB.Name = "transport"
	portB.ContainerPort = 9300
	portB.Protocol = "TCP"
	ports = append(ports,portB)

	volumeMounts := []corev1.VolumeMount{}
	volumeMountData := corev1.VolumeMount{}
	volumeMountData.Name = instance.Name + "-data"
	volumeMountData.MountPath = "/usr/share/elasticsearch/data"
	volumeMounts = append(volumeMounts,volumeMountData)
	volumeMountConfig := corev1.VolumeMount{}
	volumeMountConfig.Name = instance.Name + "-cm"
	volumeMountConfig.MountPath = "/usr/share/elasticsearch/config/elasticsearch.yml"
	volumeMountConfig.SubPath = "elasticsearch.yml"
	volumeMounts = append(volumeMounts,volumeMountConfig)

	esContainer.Env = envs
	esContainer.VolumeMounts = volumeMounts
	esContainer.Ports = ports
	esContainer.Name = "elasticsearch"
	esContainer.ImagePullPolicy = "IfNotPresent"
	containers = append(containers,esContainer)

	podtemplate.Spec.InitContainers = initContainers
	podtemplate.Spec.Containers = containers

	return podtemplate
}

func labelsSelectorForElastic(name string) *metav1.LabelSelector{

	return &metav1.LabelSelector{
		MatchLabels:      map[string]string{"app": "elasticsearch", "instance": name},
		MatchExpressions: nil,
	}

}

func InitESClusterEnvValue(instance *Elasticsearch) string {
	initESClusterEnvValue := ""
	if instance.Spec.Size%2 == 0 {
		for i := 0; i < int(instance.Spec.Size)-1; i++ {
			initESClusterEnvValue = initESClusterEnvValue + instance.Name + "-" + strconv.Itoa(i) + "." + instance.Name + "-headless-service" + "." + instance.Namespace + ".svc.cluster.local,"
		}
	} else {
		for i := 0; i < int(instance.Spec.Size); i++ {
			initESClusterEnvValue = initESClusterEnvValue + instance.Name + "-" + strconv.Itoa(i) + "." + instance.Name + "-headless-service" + "." + instance.Namespace + ".svc.cluster.local,"
		}
	}
	return initESClusterEnvValue
}

func (ownStatefulSet *OwnStatefulSet) ApplyOwnResource(instance *Elasticsearch, client client.Client, logger logr.Logger, scheme *runtime.Scheme) error {

	exist, found, err := ownStatefulSet.OwnResourceExist(instance, client, logger)
	if err != nil {
		return err
	}

	sts, err := ownStatefulSet.MakeOwnResource(instance, logger, scheme)
	if err != nil {
		return err
	}
	newStatefulSet := sts.(*appsv1.StatefulSet)
	if !exist {
		initESClusterEnvExist := false
		containerdID := 0
		for keyA,valueA := range newStatefulSet.Spec.Template.Spec.Containers{
			if valueA.Name == "elasticsearch" {
				containerdID = keyA
			}
			for keyB,_ := range valueA.Env{
				if valueA.Env[keyB].Name == "cluster.initial_master_nodes"{
					initESClusterEnvExist = true
				}
			}
		}
		if initESClusterEnvExist == false {
			initESClusterEnv := corev1.EnvVar{}
			initESClusterEnv.Name = "cluster.initial_master_nodes"
			initESClusterEnv.Value = InitESClusterEnvValue(instance)
			newStatefulSet.Spec.Template.Spec.Containers[containerdID].Env = append(newStatefulSet.Spec.Template.Spec.Containers[containerdID].Env,initESClusterEnv)
		}

		msg := fmt.Sprintf("StatefulSet %s/%s not found, create it!", newStatefulSet.Namespace, newStatefulSet.Name)
		logger.Info(msg)
		if err := client.Create(context.TODO(), newStatefulSet); err != nil {
			return err
		}
		return nil

	} else {
		foundStatefulSet := found.(*appsv1.StatefulSet)

		containerdID := 0
		for keyA,valueA := range foundStatefulSet.Spec.Template.Spec.Containers{
			if valueA.Name == "elasticsearch" {
				containerdID = keyA
			}
			for keyB,_ := range valueA.Env{
				if valueA.Env[keyB].Name == "cluster.initial_master_nodes"{
					initESClusterEnv := corev1.EnvVar{}
					initESClusterEnv.Name = "cluster.initial_master_nodes"
					initESClusterEnv.Value = valueA.Env[keyB].Value
					newStatefulSet.Spec.Template.Spec.Containers[containerdID].Env = append(newStatefulSet.Spec.Template.Spec.Containers[containerdID].Env,initESClusterEnv)
				}
			}
		}

		if !reflect.DeepEqual(newStatefulSet.Spec, foundStatefulSet.Spec) {
			msg := fmt.Sprintf("Updating StatefulSet %s/%s", newStatefulSet.Namespace, newStatefulSet.Name)
			logger.Info(msg)
			return client.Update(context.TODO(), newStatefulSet)
		}
		return nil
	}

}