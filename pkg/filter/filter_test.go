package filter

import (
	"io/ioutil"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"

	"github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/kernel"
	"github.com/openshift-psap/special-resource-operator/pkg/lifecycle"
	"github.com/openshift-psap/special-resource-operator/pkg/storage"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestFilter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Filter Suite")
}

var _ = Describe("IsSpecialResource", func() {
	var (
		ctrl          *gomock.Controller
		mockLifecycle *lifecycle.MockLifecycle
		mockStorage   *storage.MockStorage
		mockKernel    *kernel.MockKernelData
		f             filter
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockLifecycle = lifecycle.NewMockLifecycle(ctrl)
		mockStorage = storage.NewMockStorage(ctrl)
		mockKernel = kernel.NewMockKernelData(ctrl)
		f = filter{
			log:        zap.New(zap.WriteTo(ioutil.Discard)),
			lifecycle:  mockLifecycle,
			storage:    mockStorage,
			kernelData: mockKernel,
		}
	})

	cases := []struct {
		name    string
		obj     client.Object
		matcher types.GomegaMatcher
	}{
		{
			name: Kind,
			obj: &v1beta1.SpecialResource{
				TypeMeta: metav1.TypeMeta{Kind: Kind},
			},
			matcher: BeTrue(),
		},
		{
			name: "Pod owned by SRO",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{OwnedLabel: "true"},
				},
			},
			matcher: BeFalse(),
		},
		{
			name: "valid selflink",
			obj: func() *unstructured.Unstructured {
				uo := &unstructured.Unstructured{}
				uo.SetSelfLink("/apis/sro.openshift.io/v1")

				return uo
			}(),
			matcher: BeTrue(),
		},
		{
			name: "selflink in Label",
			obj: func() *unstructured.Unstructured {
				uo := &unstructured.Unstructured{}
				uo.SetLabels(map[string]string{"some-label": "/apis/sro.openshift.io/v1"})

				return uo
			}(),
			matcher: BeTrue(),
		},
		{
			name:    "no selflink",
			obj:     &unstructured.Unstructured{},
			matcher: BeFalse(),
		},
	}

	entries := make([]TableEntry, 0, len(cases))

	for _, c := range cases {
		entries = append(entries, Entry(c.name, c.obj, c.matcher))
	}

	DescribeTable(
		"should return the correct value",
		func(obj client.Object, m types.GomegaMatcher) {
			Expect(f.isSpecialResource(obj)).To(m)
		},
		entries...)
})

var _ = Describe("Owned", func() {
	var (
		ctrl          *gomock.Controller
		mockLifecycle *lifecycle.MockLifecycle
		mockStorage   *storage.MockStorage
		mockKernel    *kernel.MockKernelData
		f             filter
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockLifecycle = lifecycle.NewMockLifecycle(ctrl)
		mockStorage = storage.NewMockStorage(ctrl)
		mockKernel = kernel.NewMockKernelData(ctrl)
		f = filter{
			log:        zap.New(zap.WriteTo(ioutil.Discard)),
			lifecycle:  mockLifecycle,
			storage:    mockStorage,
			kernelData: mockKernel,
		}
	})

	cases := []struct {
		name    string
		obj     client.Object
		matcher types.GomegaMatcher
	}{
		{
			name: "via ownerReferences",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{Kind: Kind},
					},
				},
			},
			matcher: BeTrue(),
		},
		{
			name: "via labels",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{OwnedLabel: "whatever"},
				},
			},
			matcher: BeTrue(),
		},
		{
			name:    "not owned",
			obj:     &corev1.Pod{},
			matcher: BeFalse(),
		},
	}

	entries := make([]TableEntry, 0, len(cases))

	for _, c := range cases {
		entries = append(entries, Entry(c.name, c.obj, c.matcher))
	}

	DescribeTable(
		"should return the expected value",
		func(obj client.Object, m types.GomegaMatcher) {
			Expect(f.owned(obj)).To(m)
		},
		entries...,
	)
})

