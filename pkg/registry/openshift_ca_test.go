package registry

import (
	"context"
	"errors"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/special-resource-operator/pkg/clients"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ = Describe("OpenShiftCAGetter", func() {
	var kubeClient *clients.MockClientsInterface

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		kubeClient = clients.NewMockClientsInterface(ctrl)
	})

	Describe("AdditionalTrustedCAs", func() {
		const imageConfigName = "cluster"

		It("should return an error if we cannot get the image.config.openshift.io", func() {
			kubeClient.
				EXPECT().
				GetImage(context.Background(), imageConfigName, metav1.GetOptions{}).
				Return(nil, errors.New("random error"))

			_, err := NewOpenShiftCAGetter(kubeClient).AdditionalTrustedCAs(context.Background())
			Expect(err).To(HaveOccurred())
		})

		It("should return an empty slice if the additionalTrustedCA is empty", func() {
			kubeClient.
				EXPECT().
				GetImage(context.Background(), imageConfigName, metav1.GetOptions{}).
				Return(&configv1.Image{}, nil)

			c, err := NewOpenShiftCAGetter(kubeClient).AdditionalTrustedCAs(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(c).To(BeEmpty())
		})

		It("should an empty slice if the configmap is empty", func() {
			const cmName = "cm-name"

			img := configv1.Image{
				Spec: configv1.ImageSpec{
					AdditionalTrustedCA: configv1.ConfigMapNameReference{Name: cmName},
				},
			}

			gomock.InOrder(
				kubeClient.
					EXPECT().
					GetImage(context.Background(), imageConfigName, metav1.GetOptions{}).
					Return(&img, nil),
				kubeClient.
					EXPECT().
					GetConfigMap(context.Background(), "openshift-config", cmName, metav1.GetOptions{}).
					Return(&v1.ConfigMap{}, nil),
			)

			c, err := NewOpenShiftCAGetter(kubeClient).AdditionalTrustedCAs(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(c).To(BeEmpty())
		})

		It("should work as expected", func() {
			const cmName = "cm-name"

			img := configv1.Image{
				Spec: configv1.ImageSpec{
					AdditionalTrustedCA: configv1.ConfigMapNameReference{Name: cmName},
				},
			}

			const (
				cmKey   = "key"
				cmValue = "value"
			)

			cm := v1.ConfigMap{
				Data: map[string]string{cmKey: cmValue},
			}

			gomock.InOrder(
				kubeClient.
					EXPECT().
					GetImage(context.Background(), imageConfigName, metav1.GetOptions{}).
					Return(&img, nil),
				kubeClient.
					EXPECT().
					GetConfigMap(context.Background(), "openshift-config", cmName, metav1.GetOptions{}).
					Return(&cm, nil),
			)

			c, err := NewOpenShiftCAGetter(kubeClient).AdditionalTrustedCAs(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(c).To(
				Equal(
					map[string][]byte{cmKey: []byte(cmValue)},
				),
			)
		})
	})

	Describe("CABundle", func() {
		const cmName = "user-ca-bundle"

		It("should return an error if we cannot fetch the ConfigMap", func() {
			randomError := errors.New("random error")

			kubeClient.
				EXPECT().
				GetConfigMap(context.Background(), "openshift-config", cmName, metav1.GetOptions{}).
				Return(nil, randomError)

			_, err := NewOpenShiftCAGetter(kubeClient).CABundle(context.Background())
			Expect(err).To(HaveOccurred())
		})

		It("should return an empty slice if the ConfigMap does not exist", func() {
			notFound := k8serrors.NewNotFound(schema.GroupResource{}, cmName)

			kubeClient.
				EXPECT().
				GetConfigMap(context.Background(), "openshift-config", cmName, metav1.GetOptions{}).
				Return(nil, notFound)

			s, err := NewOpenShiftCAGetter(kubeClient).CABundle(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(BeEmpty())
		})

		It("should return an empty slice if the ConfigMap does not have the expected key", func() {
			kubeClient.
				EXPECT().
				GetConfigMap(context.Background(), "openshift-config", cmName, metav1.GetOptions{}).
				Return(&v1.ConfigMap{}, nil)

			s, err := NewOpenShiftCAGetter(kubeClient).CABundle(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(BeEmpty())
		})

		It("should work as expected", func() {
			const value = "test-value"

			cm := v1.ConfigMap{
				Data: map[string]string{"ca-bundle.crt": value},
			}

			kubeClient.
				EXPECT().
				GetConfigMap(context.Background(), "openshift-config", cmName, metav1.GetOptions{}).
				Return(&cm, nil)

			s, err := NewOpenShiftCAGetter(kubeClient).CABundle(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(
				Equal([]byte(value)),
			)
		})
	})
})
