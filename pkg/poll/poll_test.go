package poll

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gtypes "github.com/onsi/gomega/types"

	"github.com/openshift/special-resource-operator/pkg/clients"
	"github.com/openshift/special-resource-operator/pkg/lifecycle"
	"github.com/openshift/special-resource-operator/pkg/storage"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	restclient "k8s.io/client-go/rest"
	fakerestclient "k8s.io/client-go/rest/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var Any = gomock.Any

const (
	namespace     = "some-namespace"
	daemonSetName = "some-driver-container"
)

var (
	ctrl                 *gomock.Controller
	mockClientsInterface *clients.MockClientsInterface
	mockLifecycle        *lifecycle.MockLifecycle
	mockStorage          *storage.MockStorage
	pa                   PollActions
)

func TestPoll(t *testing.T) {
	RegisterFailHandler(Fail)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockClientsInterface = clients.NewMockClientsInterface(ctrl)
		mockLifecycle = lifecycle.NewMockLifecycle(ctrl)
		mockStorage = storage.NewMockStorage(ctrl)
		pa = New(mockClientsInterface, mockLifecycle, mockStorage)
	})

	RunSpecs(t, "PollActions Suite")
}

func prepareUnstructured(kind, name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetKind(kind)
	obj.SetName(name)
	obj.SetNamespace(namespace)
	return obj
}

