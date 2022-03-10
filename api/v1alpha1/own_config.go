package v1alpha1

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type OwnConfig struct {

	Data map[string]string `json:"-"`

}

var ElasticSearchNotSetAuthCommonConfigs = map[string]string{

	"cluster.name": "${CLUSTER_NAME}",
	"cluster.routing.allocation.awareness.attributes": "k8s_node_name",

	"http.publish_host": "${POD_NAME}.${HEADLESS_SERVICE_NAME}.${NAMESPACE}.svc.cluster.local",
	"http.cors.enabled": "true",
	"http.cors.allow-origin": "*",

	"index.store.type": "niofs",

	"network.host": "0.0.0.0",
	"network.publish_host": "${POD_IP}",

	"node.attr.k8s_node_name": "${POD_NAME}.${HEADLESS_SERVICE_NAME}.${NAMESPACE}.svc.cluster.local",
	"node.name": "${POD_NAME}.${HEADLESS_SERVICE_NAME}.${NAMESPACE}.svc.cluster.local",
	"node.store.allow_mmap": "false",
	"node.roles": "master, data, ingest, ml",

	"path.data": "/usr/share/elasticsearch/data",
	"path.logs": "/usr/share/elasticsearch/logs",
	"ingest.geoip.downloader.enabled": "false",
	"xpack.security.enabled": "false",

}

func (ownConfig *OwnConfig) MakeOwnResource(instance *Elasticsearch, logger logr.Logger, scheme *runtime.Scheme) (interface{}, error) {
	// new a StatefulSet object
	config := ElasticSearchNotSetAuthCommonConfigs
	if instance.Spec.Config != nil {
		for key,value := range instance.Spec.Config.Data{
			if _,ok := config[key]; !ok{
				config[key] = value
			}else {
				config[key] = value
			}
		}
	}
	configMapData := make(map[string]string)
	dataType , err := json.Marshal(config)
	if err != nil {
		logger.Error(err, "config switch error...")
	}
	dataString := string(dataType)
	configMapData["elasticsearch.yml"] = dataString
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-cm",
			Namespace: instance.Namespace,
			Labels:    labelsSelectorForElastic(instance.Name).MatchLabels},
	}
	cm.Data = configMapData

	if err := controllerutil.SetControllerReference(instance, cm, scheme); err != nil {
		msg := fmt.Sprintf("set controllerReference for ConfigMap %s/%s failed", instance.Namespace, instance.Name)
		logger.Error(err, msg)
		return nil, err
	}

	return cm, nil

}

func (ownConfig *OwnConfig) OwnResourceExist(instance *Elasticsearch, client client.Client, logger logr.Logger) (bool, interface{}, error) {

	found := &corev1.ConfigMap{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: instance.Name + "-cm", Namespace: instance.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		msg := fmt.Sprintf("ConfigMap %s/%s found, but with error", instance.Namespace, instance.Name)
		logger.Error(err, msg)
		return true, found, err
	}
	return true, found, nil

}

func (ownConfig *OwnConfig) ApplyOwnResource(instance *Elasticsearch, client client.Client, logger logr.Logger, scheme *runtime.Scheme) error {

	// assert if StatefulSet exist
	exist, found, err := ownConfig.OwnResourceExist(instance, client, logger)
	if err != nil {
		return err
	}

	// make StatefulSet object
	configmap, err := ownConfig.MakeOwnResource(instance, logger, scheme)
	if err != nil {
		return err
	}
	newConfigmap := configmap.(*corev1.ConfigMap)
	// apply the StatefulSet object just make
	if !exist {
		// if StatefulSet not exist，then create it
		msg := fmt.Sprintf("ConfigMap %s/%s not found, create it!", newConfigmap.Namespace, newConfigmap.Name)
		logger.Info(msg)
		if err := client.Create(context.TODO(), newConfigmap); err != nil {
			return err
		}
		return nil

	} else {
		foundConfigmap := found.(*corev1.ConfigMap)

		// if StatefulSet exist with change，then try to update it
		if !reflect.DeepEqual(newConfigmap.Data, foundConfigmap.Data) {
			msg := fmt.Sprintf("Updating StatefulSet %s/%s", newConfigmap.Namespace, newConfigmap.Name)
			logger.Info(msg)
			return client.Update(context.TODO(), newConfigmap)
		}
		return nil
	}

}