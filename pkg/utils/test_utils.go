package utils

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
)

func CreateNodesList(numNodes int, labels map[string]string) *corev1.NodeList {
	nodesList := corev1.NodeList{}
	nodesList.Items = make([]corev1.Node, numNodes)
	for i := range nodesList.Items {
		nodesList.Items[i].APIVersion = "v1"
		nodesList.Items[i].Kind = "Node"
		nodesList.Items[i].Name = fmt.Sprintf("Node%d", i)
		nodesList.Items[i].SetLabels(labels)
	}

	return &nodesList
}

func SetTaint(node *corev1.Node, taintKey, taintValue string, taintEffect corev1.TaintEffect) {
	var newTaints []corev1.Taint
	newTaints = append(newTaints, corev1.Taint{Key: taintKey, Value: taintValue, Effect: taintEffect})
	node.Spec.Taints = newTaints
}