var _ = Context("Waiting for resource", func() {
	// Following test focuses on forResourceAvailability so other tests can focus on more specific use cases
	DescribeTable("Namespace/Certificates/Secrets",
		func(obj *unstructured.Unstructured, e error, matcher gtypes.GomegaMatcher) {
			mockClientsInterface.EXPECT().
				Get(Any(), types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, Any()).
				Return(e).AnyTimes()

			Expect(pa.ForResource(context.Background(), obj)).To(matcher)
		},
		Entry("namespace is found",
			prepareUnstructured("Namespace", namespace, ""),
			nil,
			Succeed()),
		Entry("wait for namespace times out",
			prepareUnstructured("Namespace", namespace, ""),
			&apierrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound}},
			Not(Succeed())),
		Entry("another error occurrs when getting namespace",
			prepareUnstructured("Namespace", namespace, ""),
			&apierrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonUnauthorized}},
			Not(Succeed())),
		Entry("Certificates are found",
			prepareUnstructured("Certificates", namespace, "certs-name"),
			nil,
			Succeed()),
		Entry("Secret is found",
			prepareUnstructured("Secret", namespace, "secret-name"),
			nil,
			Succeed()),
	)

	// Following test focuses on forResourceFullAvailability so other tests can focus on more specific use cases
	DescribeTable("should work for Pod",
		func(mockSetup func(), matcher gtypes.GomegaMatcher) {
			// forResourceAvailability
			mockClientsInterface.EXPECT().Get(Any(), Any(), Any()).Return(nil)

			// forResourceFullAvailability
			mockSetup()

			Expect(pa.ForResource(context.Background(), prepareUnstructured("Pod", "pod-name", namespace))).To(matcher)
		},

		Entry(
			"happy flow",
			func() {
				mockClientsInterface.EXPECT().Get(Any(), Any(), Any()).
					DoAndReturn(func(_ context.Context, _ client.ObjectKey, o client.Object) error {
						u := o.(*unstructured.Unstructured)
						Expect(unstructured.SetNestedField(u.Object, "Succeeded", "status", "phase")).To(Succeed())
						return nil
					})
			},
			Succeed(),
		),

		Entry(
			"pod is still running and times out",
			func() {
				mockClientsInterface.EXPECT().Get(Any(), Any(), Any()).
					DoAndReturn(func(_ context.Context, _ client.ObjectKey, o client.Object) error {
						u := o.(*unstructured.Unstructured)
						Expect(unstructured.SetNestedField(u.Object, "Running", "status", "phase")).To(Succeed())
						return nil
					}).AnyTimes()
			},
			Not(Succeed()),
		),

		Entry(
			"k8s fails to fulfil a Get request",
			func() {
				mockClientsInterface.EXPECT().
					Get(Any(), Any(), Any()).
					Return(&apierrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonUnauthorized}}).
					AnyTimes()
			},
			Not(Succeed()),
		),

		Entry(
			"callback fails to obtain field",
			func() {
				mockClientsInterface.EXPECT().
					Get(Any(), Any(), Any()).
					Return(nil).
					AnyTimes()
			},
			Not(Succeed()),
		),
	)

	Specify("should work for CRDs", func() {
		// forCRD
		mockClientsInterface.EXPECT().Invalidate()

		// forResourceAvailability
		mockClientsInterface.EXPECT().Get(Any(), Any(), Any()).Return(nil)

		// forCRD
		mockClientsInterface.EXPECT().ServerGroups().Return(nil, nil)

		Expect(pa.ForResource(context.Background(), prepareUnstructured("CustomResourceDefinition", "crd-name", ""))).To(Succeed())
	})

	DescribeTable("should work for StatefulSets",
		func(desiredReplicas, currentReplicas int64, matcher gtypes.GomegaMatcher) {
			// forResourceAvailability
			mockClientsInterface.EXPECT().Get(Any(), Any(), Any()).Return(nil)

			// forResourceFullAvailability
			mockClientsInterface.EXPECT().Get(Any(), Any(), Any()).
				DoAndReturn(func(_ context.Context, _ client.ObjectKey, o client.Object) error {
					u := o.(*unstructured.Unstructured)
					Expect(unstructured.SetNestedField(u.Object, desiredReplicas, "spec", "replicas")).To(Succeed())
					Expect(unstructured.SetNestedField(u.Object, currentReplicas, "status", "currentReplicas")).To(Succeed())
					return nil
				}).AnyTimes()

			Expect(pa.ForResource(context.Background(), prepareUnstructured("StatefulSet", "ss-name", namespace))).To(matcher)
		},
		Entry("when there's not enough replicas", int64(3), int64(2), Not(Succeed())),
		Entry("when there's  enough replicas", int64(3), int64(3), Succeed()),
	)

	DescribeTable("should work for Jobs",
		func(status string, matcher gtypes.GomegaMatcher) {
			// forResourceAvailability
			mockClientsInterface.EXPECT().Get(Any(), Any(), Any()).Return(nil)

			// forResourceFullAvailability
			mockClientsInterface.EXPECT().Get(Any(), Any(), Any()).
				DoAndReturn(func(_ context.Context, _ client.ObjectKey, o client.Object) error {
					u := o.(*unstructured.Unstructured)
					err := unstructured.SetNestedSlice(u.Object,
						[]interface{}{
							map[string]interface{}{
								"status": "True",
								"type":   status,
							}},
						"status", "conditions")
					Expect(err).ToNot(HaveOccurred())
					return nil
				}).AnyTimes()

			Expect(pa.ForResource(context.Background(), prepareUnstructured("Job", "job-name", namespace))).To(matcher)
		},
		Entry("which have finished", "Complete", Succeed()),
		Entry("which are still running", "Running", Not(Succeed())),
	)

	DescribeTable("should work for Deployments",
		func(desiredReplicas, currentReplicas int64, matcher gtypes.GomegaMatcher) {
			// forResourceAvailability
			mockClientsInterface.EXPECT().Get(Any(), Any(), Any()).Return(nil)

			// forResourceFullAvailability
			mockClientsInterface.EXPECT().Get(Any(), Any(), Any()).
				DoAndReturn(func(_ context.Context, _ client.ObjectKey, o client.Object) error {
					u := o.(*unstructured.Unstructured)
					err := unstructured.SetNestedMap(u.Object, map[string]interface{}{
						"app": "some-app",
					}, "spec", "selector", "matchLabels")
					Expect(err).ToNot(HaveOccurred())
					return nil
				}).AnyTimes()

			// callback
			mockClientsInterface.EXPECT().
				List(Any(), Any(), Any()).
				DoAndReturn(func(_ context.Context, obj client.ObjectList, _ ...client.ListOption) error {
					u := obj.(*unstructured.UnstructuredList)
					rs := *prepareUnstructured("ReplicaSet", "replicaset-name", namespace)

					err := unstructured.SetNestedMap(rs.Object, map[string]interface{}{
						"replicas":          desiredReplicas,
						"availableReplicas": currentReplicas,
					}, "status")
					Expect(err).ToNot(HaveOccurred())
					u.Items = append(u.Items, rs)
					return nil
				}).AnyTimes()

			Expect(pa.ForResource(context.Background(), prepareUnstructured("Deployment", "deploy-name", namespace))).To(matcher)
		},
		Entry("when there's not enough replicas", int64(3), int64(2), Not(Succeed())),
		Entry("when there's enough replicas", int64(3), int64(3), Succeed()),
	)
})

