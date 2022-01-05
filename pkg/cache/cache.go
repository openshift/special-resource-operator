package cache

import (
	"context"

	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/utils"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type NodesCache struct {
	List  *unstructured.UnstructuredList
	Count int64
}

var (
	log  = zap.New(zap.UseDevMode(true)).WithName(utils.Print("cache", utils.Brown))
	Node = NodesCache{
		List: &unstructured.UnstructuredList{
			Object: map[string]interface{}{},
			Items:  []unstructured.Unstructured{},
		},
		Count: 0xDEADBEEF,
	}
)

func Nodes(ctx context.Context, matchingLabels map[string]string, force bool) error {

	// The initial list is what we're working with
	// a SharedInformer will update the list of nodes if
	// more nodes join the cluster.
	cached := int64(len(Node.List.Items))
	if cached == Node.Count && !force {
		return nil
	}

	Node.List.SetAPIVersion("v1")
	Node.List.SetKind("NodeList")

	// First check if we have nodeSelectors set and only include those nodes
	// Otherwise select all nodes without NoExecute and NoSchedule taint.
	opts := []client.ListOption{}
	if len(matchingLabels) > 0 {
		opts = append(opts, client.MatchingLabels(matchingLabels))
	}

	// TODO(qbarrand): use a v1.NodeList?
	var list unstructured.UnstructuredList
	list.SetAPIVersion("v1")
	list.SetKind("NodeList")

	err := clients.Interface.List(ctx, &list, opts...)
	if err != nil {
		return errors.Wrap(err, "Client cannot get NodeList")
	}

	Node.List.Object = list.Object
	Node.List.Items = []unstructured.Unstructured{}

	// Filter all nodes out that have NoExecute or NoSchedule taint
	for idx, node := range list.Items {

		taints, ok, err := unstructured.NestedSlice(node.Object, "spec", "taints")
		if err != nil {
			utils.WarnOnError(err)
			return errors.Wrap(err, "Cannot extract taints from Node object")
		}
		// Nothing to filter no taints on the current node object, continue
		if !ok {
			Node.List.Items = append(Node.List.Items, list.Items[idx])
			log.Info("Nodes cached", "name", node.GetName())
			continue
		}

		keep := true

		for _, taint := range taints {

			effect, ok, err := unstructured.NestedString(taint.(map[string]interface{}), "effect")
			if err != nil {
				utils.WarnOnError(err)
				return errors.Wrap(err, "Cannot extract effect from taint object")
			}
			// No effect found continuing
			if !ok {
				continue
			}
			if effect == "NoSchedule" || effect == "NoExecute" {
				keep = false
			}
		}

		if keep {
			Node.List.Items = append(Node.List.Items, list.Items[idx])
			log.Info("Nodes cached", "name", node.GetName())
		}

	}

	log.Info("Node list:", "length", len(Node.List.Items))
	if len(Node.List.Items) == 0 {
		log.Info("No nodes found for the SpecialResource. Consider setting .Spec.Node.Selector in the CR or labeling worker nodes.")
	}

	log.Info("Nodes", "num", len(Node.List.Items))

	return err
}
