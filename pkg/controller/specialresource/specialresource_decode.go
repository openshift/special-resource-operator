package specialresource

import (
	"fmt"
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


type StateDriverDecoded struct {
	serviceAccount corev1.ServiceAccount
	role           rbacv1.Role
	roleBinding    rbacv1.RoleBinding
	configMap      corev1.ConfigMap
	daemonSet      kappsv1.DaemonSet
}

type StateDevicePluginDecoded struct {
	serviceAccount corev1.ServiceAccount
	role           rbacv1.Role
	roleBinding    rbacv1.RoleBinding
	daemonSet      kappsv1.DaemonSet
}

type StateMonitoringDecoded struct {
	serviceAccount corev1.ServiceAccount
}

var stateDriverDecoded       StateDriverDecoded
var stateDevicePluginDecoded StateDevicePluginDecoded
var stateMonitoringDecoded   StateMonitoringDecoded

func DecodeStateDriver() {

	s := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme.Scheme,
                scheme.Scheme)
	_, _, err := s.Decode(stateDriverManifests.serviceAccount, nil, &stateDriverDecoded.serviceAccount)                      
	if err != nil { panic(err) }
	_, _, err  = s.Decode(stateDriverManifests.role, nil, &stateDriverDecoded.role) 
	if err != nil { panic(err) }
 	_, _, err  = s.Decode(stateDriverManifests.roleBinding, nil, &stateDriverDecoded.roleBinding)
	if err != nil { panic(err) }
	_, _, err  = s.Decode(stateDriverManifests.configMap, nil, &stateDriverDecoded.configMap)
	if err != nil { panic(err) }
	_, _, err  = s.Decode(stateDriverManifests.daemonSet, nil, &stateDriverDecoded.daemonSet)
	if err != nil { panic(err) }
}

func DecodeStateDevicePlugin() {
	s := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme.Scheme,
                scheme.Scheme)
	_, _, err := s.Decode(stateDevicePluginManifests.serviceAccount, nil, &stateDevicePluginDecoded.serviceAccount)                      
	if err != nil { panic(err) }
	_, _, err  = s.Decode(stateDevicePluginManifests.role, nil, &stateDevicePluginDecoded.role) 
	if err != nil { panic(err) }
 	_, _, err  = s.Decode(stateDevicePluginManifests.roleBinding, nil, &stateDevicePluginDecoded.roleBinding)
	if err != nil { panic(err) }
	_, _, err  = s.Decode(stateDevicePluginManifests.daemonSet, nil, &stateDevicePluginDecoded.daemonSet)
	if err != nil { panic(err) }
}

func DecodeStateMonitoring() {
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

	GenerateStateDriverManifests()
	GenerateStateDevicePluginManifests()
	GenerateStateMonitoringManifests()

	DecodeStateDriver()
	DecodeStateDevicePlugin()
	DecodeStateMonitoring()
}