var _ = Context("Waiting for Build", func() {
	It("should fail when resource is not created yet", func() {
		// forResourceAvailability
		mockClientsInterface.EXPECT().Get(Any(), Any(), Any()).Return(nil)
		// forBuild
		mockClientsInterface.EXPECT().List(Any(), Any(), Any()).Return(nil)
		Expect(pa.ForResource(context.Background(), prepareUnstructured("BuildConfig", "build-name", namespace))).To(Not(Succeed()))
	})
	It("resource is created and belongs to my BuildConfig and is finished", func() {
		// forResourceAvailability
		mockClientsInterface.EXPECT().Get(Any(), Any(), Any()).Return(nil)
		// forBuild
		mockClientsInterface.EXPECT().
			List(Any(), Any(), Any()).
			DoAndReturn(func(_ context.Context, obj client.ObjectList, _ ...client.ListOption) error {
				build := prepareUnstructured("Build", "build-name-1", namespace)
				Expect(unstructured.SetNestedSlice(build.Object, []interface{}{map[string]interface{}{
					"name": "build-name",
				}}, "metadata", "ownerReferences")).To(Succeed())
				u := obj.(*unstructured.UnstructuredList)
				u.Items = append(u.Items, *build)
				return nil
			})
		// forResourceFullAvailability
		mockClientsInterface.EXPECT().Get(Any(), Any(), Any()).
			DoAndReturn(func(_ context.Context, _ client.ObjectKey, o client.Object) error {
				u := o.(*unstructured.Unstructured)
				Expect(unstructured.SetNestedField(u.Object, "Complete", "status", "phase")).To(Succeed())
				return nil
			})

		Expect(pa.ForResource(context.Background(), prepareUnstructured("BuildConfig", "build-name", namespace))).To(Succeed())
	})
	It("resource is created and does not belong to my BuildConfig", func() {
		// forResourceAvailability
		mockClientsInterface.EXPECT().Get(Any(), Any(), Any()).Return(nil)

		// forBuild
		mockClientsInterface.EXPECT().
			List(Any(), Any(), Any()).
			DoAndReturn(func(_ context.Context, obj client.ObjectList, _ ...client.ListOption) error {
				build := prepareUnstructured("Build", "build-name-1", namespace)
				Expect(unstructured.SetNestedSlice(build.Object, []interface{}{map[string]interface{}{
					"name": "other-build-name",
				}}, "metadata", "ownerReferences")).To(Succeed())
				u := obj.(*unstructured.UnstructuredList)
				u.Items = append(u.Items, *build)
				return nil
			})

		Expect(pa.ForResource(context.Background(), prepareUnstructured("BuildConfig", "build-name", namespace))).To(Not(Succeed()))
	})
})

