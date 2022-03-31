package resource

import (
	"context"
	"errors"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubetypes "k8s.io/apimachinery/pkg/types"

	"github.com/openshift/special-resource-operator/internal/resourcehelper"
	"github.com/openshift/special-resource-operator/pkg/clients"
	"github.com/openshift/special-resource-operator/pkg/kernel"
	"github.com/openshift/special-resource-operator/pkg/lifecycle"
	"github.com/openshift/special-resource-operator/pkg/metrics"
	"github.com/openshift/special-resource-operator/pkg/poll"
	"github.com/openshift/special-resource-operator/pkg/proxy"
	"github.com/openshift/special-resource-operator/pkg/utils"
)

var (
	unstructuredMatcher = gomock.AssignableToTypeOf(&unstructured.Unstructured{})
)

const (
	ownedLabel = "specialresource.openshift.io/owned"
)

func TestResource(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Resource Suite")
}

var _ = Describe("resource_CreateFromYAML", func() {
	var (
		ctrl          *gomock.Controller
		kubeClient    *clients.MockClientsInterface
		mockLifecycle *lifecycle.MockLifecycle
		metricsClient *metrics.MockMetrics
		pollActions   *poll.MockPollActions
		kernelData    *kernel.MockKernelData
		proxyAPI      *proxy.MockProxyAPI
		helper        *resourcehelper.MockHelper
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		kubeClient = clients.NewMockClientsInterface(ctrl)
		mockLifecycle = lifecycle.NewMockLifecycle(ctrl)
		metricsClient = metrics.NewMockMetrics(ctrl)
		pollActions = poll.NewMockPollActions(ctrl)
		kernelData = kernel.NewMockKernelData(ctrl)
		proxyAPI = proxy.NewMockProxyAPI(ctrl)
		helper = resourcehelper.NewMockHelper(ctrl)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	yamlSpec := []byte(`---
apiVersion: v1
kind: Pod
metadata:
  name: nginx
spec:
  containers:
  - name: nginx
    image: nginx:1.14.2
    ports:
    - containerPort: 80
  restartPolicy: Always
`)

	It("should not return an error when the resource is already there", func() {
		const (
			kernelFullVersion         = "1.2.3"
			namespace                 = "ns"
			operatingSystemMajorMinor = "8.5"
			specialResourceName       = "special-resource"
		)

		nodeSelector := map[string]string{"key": "value"}

		owner := v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "owner",
				Namespace: namespace,
			},
		}

		nsn := kubetypes.NamespacedName{
			Namespace: namespace,
			Name:      "nginx",
		}

		gomock.InOrder(
			helper.EXPECT().IsNamespaced("Pod").Times(1).Return(true),
			helper.EXPECT().SetLabel(gomock.Any(), ownedLabel).Times(1).
				DoAndReturn(func(obj *unstructured.Unstructured, label string) error {
					return resourcehelper.New().SetLabel(obj, label)
				}),
			kernelData.EXPECT().IsObjectAffine(gomock.Any()).Times(1).Return(false),
			helper.EXPECT().SetNodeSelectorTerms(gomock.Any(), nodeSelector).Times(1).
				DoAndReturn(func(obj *unstructured.Unstructured, terms map[string]string) error {
					return resourcehelper.New().SetNodeSelectorTerms(obj, terms)
				}),
			helper.EXPECT().IsNamespaced("Pod").Times(1).Return(true),
			helper.EXPECT().SetMetaData(gomock.Any(), specialResourceName, namespace).Times(1).
				Do(func(obj *unstructured.Unstructured, nm string, ns string) {
					resourcehelper.New().SetMetaData(obj, nm, ns)
				}),
			kubeClient.EXPECT().Get(context.TODO(), nsn, unstructuredMatcher).Times(1),
			helper.EXPECT().IsNotUpdateable("Pod").Times(1).Return(true),
			metricsClient.EXPECT().SetCompletedKind(specialResourceName, "Pod", "nginx", namespace, 1).Times(1),
		)

		scheme := runtime.NewScheme()

		err := v1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		err =
			NewResourceAPI(kubeClient, metricsClient, pollActions, kernelData, scheme, mockLifecycle, proxyAPI, helper).
				CreateFromYAML(
					context.TODO(),
					yamlSpec,
					false,
					&owner,
					specialResourceName,
					namespace,
					nodeSelector,
					kernelFullVersion,
					operatingSystemMajorMinor,
				)

		Expect(err).NotTo(HaveOccurred())
	})

	It("should create the resource when it is not already there", func() {
		const (
			kernelFullVersion         = "1.2.3"
			name                      = "nginx"
			namespace                 = "ns"
			operatingSystemMajorMinor = "8.5"
			ownerName                 = "owner"
			specialResourceName       = "special-resource"
		)

		nodeSelector := map[string]string{"key": "value"}

		owner := v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ownerName,
				Namespace: namespace,
			},
		}

		trueVar := true

		newPod := unstructured.Unstructured{}
		newPod.SetAPIVersion("v1")
		newPod.SetKind("Pod")
		newPod.SetName(name)
		newPod.SetNamespace(namespace)
		newPod.SetAnnotations(map[string]string{
			"meta.helm.sh/release-name":         specialResourceName,
			"meta.helm.sh/release-namespace":    namespace,
			"specialresource.openshift.io/hash": "5473155173593167161",
		})
		newPod.SetLabels(map[string]string{
			"app.kubernetes.io/managed-by": "Helm",
			ownedLabel:                     "true",
		})
		newPod.SetOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion:         "v1",
				Kind:               "Pod",
				Name:               ownerName,
				BlockOwnerDeletion: &trueVar,
				Controller:         &trueVar,
			},
		})

		container := map[string]interface{}{
			"name":  "nginx",
			"image": "nginx:1.14.2",
			"ports": []interface{}{
				map[string]interface{}{"containerPort": int64(80)},
				// YAML deserializer converts all integers to int64, so use an int64 here as well
			},
		}

		// Setting this manually because unstructured.SetNestedMap struggles to deep copy the container ports
		newPod.Object["spec"] = map[string]interface{}{
			"containers": []interface{}{container},
		}

		err := unstructured.SetNestedStringMap(newPod.Object, nodeSelector, "spec", "nodeSelector")
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(newPod.Object, "Always", "spec", "restartPolicy")
		Expect(err).NotTo(HaveOccurred())

		nsn := kubetypes.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}

		gomock.InOrder(
			helper.EXPECT().IsNamespaced("Pod").Times(1).Return(true),
			helper.EXPECT().SetLabel(gomock.Any(), ownedLabel).Times(1).
				DoAndReturn(func(obj *unstructured.Unstructured, label string) error {
					return resourcehelper.New().SetLabel(obj, label)
				}),
			kernelData.EXPECT().IsObjectAffine(gomock.Any()).Times(1).Return(false),
			helper.EXPECT().SetNodeSelectorTerms(gomock.Any(), nodeSelector).Times(1).
				DoAndReturn(func(obj *unstructured.Unstructured, terms map[string]string) error {
					return resourcehelper.New().SetNodeSelectorTerms(obj, terms)
				}),
			helper.EXPECT().IsNamespaced("Pod").Times(1).Return(true),
			helper.EXPECT().SetMetaData(gomock.Any(), specialResourceName, namespace).Times(1).
				Do(func(obj *unstructured.Unstructured, nm string, ns string) {
					resourcehelper.New().SetMetaData(obj, nm, ns)
				}),
			kubeClient.
				EXPECT().
				Get(context.TODO(), nsn, unstructuredMatcher).
				Return(k8serrors.NewNotFound(v1.Resource("pod"), name)),
			helper.EXPECT().IsOneTimer(gomock.Any()).Times(1),
			helper.EXPECT().SetMetaData(gomock.Any(), specialResourceName, namespace).Times(1).
				Do(func(obj *unstructured.Unstructured, nm string, ns string) {
					resourcehelper.New().SetMetaData(obj, nm, ns)
				}),
			kubeClient.
				EXPECT().
				Create(context.TODO(), &newPod),
			metricsClient.
				EXPECT().
				SetCompletedKind(specialResourceName, "Pod", name, namespace, 1),
		)

		scheme := runtime.NewScheme()

		err = v1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		err =
			NewResourceAPI(kubeClient, metricsClient, pollActions, kernelData, scheme, mockLifecycle, proxyAPI, helper).
				CreateFromYAML(
					context.TODO(),
					yamlSpec,
					false,
					&owner,
					specialResourceName,
					namespace,
					nodeSelector,
					kernelFullVersion,
					operatingSystemMajorMinor,
				)

		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("resource_GetObjectsFromYAML", func() {
	var (
		ctrl          *gomock.Controller
		kubeClient    *clients.MockClientsInterface
		mockLifecycle *lifecycle.MockLifecycle
		metricsClient *metrics.MockMetrics
		pollActions   *poll.MockPollActions
		kernelData    *kernel.MockKernelData
		proxyAPI      *proxy.MockProxyAPI
		helper        *resourcehelper.MockHelper
		scheme        *runtime.Scheme
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		kubeClient = clients.NewMockClientsInterface(ctrl)
		mockLifecycle = lifecycle.NewMockLifecycle(ctrl)
		metricsClient = metrics.NewMockMetrics(ctrl)
		pollActions = poll.NewMockPollActions(ctrl)
		kernelData = kernel.NewMockKernelData(ctrl)
		proxyAPI = proxy.NewMockProxyAPI(ctrl)
		helper = resourcehelper.NewMockHelper(ctrl)
		scheme = runtime.NewScheme()
		err := v1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	yamlSpec := []byte(`---
apiVersion: v1
kind: Pod
metadata:
  name: podName
spec:
  containers:
  - name: podName
    image: podimage:1.14.2
    ports:
    - containerPort: 80
  restartPolicy: Always
---
apiVersion: v1
kind: DaemonSet
metadata:
  name: dsName
spec:
  containers:
  - name: dsName
    image: dsimage:1.14.2
    ports:
    - containerPort: 80
  restartPolicy: Always
`)
	It("good flow", func() {
		r := NewResourceAPI(kubeClient, metricsClient, pollActions, kernelData, scheme, mockLifecycle, proxyAPI, helper)
		resultList, err := r.GetObjectsFromYAML(yamlSpec)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(resultList.Items)).To(Equal(2))
		Expect(resultList.Items[0].GetName()).To(Equal("podName"))
		Expect(resultList.Items[1].GetName()).To(Equal("dsName"))
		Expect(resultList.Items[0].GetKind()).To(Equal("Pod"))
		Expect(resultList.Items[1].GetKind()).To(Equal("DaemonSet"))
	})
})

