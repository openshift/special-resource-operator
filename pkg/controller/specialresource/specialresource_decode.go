package specialresource

import (
	"log"
	kappsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	
	appsv1 "github.com/openshift/api/apps/v1"
        authorizationv1 "github.com/openshift/api/authorization/v1"
        buildv1 "github.com/openshift/api/build/v1"
        imagev1 "github.com/openshift/api/image/v1"
        networkv1 "github.com/openshift/api/network/v1"
        oauthv1 "github.com/openshift/api/oauth/v1"
        projectv1 "github.com/openshift/api/project/v1"
        quotav1 "github.com/openshift/api/quota/v1"
        routev1 "github.com/openshift/api/route/v1"
        securityv1 "github.com/openshift/api/security/v1"
        templatev1 "github.com/openshift/api/template/v1"
        userv1 "github.com/openshift/api/user/v1"
	
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
)


type StageDriverDecoded struct {
	serviceAccount corev1.ServiceAccount
	role           rbacv1.Role
	roleBinding    rbacv1.RoleBinding
	configMap      corev1.ConfigMap
	daemonSet      kappsv1.DaemonSet
}

type StageDevicePluginDecoded struct {
	serviceAccount corev1.ServiceAccount
	role           rbacv1.Role
	roleBinding    rbacv1.RoleBinding
	daemonSet      kappsv1.DaemonSet
}

type StageMonitoringDecoded struct {
	serviceAccount corev1.ServiceAccount
}

var stageDriverDecoded       StageDriverDecoded
var stageDevicePluginDecoded StageDevicePluginDecoded
var stageMonitoringDecoded   StageMonitoringDecoded

func DecodeStageDriver() {

	s := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme.Scheme,
                scheme.Scheme)
	_, _, err := s.Decode(stageDriverManifests.serviceAccount, nil, &stageDriverDecoded.serviceAccount)                      
	if err != nil { panic(err) }
	_, _, err  = s.Decode(stageDriverManifests.role, nil, &stageDriverDecoded.role) 
	if err != nil { panic(err) }
 	_, _, err  = s.Decode(stageDriverManifests.roleBinding, nil, &stageDriverDecoded.roleBinding)
	if err != nil { panic(err) }
	_, _, err  = s.Decode(stageDriverManifests.configMap, nil, &stageDriverDecoded.configMap)
	if err != nil { panic(err) }
	_, _, err  = s.Decode(stageDriverManifests.daemonSet, nil, &stageDriverDecoded.daemonSet)
	if err != nil { panic(err) }

}

func DecodeStageDevicePlugin() {
	s := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme.Scheme,
                scheme.Scheme)
	_, _, err := s.Decode(stageDevicePluginManifests.serviceAccount, nil, &stageDevicePluginDecoded.serviceAccount)                      
	if err != nil { panic(err) }
	_, _, err  = s.Decode(stageDevicePluginManifests.role, nil, &stageDevicePluginDecoded.role) 
	if err != nil { panic(err) }
 	_, _, err  = s.Decode(stageDevicePluginManifests.roleBinding, nil, &stageDevicePluginDecoded.roleBinding)
	if err != nil { panic(err) }
	_, _, err  = s.Decode(stageDevicePluginManifests.daemonSet, nil, &stageDevicePluginDecoded.daemonSet)
	if err != nil { panic(err) }
}

func DecodeStageMonitoring() {
	return
}

func init() {
	// The Kubernetes Go client (nested within the OpenShift Go client)
        // automatically registers its types in scheme.Scheme, however the
        // additional OpenShift types must be registered manually.  AddToScheme
        // registers the API group types (e.g. route.openshift.io/v1, Route) only.
        appsv1.AddToScheme(scheme.Scheme)
        authorizationv1.AddToScheme(scheme.Scheme)
        buildv1.AddToScheme(scheme.Scheme)
        imagev1.AddToScheme(scheme.Scheme)
        networkv1.AddToScheme(scheme.Scheme)
        oauthv1.AddToScheme(scheme.Scheme)
        projectv1.AddToScheme(scheme.Scheme)
        quotav1.AddToScheme(scheme.Scheme)
        routev1.AddToScheme(scheme.Scheme)
        securityv1.AddToScheme(scheme.Scheme)
        templatev1.AddToScheme(scheme.Scheme)
        userv1.AddToScheme(scheme.Scheme)

	GenerateStageDriverManifests()
	GenerateStageDevicePluginManifests()
	GenerateStageMonitoringManifests()

	DecodeStageDriver()
	DecodeStageDevicePlugin()
	DecodeStageMonitoring()
}
