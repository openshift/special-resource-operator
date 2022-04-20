package watcher

import (
	context "context"
	"fmt"
	"strings"
	"testing"

	srov1beta1 "github.com/openshift/special-resource-operator/api/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	reconcile "sigs.k8s.io/controller-runtime/pkg/reconcile"
	source "sigs.k8s.io/controller-runtime/pkg/source"

	gomock "github.com/golang/mock/gomock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestWatcher(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Watcher Suite")
}

var _ = Describe("Watcher", func() {
	var (
		mockCtrl       *gomock.Controller
		mockController *MockController
		w              Watcher
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockController = NewMockController(mockCtrl)

		mockController.EXPECT().GetLogger().Return(ctrl.LoggerFrom(context.Background()))
		w = New(mockController)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	clusterVersionWatch := srov1beta1.SpecialResourceModuleWatch{
		ApiVersion: "config.openshift.io/v1",
		Kind:       "ClusterVersion",
		Name:       "version",
		Namespace:  "",
		Path:       "$.status.history[*].image",
	}
	clusterVersion := &unstructured.Unstructured{}
	clusterVersion.SetAPIVersion(clusterVersionWatch.ApiVersion)
	clusterVersion.SetKind(clusterVersionWatch.Kind)
	clusterVersion.SetName(clusterVersionWatch.Name)

	history := []interface{}{
		map[string]interface{}{"image": "some-registry.dummy/cluster/image:sha456"},
		map[string]interface{}{"image": "some-registry.dummy/cluster/image:sha123"},
	}
	err := unstructured.SetNestedField(clusterVersion.Object, history, "status", "history")
	Expect(err).ToNot(HaveOccurred())

	dummyWatch := srov1beta1.SpecialResourceModuleWatch{
		ApiVersion: "some.api.io/v1",
		Kind:       "SpokeVersion",
		Name:       "version",
		Namespace:  "",
		Path:       "$.status.image",
	}
	dummyImage := "another-registry.dummy/cluster/image:sha0987654321"
	dummy := &unstructured.Unstructured{}
	dummy.SetAPIVersion(dummyWatch.ApiVersion)
	dummy.SetKind(dummyWatch.Kind)
	dummy.SetName(dummyWatch.Name)
	err = unstructured.SetNestedField(dummy.Object, dummyImage,
		strings.Split(strings.TrimPrefix(dummyWatch.Path, "$."), ".")...)

	Expect(err).ToNot(HaveOccurred())

	srm1 := srov1beta1.SpecialResourceModule{
		ObjectMeta: v1.ObjectMeta{
			Name:      "kmod1",
			Namespace: "",
		},
		Spec: srov1beta1.SpecialResourceModuleSpec{
			Watch: []srov1beta1.SpecialResourceModuleWatch{
				clusterVersionWatch,
			},
		},
	}

	srm2 := srov1beta1.SpecialResourceModule{
		ObjectMeta: v1.ObjectMeta{
			Name:      "kmod2",
			Namespace: "sro",
		},
		Spec: srov1beta1.SpecialResourceModuleSpec{
			Watch: []srov1beta1.SpecialResourceModuleWatch{
				clusterVersionWatch,
				dummyWatch,
			},
		},
	}

	Context("expected flow in runtime", func() {
		It("should work accordingly", func() {
			// Watch() for srm1
			mockController.EXPECT().Watch(gvkMatcherFromUnstructured(*clusterVersion), gomock.Any()).Return(nil)
			// Watch()-es for srm2
			// Watch() for ClusterVersion is called again because isAlreadyBeingWatched() checks strictly (including SRM CR to trigger)
			// This could be improved with not checking "CRs to trigger" but still adding the "CR to trigger" if already watched
			mockController.EXPECT().Watch(gvkMatcherFromUnstructured(*clusterVersion), gomock.Any()).Return(nil)
			mockController.EXPECT().Watch(gvkMatcherFromUnstructured(*dummy), gomock.Any()).Return(nil)

			By("adding watches for SpecialResourceModule")
			err := w.ReconcileWatches(context.Background(), srm1)
			Expect(err).ToNot(HaveOccurred())

			By("simulating update of watched ClusterVersion")
			requests := w.(*watcher).mapper(clusterVersion)
			Expect(requests).ToNot(BeEmpty())
			Expect(requests).To(ContainElements(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: srm1.Namespace, Name: srm1.Name}}))

			By("simulating update of watched ClusterVersion, but the observed data did not change")
			requests = w.(*watcher).mapper(clusterVersion)
			Expect(requests).To(BeEmpty())

			By("adding watches for another SpecialResourceModule")
			err = w.ReconcileWatches(context.Background(), srm2)
			Expect(err).ToNot(HaveOccurred())

			By("simulating update of watched ClusterVersion with new data")
			requests = w.(*watcher).mapper(clusterVersion)
			// Potential gap: a new SRM was added with a resource to watched that was already watches.
			// Since the resource did not change, the Reconcile won't be triggered for either srm1 or srm2.
			// This shouldn't be a problem if new SRM will have kmod built upon creation (of the SRM CR).
			// If this is not desired, then pathData must be changed from data+[]paths to []{data, path}.
			Expect(requests).To(BeEmpty())

			By("simulating update of watched fake SpokeVersion")
			requests = w.(*watcher).mapper(dummy)
			Expect(requests).ToNot(BeEmpty())
			Expect(requests).To(ContainElements(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: srm2.Namespace, Name: srm2.Name}}))

			By("simulating update of watched ClusterVersion with the same data but in different order")
			history := []interface{}{
				map[string]interface{}{"image": "some-registry.dummy/cluster/image:sha123"},
				map[string]interface{}{"image": "some-registry.dummy/cluster/image:sha456"},
			}

			err = unstructured.SetNestedField(clusterVersion.Object, history, "status", "history")
			Expect(err).ToNot(HaveOccurred())

			requests = w.(*watcher).mapper(clusterVersion)
			Expect(requests).To(BeEmpty())

			By("simulating update of watched ClusterVersion with new data")
			history = []interface{}{
				map[string]interface{}{"image": "some-registry.dummy/cluster/image:sha123"},
				map[string]interface{}{"image": "some-registry.dummy/cluster/image:sha789"},
			}

			err = unstructured.SetNestedField(clusterVersion.Object, history, "status", "history")
			Expect(err).ToNot(HaveOccurred())

			requests = w.(*watcher).mapper(clusterVersion)
			Expect(requests).ToNot(BeEmpty())
			Expect(requests).To(ContainElements(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: srm1.Namespace, Name: srm1.Name}}))
			Expect(requests).To(ContainElements(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: srm2.Namespace, Name: srm2.Name}}))

			By("removing one of the SpecialResourceModules")
			srm2.Spec.Watch = make([]srov1beta1.SpecialResourceModuleWatch, 0)
			err = w.ReconcileWatches(context.Background(), srm2)
			Expect(err).ToNot(HaveOccurred())

			By("simulating update of watched ClusterVersion, but the observed data changed again")
			history = []interface{}{
				map[string]interface{}{"image": "some-registry.dummy/cluster/image:sha321"},
			}
			err = unstructured.SetNestedField(clusterVersion.Object, history, "status", "history")
			Expect(err).ToNot(HaveOccurred())

			requests = w.(*watcher).mapper(clusterVersion)
			Expect(requests).ToNot(BeEmpty())
			Expect(requests).To(ContainElements(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: srm1.Namespace, Name: srm1.Name}}))
			Expect(requests).ToNot(ContainElements(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: srm2.Namespace, Name: srm2.Name}}))
		})
	})
})

type gvkMatcher struct {
	ApiVersion string
	Kind       string
}

func gvkMatcherFromUnstructured(u unstructured.Unstructured) gvkMatcher {
	return gvkMatcher{
		ApiVersion: u.GetAPIVersion(),
		Kind:       u.GetKind(),
	}
}

func (m gvkMatcher) Matches(x interface{}) bool {
	if s, ok := x.(*source.Kind); ok {
		gvk := s.Type.GetObjectKind().GroupVersionKind()
		return m.ApiVersion == getAPIVersion(gvk) && m.Kind == gvk.Kind
	}
	return false
}

func (m gvkMatcher) String() string {
	return fmt.Sprintf("is equal to { ApiVersion: %v, Kind: %v }", m.ApiVersion, m.Kind)
}