var _ = Describe("resource_CheckForImagePullBackOff", func() {
	var (
		ctrl        *gomock.Controller
		kubeClient  *clients.MockClientsInterface
		pollActions *poll.MockPollActions
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		kubeClient = clients.NewMockClientsInterface(ctrl)
		pollActions = poll.NewMockPollActions(ctrl)
	})

	AfterEach(func() {
		UpdateVendor = ""
	})

	const (
		app       = "test"
		namespace = "ns"
	)

	opts := []interface{}{
		client.InNamespace(namespace),
		client.MatchingLabels(map[string]string{"app": app}),
	}

	It("should return no error if polling returned nil", func() {
		ds := &unstructured.Unstructured{}

		pollActions.EXPECT().ForDaemonSet(context.TODO(), ds)

		err := NewResourceAPI(kubeClient, nil, pollActions, nil, nil, nil, nil, nil).(*resource).
			checkForImagePullBackOff(context.TODO(), ds, namespace)

		Expect(err).NotTo(HaveOccurred())
	})

	getDaemonSet := func() *unstructured.Unstructured {
		ds := &unstructured.Unstructured{}
		ds.SetAPIVersion("v1")
		ds.SetKind("DaemonSet")
		ds.SetLabels(map[string]string{"app": app})

		return ds
	}

	It("should return an error if we cannot get the pod list", func() {
		randomError := errors.New("random error")
		ds := getDaemonSet()

		gomock.InOrder(
			pollActions.EXPECT().ForDaemonSet(context.TODO(), ds).Return(errors.New("some error")),
			kubeClient.EXPECT().List(context.TODO(), &v1.PodList{}, opts...).Return(randomError),
		)

		err := NewResourceAPI(kubeClient, nil, pollActions, nil, nil, nil, nil, nil).(*resource).
			checkForImagePullBackOff(context.TODO(), ds, namespace)

		Expect(err).To(Equal(randomError))
	})

	It("should return an error if no pods were found", func() {
		ds := getDaemonSet()

		gomock.InOrder(
			pollActions.EXPECT().ForDaemonSet(context.TODO(), ds).Return(errors.New("some error")),
			kubeClient.EXPECT().List(context.TODO(), &v1.PodList{}, opts...),
		)

		err := NewResourceAPI(kubeClient, nil, pollActions, nil, nil, nil, nil, nil).(*resource).
			checkForImagePullBackOff(context.TODO(), ds, namespace)

		Expect(err).To(HaveOccurred())
	})

	It("should return an error if one of the pods is Waiting because ImagePullBackOff", func() {
		const vendor = "test-vendor"

		ds := getDaemonSet()
		ds.SetAnnotations(map[string]string{"specialresource.openshift.io/driver-container-vendor": vendor})

		gomock.InOrder(
			pollActions.EXPECT().ForDaemonSet(context.TODO(), ds).Return(errors.New("some error")),
			kubeClient.
				EXPECT().
				List(context.TODO(), &v1.PodList{}, opts...).
				Do(func(_ context.Context, pl *v1.PodList, _ client.InNamespace, _ client.MatchingLabels) {
					pl.Items = []v1.Pod{
						{
							Status: v1.PodStatus{Phase: "test"},
						},
						{
							Status: v1.PodStatus{
								ContainerStatuses: []v1.ContainerStatus{
									{
										State: v1.ContainerState{
											Waiting: &v1.ContainerStateWaiting{
												Reason: "ImagePullBackOff",
											},
										},
									},
								},
							},
						},
					}
				}),
		)

		err := NewResourceAPI(kubeClient, nil, pollActions, nil, nil, nil, nil, nil).(*resource).
			checkForImagePullBackOff(context.TODO(), ds, namespace)

		Expect(err).To(MatchError("ImagePullBackOff need to rebuild " + vendor + " driver-container"))
		Expect(UpdateVendor).To(Equal(vendor))
	})

	It("should return an error if one of the pods is Waiting for a random reason", func() {
		ds := getDaemonSet()

		gomock.InOrder(
			pollActions.EXPECT().ForDaemonSet(context.TODO(), ds).Return(errors.New("some error")),
			kubeClient.
				EXPECT().
				List(context.TODO(), &v1.PodList{}, opts...).
				Do(func(_ context.Context, pl *v1.PodList, _ client.InNamespace, _ client.MatchingLabels) {
					pl.Items = []v1.Pod{
						{
							Status: v1.PodStatus{Phase: "test"},
						},
						{
							Status: v1.PodStatus{
								ContainerStatuses: []v1.ContainerStatus{
									{
										State: v1.ContainerState{
											Waiting: &v1.ContainerStateWaiting{
												Reason: "Random",
											},
										},
									},
								},
							},
						},
					}
				}),
		)

		err := NewResourceAPI(kubeClient, nil, pollActions, nil, nil, nil, nil, nil).(*resource).
			checkForImagePullBackOff(context.TODO(), ds, namespace)

		Expect(err).NotTo(HaveOccurred())
		Expect(UpdateVendor).To(BeEmpty())
	})

	It("should not panic if a container is not waiting", func() {
		ds := getDaemonSet()

		gomock.InOrder(
			pollActions.EXPECT().ForDaemonSet(context.TODO(), ds).Return(errors.New("some error")),
			kubeClient.
				EXPECT().
				List(context.TODO(), &v1.PodList{}, opts...).
				Do(func(_ context.Context, pl *v1.PodList, _ client.InNamespace, _ client.MatchingLabels) {
					pl.Items = []v1.Pod{
						{
							Status: v1.PodStatus{
								ContainerStatuses: make([]v1.ContainerStatus, 1),
							},
						},
					}
				}),
		)

		err := NewResourceAPI(kubeClient, nil, pollActions, nil, nil, nil, nil, nil).(*resource).
			checkForImagePullBackOff(context.TODO(), ds, namespace)

		Expect(err).NotTo(HaveOccurred())
		Expect(UpdateVendor).To(BeEmpty())
	})
})