var _ = Context("Waiting for DaemonSet", func() {
	namespacedName := types.NamespacedName{Namespace: namespace, Name: daemonSetName}
	var obj *unstructured.Unstructured

	BeforeEach(func() {
		obj = prepareUnstructured("DaemonSet", daemonSetName, namespace)
		Expect(unstructured.SetNestedField(obj.Object, "OnDelete", "spec", "updateStrategy", "type")).To(Succeed())
	})

	Context("but the pod is marked", func() {
		It("timeout waiting for no such pods", func() {
			podList := &v1.PodList{
				Items: []v1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "some-driver-container-1",
							Namespace: "some-namespace",
						},
					},
				},
			}

			// forResourceAvailability
			mockClientsInterface.EXPECT().
				Get(gomock.Any(), namespacedName, gomock.Any()).
				Return(nil).
				AnyTimes()

			// forLifecycleAvailability
			mockLifecycle.EXPECT().
				GetPodFromDaemonSet(gomock.Any(), namespacedName).
				Return(podList).
				AnyTimes()

			// Pod has an entry in 'lifecycle' ConfigMap. Entry adding occurs when OS upgrade is performed
			// (refer to pkg/filter)
			mockStorage.EXPECT().
				CheckConfigMapEntry(gomock.Any(), gomock.Any(), gomock.Any()).
				Return("*v1.Pod", nil).
				AnyTimes()

			err := pa.ForDaemonSet(context.Background(), obj)
			Expect(err).ToNot(BeNil())
		})
	})

	Context("which is created, but the pods are not ready", func() {
		It("will propagate an error", func() {
			podList := &v1.PodList{
				Items: []v1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "some-driver-container-1",
							Namespace: "some-namespace",
						},
					},
				},
			}

			gomock.InOrder(
				// forResourceAvailability
				mockClientsInterface.EXPECT().
					Get(gomock.Any(), namespacedName, gomock.Any()).
					Return(nil),

				// forLifecycleAvailability
				mockLifecycle.EXPECT().
					GetPodFromDaemonSet(gomock.Any(), namespacedName).
					Return(podList).
					AnyTimes(),

				mockStorage.EXPECT().
					CheckConfigMapEntry(gomock.Any(), gomock.Any(), gomock.Any()).
					Return("", nil).
					AnyTimes(),

				// forResourceFullAvailability
				mockClientsInterface.EXPECT().
					Get(gomock.Any(), namespacedName, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ client.ObjectKey, o client.Object) error {
						u := o.(*unstructured.Unstructured)
						Expect(unstructured.SetNestedField(u.Object, int64(1), "status", "desiredNumberScheduled")).To(Succeed())
						Expect(unstructured.SetNestedField(u.Object, int64(0), "status", "numberUnavailable")).To(Succeed())
						Expect(unstructured.SetNestedField(u.Object, int64(1), "status", "numberAvailable")).To(Succeed())
						return nil
					}).
					AnyTimes(),
			)

			err := pa.ForDaemonSet(context.Background(), obj)
			Expect(err).To(BeNil())
		})
	})
})

