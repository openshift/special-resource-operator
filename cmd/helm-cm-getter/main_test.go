package main

import (
	"context"
	"io"
	"log"
	"net/url"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestHelmCmGetter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HelmCmGetter Suite")
}

var _ = Describe("HelmCmGetter", func() {
	Describe("ConfigMapGetter_Get", func() {
		parseURL := func(s string) *url.URL {
			u, err := url.Parse(s)
			Expect(err).NotTo(HaveOccurred())

			return u
		}

		ctx := context.Background()

		discardLogger := log.New(io.Discard, "", 0)

		It("should return an error when the ConfigMap does not exist", func() {
			cmg := ConfigMapGetter{
				kubeClient: fake.NewSimpleClientset(),
				logger:     discardLogger,
			}

			_, err := cmg.Get(ctx, parseURL("cm://some-ns/some-chart/index.yaml"))
			Expect(err).To(HaveOccurred())
		})

		When("ConfigMap exists", func() {
			const (
				name = "cm"
				ns   = "ns"
			)

			baseCM := func() *corev1.ConfigMap {
				return &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				}
			}

			It("should return an empty buffer when there is no index.yaml key", func() {
				cmg := ConfigMapGetter{
					kubeClient: fake.NewSimpleClientset(baseCM()),
					logger:     discardLogger,
				}

				b, err := cmg.Get(ctx, parseURL("cm://ns/cm/index.yaml"))
				Expect(err).NotTo(HaveOccurred())
				Expect(b).To(BeEmpty())
			})

			It("should return the data when index.yaml is present", func() {
				const contents = "test data"

				cm := baseCM()
				cm.Data = map[string]string{"index.yaml": contents}

				cmg := ConfigMapGetter{
					kubeClient: fake.NewSimpleClientset(cm),
					logger:     discardLogger,
				}

				b, err := cmg.Get(ctx, parseURL("cm://ns/cm/index.yaml"))
				Expect(err).NotTo(HaveOccurred())
				Expect(b).To(Equal([]byte(contents)))
			})

			It("should return the chart when it is present", func() {
				contents := []byte("test data")

				cm := baseCM()
				cm.BinaryData = map[string][]byte{"chart.tgz": contents}

				cmg := ConfigMapGetter{
					kubeClient: fake.NewSimpleClientset(cm),
					logger:     discardLogger,
				}

				b, err := cmg.Get(ctx, parseURL("cm://ns/cm/chart.tgz"))
				Expect(err).NotTo(HaveOccurred())
				Expect(b).To(Equal(contents))
			})
		})
	})
})