var _ = Describe("resource_BeforeCRUD", func() {
	var (
		ctrl     *gomock.Controller
		proxyAPI *proxy.MockProxyAPI
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		proxyAPI = proxy.NewMockProxyAPI(ctrl)
	})

	It("should setup a proxy if an proxy annotation is present", func() {
		obj := &unstructured.Unstructured{}
		obj.SetAnnotations(map[string]string{
			"specialresource.openshift.io/proxy": "true",
		})

		proxyAPI.EXPECT().Setup(obj).Return(nil).Times(1)

		err := NewResourceAPI(nil, nil, nil, nil, nil, nil, proxyAPI, nil).(*resource).
			BeforeCRUD(obj, nil)

		Expect(err).ToNot(HaveOccurred())
	})

})

var _ = Describe("resource_AfterCRUD", func() {
	var (
		ctrl        *gomock.Controller
		pollActions *poll.MockPollActions
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		pollActions = poll.NewMockPollActions(ctrl)
	})

	DescribeTable("annotations trigger specific work",
		func(annotation, value string, expectations func()) {
			obj := &unstructured.Unstructured{}
			obj.SetAnnotations(map[string]string{
				annotation: value,
			})

			expectations()

			err := NewResourceAPI(nil, nil, pollActions, nil, nil, nil, nil, nil).(*resource).
				AfterCRUD(context.Background(), obj, "ns")

			Expect(err).ToNot(HaveOccurred())

		},
		Entry("specialresource.openshift.io/state",
			"specialresource.openshift.io/state", "driver-container",
			func() {
				pollActions.EXPECT().ForDaemonSet(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
		),
		Entry("specialresource.openshift.io/wait",
			"specialresource.openshift.io/wait", "true",
			func() {
				pollActions.EXPECT().ForResource(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
		),
		Entry("specialresource.openshift.io/wait-for-logs",
			"specialresource.openshift.io/wait-for-logs", "pattern",
			func() {
				pollActions.EXPECT().ForDaemonSetLogs(gomock.Any(), gomock.Any(), "pattern").Return(nil).Times(1)
			},
		),

		Entry("helm.sh/hook",
			"helm.sh/hook", "true",
			func() {
				pollActions.EXPECT().ForResource(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
		),
	)

	It("will wait for a resource if the kind is CRD", func() {
		obj := &unstructured.Unstructured{}
		obj.SetKind("CustomResourceDefinition")

		pollActions.EXPECT().ForResource(gomock.Any(), gomock.Any()).Return(nil).Times(1)

		err := NewResourceAPI(nil, nil, pollActions, nil, nil, nil, nil, nil).(*resource).
			AfterCRUD(context.Background(), obj, "ns")

		Expect(err).ToNot(HaveOccurred())
	})
})

var _ = Describe("resource_CRUD", func() {
	var (
		ctrl       *gomock.Controller
		kubeClient *clients.MockClientsInterface
		helper     *resourcehelper.MockHelper

		r *resource
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		kubeClient = clients.NewMockClientsInterface(ctrl)
		helper = resourcehelper.NewMockHelper(ctrl)

		scheme := runtime.NewScheme()
		Expect(v1.AddToScheme(scheme)).To(Succeed())

		r = NewResourceAPI(kubeClient, nil, nil, nil, scheme, nil, nil, helper).(*resource)
	})

	specialResourceName := "special-resource"
	namespace := "ns"
	owner := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owner",
			Namespace: namespace,
		},
	}

	prepareUnstructured := func(kind, name, namespace string) *unstructured.Unstructured {
		u := &unstructured.Unstructured{}
		u.SetKind(kind)
		u.SetName(name)
		u.SetNamespace(namespace)
		return u
	}

	DescribeTable("resource should have metadata (and owner) set up, depending on its type",
		func(kind, name, namespace string, isNamespaced, shouldSetMetaData bool) {
			u := prepareUnstructured(kind, name, namespace)

			helper.EXPECT().IsNamespaced(u.GetKind()).Return(isNamespaced)
			kubeClient.EXPECT().
				Get(gomock.Any(), types.NamespacedName{Namespace: u.GetNamespace(), Name: u.GetName()}, gomock.Any()).
				Return(nil)
			helper.EXPECT().IsNotUpdateable(u.GetKind()).Return(true)

			// assert
			times := 0
			if shouldSetMetaData {
				times = 1
			}
			helper.EXPECT().SetMetaData(u, specialResourceName, namespace).Times(times)

			Expect(r.CRUD(context.Background(), u, false, &owner, specialResourceName, namespace)).To(Succeed())
		},
		Entry("neither SpecialResource nor Namespace", "Pod", "name", namespace, true, true),
		Entry("Namespace", "Namespace", namespace, "", false, false),
		Entry("SpecialResource", "SpecialResource", "sr-name", "", false, false),
	)

	DescribeTable("when object does not exist",
		func(isOneTimer, releaseInstalled bool) {
			name := "nginx"
			obj := prepareUnstructured("Pod", name, namespace)

			helper.EXPECT().IsNamespaced(obj.GetKind()).Return(true)
			helper.EXPECT().SetMetaData(obj, specialResourceName, namespace).AnyTimes()
			kubeClient.EXPECT().
				Get(gomock.Any(), types.NamespacedName{Namespace: namespace, Name: name}, gomock.Any()).
				Return(&k8serrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound}})
			helper.EXPECT().IsOneTimer(obj).Return(isOneTimer, nil)

			// assert
			times := 1
			if isOneTimer && releaseInstalled {
				times = 0
			}
			kubeClient.EXPECT().Create(gomock.Any(), gomock.Any()).Times(times)

			Expect(r.CRUD(context.Background(), obj, releaseInstalled, &owner, specialResourceName, namespace)).To(Succeed())
		},
		Entry("object is OneTimer & release is installed = no object recreation", true, true),
		Entry("object is OneTimer & release is not installed = object recreation", true, false),
		Entry("object is not OneTimer & release is installed = object recreation", false, true),
		Entry("object is not OneTimer & release is not installed = object recreation", false, false))

	DescribeTable("GET fails",
		func(errReason metav1.StatusReason, expectedSubstring string) {
			name := "nginx"
			obj := prepareUnstructured("Pod", name, namespace)

			helper.EXPECT().IsNamespaced(obj.GetKind()).Return(true)
			helper.EXPECT().SetMetaData(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
			kubeClient.EXPECT().
				Get(gomock.Any(), types.NamespacedName{Namespace: namespace, Name: name}, gomock.Any()).
				Return(&k8serrors.StatusError{ErrStatus: metav1.Status{Reason: errReason}})

			releaseInstalled := false
			err := r.CRUD(context.Background(), obj, releaseInstalled, &owner, specialResourceName, namespace)
			Expect(err.Error()).To(ContainSubstring(expectedSubstring))
		},
		Entry("forbidden error", metav1.StatusReasonForbidden, "forbidden"),
		Entry("other errors", metav1.StatusReasonUnauthorized, "unexpected error"),
	)

	DescribeTable("updating the object",
		func(mockSetups func(*unstructured.Unstructured), assert func()) {
			name := "nginx"
			obj := prepareUnstructured("Pod", name, namespace)

			helper.EXPECT().IsNamespaced(obj.GetKind()).Return(true)
			helper.EXPECT().SetMetaData(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

			mockSetups(obj)

			assert()

			releaseInstalled := false
			Expect(r.CRUD(context.Background(), obj, releaseInstalled, &owner, specialResourceName, namespace)).To(Succeed())

		},
		Entry("won't happen if object is not updateable",
			func(obj *unstructured.Unstructured) {
				kubeClient.EXPECT().
					Get(gomock.Any(), types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, gomock.Any()).
					Return(nil)
				helper.EXPECT().
					IsNotUpdateable(obj.GetKind()).
					Return(true)
			},
			func() {
				kubeClient.EXPECT().Update(gomock.Any(), gomock.Any()).Times(0)
			},
		),
		Entry("won't happen if object's hash did not change",
			func(obj *unstructured.Unstructured) {
				kubeClient.EXPECT().
					Get(gomock.Any(), types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ client.ObjectKey, o client.Object) error {
						u := o.(*unstructured.Unstructured)
						obj.DeepCopyInto(u)
						Expect(utils.Annotate(u)).To(Succeed())
						return nil
					})

				helper.EXPECT().IsNotUpdateable(obj.GetKind()).Return(false)
			},
			func() {
				kubeClient.EXPECT().Update(gomock.Any(), gomock.Any()).Times(0)
			},
		),
		Entry("will happen otherwise",
			func(obj *unstructured.Unstructured) {
				kubeClient.EXPECT().
					Get(gomock.Any(), types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ client.ObjectKey, o client.Object) error {
						u := o.(*unstructured.Unstructured)
						obj.DeepCopyInto(u)
						return nil
					})

				helper.EXPECT().IsNotUpdateable(obj.GetKind()).Return(false)
				helper.EXPECT().UpdateResourceVersion(gomock.Any(), gomock.Any()).Return(nil)
			},
			func() {
				kubeClient.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, o client.Object) error {
					Expect(o.GetAnnotations()).To(HaveKey("specialresource.openshift.io/hash"))
					return nil
				}).Times(1)
			},
		),
	)
})
