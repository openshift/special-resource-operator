package storage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/storage"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

const (
	namespaceName = "test-ns"
	resourceName  = "test-resource"
)

var (
	ctrl                *gomock.Controller
	mockClient          *clients.MockClientsInterface
	nsn                 = types.NamespacedName{Namespace: namespaceName, Name: resourceName}
	unstructuredMatcher = gomock.AssignableToTypeOf(&unstructured.Unstructured{})
)

func TestStorage(t *testing.T) {
	RegisterFailHandler(Fail)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockClient = clients.NewMockClientsInterface(ctrl)

		clients.Interface = mockClient
	})

	AfterEach(func() {
		ctrl.Finish()
		clients.Interface = nil
	})

	RunSpecs(t, "Storage Suite")
}

var _ = Describe("CheckConfigMapEntry", func() {
	const key = "test-key"

	It("should return an error with no ConfigMap present", func() {
		randomError := errors.New("random error")

		mockClient.EXPECT().Get(context.TODO(), nsn, unstructuredMatcher).Return(randomError)

		_, err := storage.CheckConfigMapEntry(key, nsn)
		Expect(err).To(Equal(randomError))
	})

	It("should return an error with an empty ConfigMap", func() {
		mockClient.EXPECT().Get(context.TODO(), nsn, unstructuredMatcher)

		_, err := storage.CheckConfigMapEntry(key, nsn)
		Expect(err).To(HaveOccurred())
	})

	It("should return the expected value with a good ConfigMap", func() {
		const data = "test-data"

		mockClient.EXPECT().
			Get(context.TODO(), nsn, unstructuredMatcher).
			Do(func(_ context.Context, _ types.NamespacedName, uo *unstructured.Unstructured) {
				err := unstructured.SetNestedMap(uo.Object, map[string]interface{}{key: data}, "data")
				Expect(err).NotTo(HaveOccurred())
			})

		v, err := storage.CheckConfigMapEntry(key, nsn)

		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(data))
	})
})

var _ = Describe("GetConfigMap", func() {
	It("should return an error with no ConfigMap present", func() {
		randomError := errors.New("random error")

		mockClient.EXPECT().Get(context.TODO(), nsn, unstructuredMatcher).Return(randomError)

		_, err := storage.GetConfigMap(namespaceName, resourceName)
		Expect(err).To(HaveOccurred())
	})

	It("should work as expected", func() {
		data := map[string]string{"key": "value"}

		mockClient.EXPECT().Get(context.TODO(), nsn, unstructuredMatcher).
			Do(func(_ context.Context, _ types.NamespacedName, uo *unstructured.Unstructured) {
				err := unstructured.SetNestedStringMap(uo.Object, data, "data")
				Expect(err).NotTo(HaveOccurred())
			})

		res, err := storage.GetConfigMap(namespaceName, resourceName)
		Expect(err).NotTo(HaveOccurred())

		resData, found, err := unstructured.NestedStringMap(res.Object, "data")
		Expect(err).NotTo(HaveOccurred())

		Expect(found).To(BeTrue())
		Expect(resData).To(Equal(data))
	})
})

var _ = Describe("UpdateConfigMapEntry", func() {
	It("should return an error when the ConfigMap does not exist", func() {
		randomError := errors.New("random error")

		mockClient.EXPECT().Get(context.TODO(), nsn, unstructuredMatcher).Return(randomError)

		err := storage.UpdateConfigMapEntry("any-key", "any-value", nsn)
		Expect(err).To(HaveOccurred())
	})

	It("set a key that does not already exist", func() {
		const (
			key   = "key"
			value = "value"
		)

		gomock.InOrder(
			mockClient.EXPECT().Get(context.TODO(), nsn, unstructuredMatcher),
			mockClient.EXPECT().
				Update(context.TODO(), unstructuredMatcher).
				Do(func(_ context.Context, uo *unstructured.Unstructured) {
					cm := v1.ConfigMap{}

					err := runtime.DefaultUnstructuredConverter.FromUnstructured(uo.Object, &cm)
					Expect(err).NotTo(HaveOccurred())
					Expect(cm.Data).To(HaveKeyWithValue(key, value))
				}),
		)

		err := storage.UpdateConfigMapEntry(key, value, nsn)
		Expect(err).NotTo(HaveOccurred())
	})

	It("set a key that already exists", func() {
		const (
			key      = "key"
			newValue = "new-value"
		)

		gomock.InOrder(
			mockClient.EXPECT().
				Get(context.TODO(), nsn, unstructuredMatcher).
				Do(func(_ context.Context, _ types.NamespacedName, uo *unstructured.Unstructured) {
					err := unstructured.SetNestedStringMap(uo.Object, map[string]string{key: "oldvalue"}, "data")
					Expect(err).NotTo(HaveOccurred())
				}),
			mockClient.EXPECT().
				Update(context.TODO(), unstructuredMatcher).
				Do(func(_ context.Context, uo *unstructured.Unstructured) {
					cm := v1.ConfigMap{}

					err := runtime.DefaultUnstructuredConverter.FromUnstructured(uo.Object, &cm)
					Expect(err).NotTo(HaveOccurred())
					Expect(cm.Data).To(HaveKeyWithValue(key, newValue))
				}),
		)
		err := storage.UpdateConfigMapEntry(key, newValue, nsn)
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("DeleteConfigMapEntry", func() {
	It("should return an error when the ConfigMap does not exist", func() {
		randomError := errors.New("random error")

		mockClient.EXPECT().Get(context.TODO(), nsn, unstructuredMatcher).Return(randomError)

		err := storage.DeleteConfigMapEntry("any-key", nsn)
		Expect(err).To(HaveOccurred())
	})

	It("should not return an error when the key does not exist", func() {
		mockClient.EXPECT().Get(context.TODO(), nsn, unstructuredMatcher)

		err := storage.DeleteConfigMapEntry("some-other-key", nsn)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should delete the key when the key exists", func() {
		const (
			key      = "key"
			otherKey = "other-key"
			value    = "value"
		)

		data := map[string]string{key: value, otherKey: "other-value"}

		gomock.InOrder(
			mockClient.EXPECT().
				Get(context.TODO(), nsn, unstructuredMatcher).
				Do(func(_ context.Context, _ types.NamespacedName, uo *unstructured.Unstructured) {
					err := unstructured.SetNestedStringMap(uo.Object, data, "data")
					Expect(err).NotTo(HaveOccurred())
				}),
			mockClient.EXPECT().
				Update(context.TODO(), unstructuredMatcher).
				Do(func(_ context.Context, uo *unstructured.Unstructured) {
					data, found, err := unstructured.NestedStringMap(uo.Object, "data")
					Expect(err).NotTo(HaveOccurred())
					Expect(found).To(BeTrue())
					Expect(data).NotTo(HaveKey(otherKey))
					Expect(data).To(HaveKey(key))
				}),
		)
		err := storage.DeleteConfigMapEntry(otherKey, nsn)
		Expect(err).NotTo(HaveOccurred())
	})
})
