package helmer

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift/special-resource-operator/pkg/clients"
	"github.com/openshift/special-resource-operator/pkg/resource"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli"
	v1 "k8s.io/api/core/v1"
)

var (
	ctrl           *gomock.Controller
	mockCreator    *resource.MockCreator
	mockKubeClient *clients.MockClientsInterface
)

func TestHelmer(t *testing.T) {
	RegisterFailHandler(Fail)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockCreator = resource.NewMockCreator(ctrl)
		mockKubeClient = clients.NewMockClientsInterface(ctrl)
	})

	RunSpecs(t, "Helmer Suite")
}

var _ = Describe("helmer_InstallCRDs", func() {
	const (
		name      = "some-name"
		namespace = "some-namespace"
	)

	owner := &v1.Pod{}

	It("should return an error when a CRD cannot be created", func() {
		randomError := errors.New("random error")

		mockCreator.
			EXPECT().
			CreateFromYAML(context.TODO(), nil, false, owner, name, namespace, nil, "", "").
			Return(randomError)

		h, err := newHelmerWithVersions(mockCreator, cli.New(), nil, nil, mockKubeClient)
		Expect(err).NotTo(HaveOccurred())
		err = h.InstallCRDs(context.TODO(), nil, owner, name, namespace)
		Expect(err).To(Equal(randomError))
	})

	It("should install all CRDs", func() {
		crds := []chart.CRD{
			{
				Filename: "/path/to/crd0",
				File:     &chart.File{Data: []byte("abc")},
			},
			{
				Filename: "/path/to/crd1",
				File:     &chart.File{Data: []byte("def")},
			},
		}

		manifests := []byte(`---
# Source: /path/to/crd0
abc
---
# Source: /path/to/crd1
def
`)

		mockCreator.
			EXPECT().
			CreateFromYAML(context.TODO(), manifests, false, owner, name, namespace, nil, "", "")

		h, err := newHelmerWithVersions(mockCreator, cli.New(), nil, nil, mockKubeClient)
		Expect(err).NotTo(HaveOccurred())
		err = h.InstallCRDs(context.TODO(), crds, owner, name, namespace)
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("helmer_Run", func() {
	const (
		name      = "some-name"
		namespace = "some-namespace"
	)

	owner := &v1.Pod{}

	It("should fail with an unsupported chart type", func() {
		ch := chart.Chart{
			Metadata: &chart.Metadata{
				Name: "invalid-type",
				Type: "random",
			},
		}

		h, err := newHelmerWithVersions(mockCreator, cli.New(), nil, nil, mockKubeClient)
		Expect(err).NotTo(HaveOccurred())
		err = h.Run(context.TODO(), ch, nil, owner, name, namespace, nil, "", "", false)
		Expect(err).To(HaveOccurred())
	})

	It("should fail if CRD installation fails", func() {
		ch := chart.Chart{
			Files: []*chart.File{
				{
					Name: "crds/test.yml",
					Data: nil,
				},
			},
			Metadata: &chart.Metadata{
				Name: name,
				Type: "application",
			},
		}

		randomError := errors.New("random error")

		mockCreator.
			EXPECT().
			CreateFromYAML(context.TODO(), gomock.Any(), false, owner, name, namespace, nil, "", "").
			Return(randomError)
		h, err := newHelmerWithVersions(mockCreator, cli.New(), nil, nil, mockKubeClient)
		Expect(err).NotTo(HaveOccurred())
		err = h.Run(context.TODO(), ch, nil, owner, name, namespace, nil, "", "", false)
		Expect(errors.Is(err, randomError)).To(BeTrue())
	})
})
