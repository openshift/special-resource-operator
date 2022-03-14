package state

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/openshift/special-resource-operator/api/v1beta1"
	"github.com/openshift/special-resource-operator/pkg/clients"
	"github.com/openshift/special-resource-operator/pkg/utils"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

//go:generate mockgen -source=statusupdater.go -package=state -destination=mock_statusupdater_api.go

type StatusUpdater interface {
	UpdateWithState(context.Context, *v1beta1.SpecialResource, string)
}

type statusUpdater struct {
	kubeClient clients.ClientsInterface
	log        logr.Logger
}

func NewStatusUpdater(kubeClient clients.ClientsInterface) StatusUpdater {
	return &statusUpdater{
		kubeClient: kubeClient,
		log:        ctrl.Log.WithName(utils.Print("status-updater", utils.Blue)),
	}
}

// UpdateWithState updates sr's Status.State property with state, and updates the object in Kubernetes.
// TODO(qbarrand) make this function return an error
func (su *statusUpdater) UpdateWithState(ctx context.Context, sr *v1beta1.SpecialResource, state string) {

	update := v1beta1.SpecialResource{}

	// If we cannot find the SR than something bad is going on ..
	objectKey := types.NamespacedName{Name: sr.GetName(), Namespace: sr.GetNamespace()}
	err := su.kubeClient.Get(ctx, objectKey, &update)
	if err != nil {
		utils.WarnOnError(errors.Wrap(err, "Is SR being deleted? Cannot get current instance"))
		return
	}

	update.Status.State = state
	update.DeepCopyInto(sr)

	err = su.kubeClient.StatusUpdate(ctx, sr)
	if apierrors.IsConflict(err) {
		objectKey := types.NamespacedName{Name: sr.Name, Namespace: ""}
		err := su.kubeClient.Get(ctx, objectKey, sr)
		if apierrors.IsNotFound(err) {
			return
		}
		// Do not update the status if we're in the process of being deleted
		isMarkedToBeDeleted := sr.GetDeletionTimestamp() != nil
		if isMarkedToBeDeleted {
			return
		}

	}

	if err != nil {
		su.log.Error(err, "Failed to update SpecialResource status")
		return
	}
}
