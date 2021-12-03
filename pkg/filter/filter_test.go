package filter_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/openshift-psap/special-resource-operator/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/filter"
	buildv1 "github.com/openshift/api/build/v1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestFilter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Filter Suite")
}

var _ = Describe("SetLabel", func() {
	objs := []runtime.Object{
		&v1.DaemonSet{
			TypeMeta: metav1.TypeMeta{Kind: "DaemonSet"},
		},
		&v1.Deployment{
			TypeMeta: metav1.TypeMeta{Kind: "Deployment"},
		},
		&v1.StatefulSet{
			TypeMeta: metav1.TypeMeta{Kind: "StatefulSet"},
		},
	}

	entries := make([]TableEntry, 0, len(objs))

	for _, o := range objs {
		entries = append(entries, Entry(o.GetObjectKind().GroupVersionKind().Kind, o))
	}

	testFunc := func(o client.Object) {
		mo, err := runtime.DefaultUnstructuredConverter.ToUnstructured(o)
		Expect(err).NotTo(HaveOccurred())

		uo := unstructured.Unstructured{Object: mo}

		// Create the map manually, otherwise SetLabel returns an error
		err = unstructured.SetNestedStringMap(uo.Object, map[string]string{}, "spec", "template", "metadata", "labels")
		Expect(err).NotTo(HaveOccurred())

		err = filter.SetLabel(&uo)
		Expect(err).NotTo(HaveOccurred())

		Expect(uo.GetLabels()).To(HaveKeyWithValue("specialresource.openshift.io/owned", "true"))

		v, found, err := unstructured.NestedString(
			uo.Object,
			"spec",
			"template",
			"metadata",
			"labels",
			"specialresource.openshift.io/owned")

		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(v).To(Equal("true"))
	}

	DescribeTable("should the label", testFunc, entries...)

	It("should the label for BuildConfig", func() {
		bc := buildv1.BuildConfig{
			TypeMeta: metav1.TypeMeta{Kind: "BuildConfig"},
		}

		mo, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&bc)
		Expect(err).NotTo(HaveOccurred())

		uo := unstructured.Unstructured{Object: mo}

		err = filter.SetLabel(&uo)
		Expect(err).NotTo(HaveOccurred())
		Expect(uo.GetLabels()).To(HaveKeyWithValue("specialresource.openshift.io/owned", "true"))
	})
})

var _ = Describe("IsSpecialResource", func() {
	cases := []struct {
		name    string
		obj     client.Object
		matcher types.GomegaMatcher
	}{
		{
			name: "SpecialResource",
			obj: &v1beta1.SpecialResource{
				TypeMeta: metav1.TypeMeta{Kind: "SpecialResource"},
			},
			matcher: BeTrue(),
		},
		{
			name: "Pod owned by SRO",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"specialresource.openshift.io/owned": "true"},
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
			Expect(filter.IsSpecialResource(obj)).To(m)
		},
		entries...)
})

var _ = Describe("Owned", func() {
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
						{Kind: "SpecialResource"},
					},
				},
			},
			matcher: BeTrue(),
		},
		{
			name: "via labels",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"specialresource.openshift.io/owned": "whatever"},
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
			Expect(filter.Owned(obj)).To(m)
		},
		entries...,
	)
})

var _ = Describe("Predicate", func() {
	resetMode := func() { filter.Mode = "" }

	AfterEach(resetMode)

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
							{Kind: "SpecialResource"},
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
				ret := filter.Predicate().Create(event.CreateEvent{Object: obj})

				Expect(ret).To(m)
				Expect(filter.Mode).To(Equal("CREATE"))
			},
			entries...,
		)
	})

	Context("UpdateFunc", func() {
		It("should work as expected", func() {
			Skip("Testing this function requires injecting a fake ClientSet")
		})
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
			// TODO(qbarrand) testing this function requires injecting a fake ClientSet
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
				ret := filter.Predicate().Delete(event.DeleteEvent{Object: obj})

				Expect(ret).To(m)
				Expect(filter.Mode).To(Equal("DELETE"))
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
							{Kind: "SpecialResource"},
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
				ret := filter.Predicate().Generic(event.GenericEvent{Object: obj})

				Expect(ret).To(m)
				Expect(filter.Mode).To(Equal("GENERIC"))
			},
			entries...,
		)
	})
})
