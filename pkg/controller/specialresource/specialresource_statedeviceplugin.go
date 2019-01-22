package specialresource

import (
	"context"

	srov1alpha1 "github.com/zvonkok/special-resource-operator/pkg/apis/sro/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"k8s.io/apimachinery/pkg/types"
)

var stateDevicePluginControlFunc       []controlFunc

func CreateStateDevicePluginControl() {
	stateDevicePluginControlFunc = append(stateDevicePluginControlFunc, stateDevicePluginCtrlReference)
	stateDevicePluginControlFunc = append(stateDevicePluginControlFunc, stateDevicePluginServiceAccountCtrl)
	stateDevicePluginControlFunc = append(stateDevicePluginControlFunc, stateDevicePluginRoleCtrl)
	stateDevicePluginControlFunc = append(stateDevicePluginControlFunc, stateDevicePluginRoleBindingCtrl)
	stateDevicePluginControlFunc = append(stateDevicePluginControlFunc, stateDevicePluginDaemonSetCtrl)
}

func init() {
	CreateStateDevicePluginControl()
}


func stateDevicePluginCtrlReference(r *ReconcileSpecialResource,
	ins *srov1alpha1.SpecialResource) error {

	err := controllerutil.SetControllerReference(ins, &stateDevicePluginDecoded.serviceAccount, r.scheme)
	if err != nil {
		log.Info("Couldn't set owner references for ServiceAccount:", err)
		return err
	}
	err = controllerutil.SetControllerReference(ins, &stateDevicePluginDecoded.role, r.scheme)
	if err != nil {
	 	log.Info("Couldn't set owner references for Role:", err)
	 	return err
	}
	err = controllerutil.SetControllerReference(ins, &stateDevicePluginDecoded.roleBinding, r.scheme)
	if err != nil {
	 	log.Info("Couldn't set owner references for RoleBinding:", err)
	 	return err
	}
	err = controllerutil.SetControllerReference(ins, &stateDevicePluginDecoded.daemonSet, r.scheme)
	if err != nil {
	 	log.Info("Couldn't set owner references for DaemonSet:", err)
	 	return err
	}
	
	return nil
}



func stateDevicePluginServiceAccountCtrl(r *ReconcileSpecialResource,
	ins *srov1alpha1.SpecialResource) error {

	obj := &stateDevicePluginDecoded.serviceAccount
	found := &corev1.ServiceAccount{}
	logger := log.WithValues("Request.Namespace", obj.Namespace, "Request.Name", obj.Name)	
	
	logger.Info("Looking for ServiceAccount")
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found, creating ServiceAccount") 
		err = r.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create ServiceAccount")
			return err
		}
		return nil
	} else if err != nil {
		return err
	}
  
	logger.Info("Found ServiceAccount")
		
	return nil
}

func stateDevicePluginRoleCtrl(r *ReconcileSpecialResource,
	ins *srov1alpha1.SpecialResource) error {

	obj := &stateDevicePluginDecoded.role
	found := &rbacv1.Role{}
	logger := log.WithValues("Request.Namespace", obj.Namespace, "Request.Name", obj.Name)	
	
	logger.Info("Looking for Role")
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found, creating Role")
		err = r.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create Role")
			logger.Info(err.Error())
			return err
		}
		return nil
	} else if err != nil {
		return err
	}

	logger.Info("Found Role")
	
	return nil
}

func stateDevicePluginRoleBindingCtrl(r *ReconcileSpecialResource,
	ins *srov1alpha1.SpecialResource) error {

	obj := &stateDevicePluginDecoded.roleBinding
	found := &rbacv1.RoleBinding{}
	logger := log.WithValues("Request.Namespace", obj.Namespace, "Request.Name", obj.Name)	
	
	logger.Info("Looking for RoleBinding")
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found creating RoleBinding")
		err = r.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create RoleBinding")
			logger.Info(err.Error())
			return err
		}
		return nil
	} else if err != nil {
		return err
	}

	logger.Info("Found RoleBinding")
	
	return nil
}

func stateDevicePluginDaemonSetCtrl(r *ReconcileSpecialResource,
	ins *srov1alpha1.SpecialResource) error {

	obj := &stateDevicePluginDecoded.daemonSet
	found := &appsv1.DaemonSet{}
	logger := log.WithValues("Request.Namespace", obj.Namespace, "Request.Name", obj.Name)	

	logger.Info("Looking for DaemonSet")
	logger.Info(string(stateDevicePluginManifests.daemonSet))

	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found creating DaemonSet")
		err = r.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create DaemonSet")
			logger.Info(err.Error())
			return err
		}
		return nil
	} else if err != nil {
		return err
	}

	logger.Info("Found DaemonSet")

	return nil
}
