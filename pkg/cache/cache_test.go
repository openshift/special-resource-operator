package cache_test

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-psap/special-resource-operator/pkg/cache"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCache(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cache Suite")
}

var _ = Describe("Nodes", func() {
	var (
		ctrl        *gomock.Controller
		mockClients *clients.MockClientsInterface
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockClients = clients.NewMockClientsInterface(ctrl)

		clients.Interface = mockClients
	})

	AfterEach(func() {
		ctrl.Finish()

		// reset globals
		clients.Interface = nil

		cache.Node.Count = 0xDEADBEEF
		cache.Node.List = &unstructured.UnstructuredList{
			Object: map[string]interface{}{},
			Items:  []unstructured.Unstructured{},
		}
	})

	listMatcher := gomock.AssignableToTypeOf(&unstructured.UnstructuredList{})

	Context("machingLabels=nil", func() {
		DescribeTable(
			"should return an error when the Kubernetes client fails",
			func(force bool) {
				randomError := errors.New("random error")

				mockClients.EXPECT().List(context.TODO(), listMatcher).Return(randomError)

				err := cache.Nodes(context.TODO(), nil, force)

				Expect(errors.Is(err, randomError)).To(BeTrue())
			},
			EntryDescription("force=%t"),
			Entry(nil, true),
			Entry(nil, false),
		)

		DescribeTable("force=false, one node with TaintEffect",
			func(te v1.TaintEffect, cacheCount int) {
				const validNodeName = "node1"

				mockClients.EXPECT().List(context.TODO(), listMatcher).Do(
					func(_ context.Context, l *unstructured.UnstructuredList) {
						validNode := unstructured.Unstructured{}
						validNode.SetName(validNodeName)

						taintedNode := unstructured.Unstructured{
							Object: map[string]interface{}{
								"spec": map[string]interface{}{
									"taints": []interface{}{
										map[string]interface{}{"effect": string(te)},
									},
								},
							},
						}

						l.Items = []unstructured.Unstructured{validNode, taintedNode}
					},
				)

				err := cache.Nodes(context.TODO(), nil, false)

				Expect(err).NotTo(HaveOccurred())
				Expect(cache.Node.List.Items).To(HaveLen(cacheCount))
				Expect(cache.Node.List.Items[0].GetName()).To(Equal(validNodeName))
			},
			EntryDescription("taint=%s, nodes=%d"),
			Entry(nil, v1.TaintEffect(""), 2),
			Entry(nil, v1.TaintEffectPreferNoSchedule, 2),
			Entry(nil, v1.TaintEffectNoExecute, 1),
			Entry(nil, v1.TaintEffectNoSchedule, 1),
		)

		Context("valid cache", func() {
			It("should not be updated the cache when force=false", func() {
				cache.Node = cache.NodesCache{
					Count: 3,
					List: &unstructured.UnstructuredList{
						Items: make([]unstructured.Unstructured, 3),
					},
				}

				err := cache.Nodes(context.TODO(), nil, false)

				Expect(err).NotTo(HaveOccurred())
				Expect(cache.Node.List.Items).To(
					HaveLen(3),
					"still 3 nodes cached at the end, although Kubernetes has only 2",
				)
			})

			It("should be updated the cache when force=true", func() {
				cache.Node = cache.NodesCache{
					Count: 3,
					List: &unstructured.UnstructuredList{
						Items: make([]unstructured.Unstructured, 3),
					},
				}

				k8sItems := make([]unstructured.Unstructured, 2)

				mockClients.EXPECT().List(context.TODO(), listMatcher).Do(
					func(_ context.Context, l *unstructured.UnstructuredList) {
						l.Items = k8sItems
					},
				)

				err := cache.Nodes(context.TODO(), nil, true)

				Expect(err).NotTo(HaveOccurred())
				Expect(cache.Node.List.Items).To(
					HaveLen(len(k8sItems)),
					"make sure the cache was updated with what Kubernetes has",
				)
			})
		})

		DescribeTable(
			"invalid cache should always be updated",
			func(force bool) {
				cache.Node = cache.NodesCache{
					Count: 4,
					List: &unstructured.UnstructuredList{
						Items: make([]unstructured.Unstructured, 3),
					},
				}

				k8sItems := make([]unstructured.Unstructured, 2)

				mockClients.EXPECT().List(context.TODO(), listMatcher).Do(
					func(_ context.Context, l *unstructured.UnstructuredList) {
						l.Items = k8sItems
					},
				)

				err := cache.Nodes(context.TODO(), nil, force)
				Expect(err).NotTo(HaveOccurred())
				Expect(
					cache.Node.List.Items).To(HaveLen(len(k8sItems)),
					"with the discrepancy between Node.Count and Node.List, the cache should always be updated",
				)
			},
			EntryDescription("force=%t"),
			Entry(nil, false),
			Entry(nil, true),
		)
	})

	It("should work as expected when matchingLabels are defined, force=false, no taints", func() {
		matchingLabels := map[string]string{"test-label": "test-value"}

		const validNodeName = "node1"

		opt := client.MatchingLabels(matchingLabels)

		mockClients.EXPECT().List(context.TODO(), listMatcher, opt).Do(
			func(_ context.Context, l *unstructured.UnstructuredList, _ client.ListOption) {
				validNode := unstructured.Unstructured{}
				validNode.SetName(validNodeName)

				l.Items = []unstructured.Unstructured{validNode}
			},
		)

		err := cache.Nodes(context.TODO(), matchingLabels, false)

		Expect(err).NotTo(HaveOccurred())
		Expect(cache.Node.List.Items).To(HaveLen(1))
		Expect(cache.Node.List.Items[0].GetName()).To(Equal(validNodeName))
	})
})