var _ = Context("Polling for DaemonSet's logs", func() {
	daemonSet := prepareUnstructured("DaemonSet", daemonSetName, namespace)
	daemonSet.SetLabels(map[string]string{
		"app": "some-driver",
	})

	pattern := ".*driver loaded.*"
	shortLog := `1st line
2nd line`

	shortLogWithPattern := shortLog + `driver loaded
next line`

	longerLog := `hRsJDTECYVJcrTIyPCsJTy94kmOjv9eDZC4hMmMm
Of2Bol1IrIrch8oikJpvPpYpFLoL3JRPvg9ur7jaX
iqHzvzyEh7GJ7drJDCgW3FXekSfmFpDLFyHJ8lj81
zZJPaSyFS22PBWlSxHOsGTLaHRZwyANdwPRHKiuW6
LAIJPJUuQI3vpzi1gh4NoZuP9eFZcIGZbPwouqd23
Of2Bol1IrIrch8oikJpvPpYpFLoL3JRPvg9ur7jaX
iqHzvzyEh7GJ7drJDCgW3FXekSfmFpDLFyHJ8lj81
zZJPaSyFS22PBWlSxHOsGTLaHRZwyANdwPRHKiuW6
LAIJPJUuQI3vpzi1gh4NoZuP9eFZcIGZbPwouqd23`

	longerLogWithPattern := longerLog + `driver loaded
NoZuP9eFZcIGZbPwou2d23zZJPaSyFS22PBWlSxHO`

	prepareReq := func(log string) *restclient.Request {
		roundTripper := func(*http.Request) (*http.Response, error) {
			header := http.Header{}
			header.Set("Content-Type", "text/plain")

			body := io.NopCloser(strings.NewReader(log))

			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body:       body,
			}
			return resp, nil
		}
		fakeHTTPClient := fakerestclient.CreateHTTPClient(roundTripper)
		return restclient.NewRequestWithClient(nil, "",
			restclient.ClientContentConfig{}, fakeHTTPClient)
	}

	DescribeTable("searches Pods' log for a pattern",
		func(log string, shouldBeFound bool) {
			podName := daemonSetName + "-123456"

			mockClientsInterface.EXPECT().
				List(Any(), Any(), Any()).
				DoAndReturn(func(_ context.Context, obj client.ObjectList, _ ...client.ListOption) error {
					u := obj.(*unstructured.UnstructuredList)
					u.Items = append(u.Items, *prepareUnstructured("Pod", podName, namespace))
					return nil
				})

			mockClientsInterface.EXPECT().
				GetPodLogs(namespace, podName, Any()).
				Return(prepareReq(log))

			err := pa.ForDaemonSetLogs(context.Background(), daemonSet, pattern)
			if shouldBeFound {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not matched against"))
			}
		},
		Entry("short (100 < lines) log without pattern", shortLog, false),
		Entry("longer (lines > 100) log without pattern", longerLog, false),
		Entry("short (100 < lines) log", shortLogWithPattern, true),
		Entry("longer (lines > 100) log", longerLogWithPattern, true))

	Context("with 4 Pods", func() {
		pods := map[string]string{
			daemonSetName + "-1": shortLogWithPattern,
			daemonSetName + "-2": longerLogWithPattern,
			daemonSetName + "-3": shortLog,
			daemonSetName + "-4": longerLog,
		}

		It("will check logs Pod by Pod", func() {
			mockClientsInterface.EXPECT().
				List(Any(), Any(), Any()).
				DoAndReturn(func(_ context.Context, obj client.ObjectList, _ ...client.ListOption) error {
					u := obj.(*unstructured.UnstructuredList)
					for podName := range pods {
						u.Items = append(u.Items, *prepareUnstructured("Pod", podName, namespace))
					}
					return nil
				})

			mockClientsInterface.EXPECT().
				GetPodLogs(Any(), Any(), Any()).
				DoAndReturn(func(ns, name string, _ *v1.PodLogOptions) *restclient.Request {
					return prepareReq(pods[name])
				}).AnyTimes()

			err := pa.ForDaemonSetLogs(context.Background(), daemonSet, pattern)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not matched against"))
		})
	})
})

var _ = Context("Polling for resource unavailability", func() {
	daemonSet := prepareUnstructured("DaemonSet", daemonSetName, namespace)

	DescribeTable(
		"works as expected when",
		func(getErr error, assert func(error)) {
			mockClientsInterface.EXPECT().
				Get(Any(), types.NamespacedName{Namespace: namespace, Name: daemonSetName}, Any()).
				Return(getErr).AnyTimes()

			err := pa.ForResourceUnavailability(context.Background(), daemonSet)
			assert(err)
		},
		Entry(
			"object is not found",
			&apierrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound}},
			func(err error) {
				Expect(err).ToNot(HaveOccurred())
			}),
		Entry(
			"object exists",
			nil,
			func(err error) {
				Expect(err).To(HaveOccurred())
			}),
		Entry(
			"another error occurs",
			&apierrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonUnauthorized}},
			func(err error) {
				Expect(apierrors.IsUnauthorized(err)).To(BeTrue())
			}),
	)
})