var _ = Describe("Predicate", func() {
	var (
		ctrl          *gomock.Controller
		mockLifecycle *lifecycle.MockLifecycle
		mockStorage   *storage.MockStorage
		mockKernel    *kernel.MockKernelData
		f             filter
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockLifecycle = lifecycle.NewMockLifecycle(ctrl)
		mockStorage = storage.NewMockStorage(ctrl)
		mockKernel = kernel.NewMockKernelData(ctrl)
		f = filter{
			log:        zap.New(zap.WriteTo(ioutil.Discard)),
			lifecycle:  mockLifecycle,
			storage:    mockStorage,
			kernelData: mockKernel,
		}
	})

	Context("CreateFunc", func() {
		cases := []struct {
			name       string
			obj        client.Object
			retMatcher types.GomegaMatcher
		}{
			{
				name:       "special resource",
				obj:        &v1beta1.SpecialResource{},
				retMatcher: BeTrue(),
			},
			{
				name: "owned",
				obj: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{Kind: Kind},
						},
					},
				},
				retMatcher: BeTrue(),
			},
			{
				name:       "random pod",
				obj:        &corev1.Pod{},
				retMatcher: BeFalse(),
			},
		}

		entries := make([]TableEntry, 0, len(cases))

		for _, c := range cases {
			entries = append(entries, Entry(c.name, c.obj, c.retMatcher))
		}

		DescribeTable(
			"should work as expected",
			func(obj client.Object, m types.GomegaMatcher) {
				ret := f.GetPredicates().Create(event.CreateEvent{Object: obj})

				Expect(ret).To(m)
				Expect(f.GetMode()).To(Equal("CREATE"))
			},
			entries...,
		)
	})

	Context("UpdateFunc", func() {

		cases := []struct {
			name       string
			mockSetup  func()
			old        client.Object
			new        client.Object
			retMatcher types.GomegaMatcher
		}{
			{
				name: "No change to object's Generation or ResourceVersion",
				mockSetup: func() {
					mockKernel.EXPECT().IsObjectAffine(gomock.Any()).Times(1).Return(false)
				},
				old: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{Kind: Kind},
						},
						Generation:      1,
						ResourceVersion: "dummy1",
					},
				},
				new: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{Kind: Kind},
						},
						Generation:      1,
						ResourceVersion: "dummy1",
					},
				},
				retMatcher: BeFalse(),
			},
			{
				name: "Object's Generation changed, no change to ResourceVersion",
				mockSetup: func() {
					mockKernel.EXPECT().IsObjectAffine(gomock.Any()).Times(1).Return(false)
				},
				old: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{Kind: Kind},
						},
						Generation:      1,
						ResourceVersion: "dummy1",
					},
				},
				new: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{Kind: Kind},
						},
						Generation:      2,
						ResourceVersion: "dummy1",
					},
				},
				retMatcher: BeFalse(),
			},
			{
				name: "Object has changed but is not owned by SRO",
				mockSetup: func() {
					mockKernel.EXPECT().IsObjectAffine(gomock.Any()).Times(1).Return(false)
				},
				old: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Generation:      1,
						ResourceVersion: "dummy1",
					},
				},
				new: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Generation:      2,
						ResourceVersion: "dummy2",
					},
				},
				retMatcher: BeFalse(),
			},
			{
				name: "Object has changed and it's a SRO owned DaemonSet",
				mockSetup: func() {
					mockKernel.EXPECT().IsObjectAffine(gomock.Any()).Times(1).Return(true)
					mockLifecycle.EXPECT().UpdateDaemonSetPods(gomock.Any()).Return(nil)
				},
				old: &appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{Kind: Kind},
						},
						Generation:      1,
						ResourceVersion: "dummy1",
					},
				},
				new: &appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{Kind: Kind},
						},
						Generation:      2,
						ResourceVersion: "dummy2",
					},
				},
				retMatcher: BeTrue(),
			},
			{
				name: "Object is a SRO owned & kernel affine DaemonSet, but did not change",
				mockSetup: func() {
					mockKernel.EXPECT().IsObjectAffine(gomock.Any()).Times(1).Return(true)
				},
				old: &appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{Kind: Kind},
						},
						Annotations: map[string]string{
							"specialresource.openshift.io/kernel-affine": "true",
						},
						Generation:      0,
						ResourceVersion: "dummy",
					},
				},
				new: &appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{Kind: Kind},
						},
						Annotations: map[string]string{
							"specialresource.openshift.io/kernel-affine": "true",
						},
						Generation:      0,
						ResourceVersion: "dummy",
					},
				},
				retMatcher: BeFalse(),
			},
			{
				name: "Object is a SRO owned & kernel affine DaemonSet",
				mockSetup: func() {
					mockKernel.EXPECT().IsObjectAffine(gomock.Any()).Times(1).Return(true)
					mockLifecycle.EXPECT().UpdateDaemonSetPods(gomock.Any()).Return(nil)
				},
				old: &appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{Kind: Kind},
						},
						Annotations: map[string]string{
							"specialresource.openshift.io/kernel-affine": "true",
						},
						Generation:      0,
						ResourceVersion: "dummy",
					},
				},
				new: &appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{Kind: Kind},
						},
						Annotations: map[string]string{
							"specialresource.openshift.io/kernel-affine": "true",
						},
						Generation:      1,
						ResourceVersion: "dummy",
					},
				},
				retMatcher: BeTrue(),
			},
			{
				name: "Object is a SpecialResource with both Generation and ResourceVersion changed",
				mockSetup: func() {
					mockKernel.EXPECT().IsObjectAffine(gomock.Any()).Times(1).Return(false)
				},
				old: &v1beta1.SpecialResource{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{Kind: Kind},
						},
						Generation:      1,
						ResourceVersion: "dummy1",
					},
				},
				new: &v1beta1.SpecialResource{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{Kind: Kind},
						},
						Generation:      2,
						ResourceVersion: "dummy2",
					},
				},
				retMatcher: BeTrue(),
			},
		}

		entries := make([]TableEntry, 0, len(cases))
		for _, c := range cases {
			entries = append(entries, Entry(c.name, c.mockSetup, c.old, c.new, c.retMatcher))
		}

		DescribeTable(
			"should work as expected",
			func(mockSetup func(), old client.Object, new client.Object, m types.GomegaMatcher) {
				mockSetup()

				ret := f.GetPredicates().Update(event.UpdateEvent{
					ObjectOld: old,
					ObjectNew: new,
				})

				Expect(ret).To(m)
				Expect(f.GetMode()).To(Equal("UPDATE"))
			},
			entries...,
		)
	})

	Context("DeleteFunc", func() {
		cases := []struct {
			name       string
			obj        client.Object
			retMatcher types.GomegaMatcher
		}{
			{
				name:       "special resource",
				obj:        &v1beta1.SpecialResource{},
				retMatcher: BeTrue(),
			},
			// TODO(qbarrand) testing this function requires injecting a fake pkg/storage
			//{ name: "owned" },
			{
				name:       "random pod",
				obj:        &corev1.Pod{},
				retMatcher: BeFalse(),
			},
		}

		entries := make([]TableEntry, 0, len(cases))

		for _, c := range cases {
			entries = append(entries, Entry(c.name, c.obj, c.retMatcher))
		}

		DescribeTable(
			"should work as expected",
			func(obj client.Object, m types.GomegaMatcher) {
				ret := f.GetPredicates().Delete(event.DeleteEvent{Object: obj})

				Expect(ret).To(m)
				Expect(f.GetMode()).To(Equal("DELETE"))
			},
			entries...,
		)
	})

	Context("GenericFunc", func() {
		cases := []struct {
			name       string
			obj        client.Object
			retMatcher types.GomegaMatcher
		}{
			{
				name:       "special resource",
				obj:        &v1beta1.SpecialResource{},
				retMatcher: BeTrue(),
			},
			{
				name: "owned",
				obj: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{Kind: Kind},
						},
					},
				},
				retMatcher: BeTrue(),
			},
			{
				name:       "random pod",
				obj:        &corev1.Pod{},
				retMatcher: BeFalse(),
			},
		}

		entries := make([]TableEntry, 0, len(cases))

		for _, c := range cases {
			entries = append(entries, Entry(c.name, c.obj, c.retMatcher))
		}

		DescribeTable(
			"should return the correct value",
			func(obj client.Object, m types.GomegaMatcher) {
				ret := f.GetPredicates().Generic(event.GenericEvent{Object: obj})

				Expect(ret).To(m)
				Expect(f.GetMode()).To(Equal("GENERIC"))
			},
			entries...,
		)
	})
})
