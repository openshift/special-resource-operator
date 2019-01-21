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
func stateControl(ctrlFuncs []controlFunc, r *ReconcileSpecialResource, 
	ins *srov1alpha1.SpecialResource) error {

	for _, fs := range ctrlFuncs {
		err := fs(r, ins)
                if err != nil {
                        return err
                 }
	}
	return nil
}

type controlFunc func(*ReconcileSpecialResource,*srov1alpha1.SpecialResource) error

var stateDriverControlFunc       []controlFunc
//var stateMonitoringControlFunc   []controlFunc

func CreateStateDriverControl() {
	stateDriverControlFunc = append(stateDriverControlFunc, stateDriverSetCtrlReference)
	stateDriverControlFunc = append(stateDriverControlFunc, stateDriverServiceAccountCtrl)
	stateDriverControlFunc = append(stateDriverControlFunc, stateDriverRoleCtrl)
	stateDriverControlFunc = append(stateDriverControlFunc, stateDriverRoleBindingCtrl)
	stateDriverControlFunc = append(stateDriverControlFunc, stateDriverConfigMapCtrl)
	stateDriverControlFunc = append(stateDriverControlFunc, stateDriverDaemonSetCtrl)
}
func init() {
	CreateStateDriverControl()
//	CreateStateMonitoringControl()
}

func stateDriverSetCtrlReference(r *ReconcileSpecialResource,
	ins *srov1alpha1.SpecialResource) error {

	err := controllerutil.SetControllerReference(ins, &stateDriverDecoded.serviceAccount, r.scheme)
	if err != nil {
		log.Info("Couldn't set owner references for ServiceAccount:", err)
		return err
	}
	err = controllerutil.SetControllerReference(ins, &stateDriverDecoded.role, r.scheme)
	if err != nil {
	 	log.Info("Couldn't set owner references for Role:", err)
	 	return err
	}
	err = controllerutil.SetControllerReference(ins, &stateDriverDecoded.roleBinding, r.scheme)
	if err != nil {
	 	log.Info("Couldn't set owner references for RoleBinding:", err)
	 	return err
	}
	err = controllerutil.SetControllerReference(ins, &stateDriverDecoded.configMap, r.scheme)
	if err != nil {
	 	log.Info("Couldn't set owner references for ConfigMap:", err)
	  	return err
	}
	err = controllerutil.SetControllerReference(ins, &stateDriverDecoded.daemonSet, r.scheme)
	if err != nil {
	 	log.Info("Couldn't set owner references for DaemonSet:", err)
	 	return err
	}
	
	return nil
}



func stateDriverServiceAccountCtrl(r *ReconcileSpecialResource,
	ins *srov1alpha1.SpecialResource) error {

	obj := &stateDriverDecoded.serviceAccount
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

func stateDriverRoleCtrl(r *ReconcileSpecialResource,
	ins *srov1alpha1.SpecialResource) error {

	obj := &stateDriverDecoded.role
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

func stateDriverRoleBindingCtrl(r *ReconcileSpecialResource,
	ins *srov1alpha1.SpecialResource) error {

	obj := &stateDriverDecoded.roleBinding
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

func stateDriverConfigMapCtrl(r *ReconcileSpecialResource,
	ins *srov1alpha1.SpecialResource) error {

	obj := &stateDriverDecoded.configMap
	found := &corev1.ConfigMap{}
	logger := log.WithValues("Request.Namespace", obj.Namespace, "Request.Name", obj.Name)	

	logger.Info("Looking for ConfigMap")
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found creating ConfigMap")
		err = r.client.Create(context.TODO(), obj)
		if err != nil {
			logger.Info("Couldn't create ConfigMap")
			logger.Info(err.Error())
			return err
		}
		return nil
	} else if err != nil {
		return err
	}

	logger.Info("Found ConfigMap")
	
	return nil
}

func stateDriverDaemonSetCtrl(r *ReconcileSpecialResource,
	ins *srov1alpha1.SpecialResource) error {

	obj := &stateDriverDecoded.daemonSet
	found := &appsv1.DaemonSet{}
	logger := log.WithValues("Request.Namespace", obj.Namespace, "Request.Name", obj.Name)	

	logger.Info("Looking for DaemonSet")
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
