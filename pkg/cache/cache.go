package cache

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	log logr.Logger
)

func init() {
	log = zap.New(zap.UseDevMode(true)).WithName(color.Print("cache", color.Brown))
}

var Node NodesCache

func init() {
	Node.Count = 0xDEADBEEF
	Node.List = &unstructured.UnstructuredList{
		Object: map[string]interface{}{},
		Items:  []unstructured.Unstructured{},
	}
}

type NodesCache struct {
	List  *unstructured.UnstructuredList
	Count int64
}

func Nodes(matchingLabels map[string]string, force bool) error {

	// The initial list is what we're working with
	// a SharedInformer will update the list of nodes if
	// more nodes join the cluster.
	cached := int64(len(Node.List.Items))
	if cached == Node.Count && !force {
		return nil
	}

	Node.List.SetAPIVersion("v1")
	Node.List.SetKind("NodeList")

	opts := []client.ListOption{}

	// Only filter if we have a selector set, otherwise zero nodes will be
	// returned and no labels can be extracted. Set the default worker label
	// otherwise.
	if len(matchingLabels) > 0 {
		opts = append(opts, client.MatchingLabels(matchingLabels))
	} else {
		opts = append(opts, client.MatchingLabels{"node-role.kubernetes.io/worker": ""})
	}

	err := clients.Interface.List(context.TODO(), Node.List, opts...)
	if err != nil {
		return errors.Wrap(err, "Client cannot get NodeList")
	}

	log.Info("Node list:", "length", len(Node.List.Items))
	if len(Node.List.Items) == 0 {
		log.Info("No nodes found for the SpecialResource. Consider setting .Spec.Node.Selector in the CR or labeling worker nodes.")
	}

	log.Info("Nodes", "num", len(Node.List.Items))

	return err
}
