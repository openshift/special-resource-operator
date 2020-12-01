package controllers

import (
	"context"

	"encoding/json"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func getPromURLPass(obj *unstructured.Unstructured, r *SpecialResourceReconciler) (string, string, error) {

	promURL := ""
	promPass := ""

	grafSecret, err := kubeclient.CoreV1().Secrets("openshift-monitoring").Get(context.TODO(), "grafana-datasources", metav1.GetOptions{})
	if err != nil {
		log.Error(err, "")
		return promURL, promPass, err
	}

	promJSON := grafSecret.Data["prometheus.yaml"]

	sec := &unstructured.Unstructured{}

	if err := json.Unmarshal(promJSON, &sec.Object); err != nil {
		log.Error(err, "UnmarshlJSON")
		return promURL, promPass, err
	}

	datasources, found, err := unstructured.NestedSlice(sec.Object, "datasources")
	exitOnErrorOrNotFound(found, err)

	for _, datasource := range datasources {
		switch datasource := datasource.(type) {
		case map[string]interface{}:
			promURL, found, err = unstructured.NestedString(datasource, "url")
			exitOnErrorOrNotFound(found, err)
			promPass, found, err = unstructured.NestedString(datasource, "basicAuthPassword")
			exitOnErrorOrNotFound(found, err)
		default:
			log.Info("PROM", "DEFAULT NOT THE CORRECT TYPE", promURL)
		}
		break
	}

	return promURL, promPass, nil
}

func customGrafanaConfigMap(obj *unstructured.Unstructured, r *SpecialResourceReconciler) error {

	promData, found, err := unstructured.NestedString(obj.Object, "data", "ocp-prometheus.yml")
	exitOnErrorOrNotFound(found, err)

	promURL, promPass, err := getPromURLPass(obj, r)
	if err != nil {
		return err
	}

	promData = strings.Replace(promData, "REPLACE_PROM_URL", promURL, -1)
	promData = strings.Replace(promData, "REPLACE_PROM_PASS", promPass, -1)
	promData = strings.Replace(promData, "REPLACE_PROM_USER", "internal", -1)

	//log.Info("PROM", "DATA", promData)
	if err := unstructured.SetNestedField(obj.Object, promData, "data", "ocp-prometheus.yml"); err != nil {
		log.Error(err, "Couldn't update ocp-prometheus.yml")
		return err
	}

	return nil
}
