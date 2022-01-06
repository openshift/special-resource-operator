package helmer_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-psap/special-resource-operator/pkg/clients"
	"github.com/openshift-psap/special-resource-operator/pkg/helmer"
	helmerv1beta1 "github.com/openshift-psap/special-resource-operator/pkg/helmer/api/v1beta1"
	"github.com/openshift-psap/special-resource-operator/pkg/resource"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/repo"
	v1 "k8s.io/api/core/v1"
)

const pluginsDir = "../../helm-plugins"

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

var _ = Describe("helmer_AddorUpdateRepo", func() {
	It("file:// provider", func() {
		entry := repo.Entry{
			Name: "test",
			URL:  "file://testdata",
		}

		tempDir := GinkgoT().TempDir()

		repoConfigFile := filepath.Join(tempDir, "config.yaml")

		settings := cli.New()

		settings.PluginsDirectory = pluginsDir
		settings.RepositoryConfig = repoConfigFile
		settings.RepositoryCache = filepath.Join(tempDir, "cache")

		err := helmer.NewHelmer(mockCreator, settings, mockKubeClient).AddorUpdateRepo(&entry)
		Expect(err).NotTo(HaveOccurred())

		expectedContents := []byte(`apiVersion: ""
generated: "0001-01-01T00:00:00Z"
repositories:
- caFile: ""
  certFile: ""
  insecure_skip_tls_verify: false
  keyFile: ""
  name: test
  pass_credentials_all: false
  password: ""
  url: file://testdata
  username: ""
`)

		contents, err := os.ReadFile(repoConfigFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(contents).To(Equal(expectedContents))
	})
})

var _ = Describe("helmer_Load", func() {
	Context("file:// provider", func() {
		It("should return an error if the repository does not exist", func() {
			spec := helmerv1beta1.HelmChart{
				Repository: helmerv1beta1.HelmRepo{
					Name: "test",
					URL:  "file://invalid-path",
				},
			}

			settings := cli.New()

			settings.PluginsDirectory = pluginsDir

			_, err := helmer.NewHelmer(mockCreator, settings, mockKubeClient).Load(spec)
			Expect(err).To(HaveOccurred())
		})

		It("should return an error if the chart does not exist", func() {
			spec := helmerv1beta1.HelmChart{
				Name: "invalid-chart",
				Repository: helmerv1beta1.HelmRepo{
					Name: "test",
					URL:  "file://testdata",
				},
			}

			tempDir := GinkgoT().TempDir()

			repoConfigFile := filepath.Join(tempDir, "config.yaml")

			settings := cli.New()

			settings.PluginsDirectory = pluginsDir
			settings.RepositoryConfig = repoConfigFile
			settings.RepositoryCache = filepath.Join(tempDir, "cache")

			_, err := helmer.NewHelmer(mockCreator, settings, mockKubeClient).Load(spec)
			Expect(err).To(HaveOccurred())
		})

		It("should return no error if the chart exists", func() {
			spec := helmerv1beta1.HelmChart{
				Name: "test-chart",
				Repository: helmerv1beta1.HelmRepo{
					Name: "test",
					URL:  "file://testdata",
				},
			}

			tempDir := GinkgoT().TempDir()

			repoConfigFile := filepath.Join(tempDir, "config.yaml")

			settings := cli.New()

			settings.PluginsDirectory = pluginsDir
			settings.RepositoryConfig = repoConfigFile
			settings.RepositoryCache = filepath.Join(tempDir, "cache")

			chart, err := helmer.NewHelmer(mockCreator, settings, mockKubeClient).Load(spec)
			Expect(err).NotTo(HaveOccurred())

			Expect(chart.Name()).To(Equal("test-chart"))
			Expect(chart.Metadata.Version).To(Equal("0.1.0"))
		})
	})
})

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

		err := helmer.NewHelmer(mockCreator, cli.New(), mockKubeClient).InstallCRDs(context.TODO(), nil, owner, name, namespace)
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

		err := helmer.NewHelmer(mockCreator, cli.New(), mockKubeClient).InstallCRDs(context.TODO(), crds, owner, name, namespace)
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

		err := helmer.
			NewHelmer(mockCreator, cli.New(), mockKubeClient).
			Run(context.TODO(), ch, nil, owner, name, namespace, nil, "", "", false)
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

		err := helmer.
			NewHelmer(mockCreator, cli.New(), mockKubeClient).
			Run(context.TODO(), ch, nil, owner, name, namespace, nil, "", "", false)
		Expect(errors.Is(err, randomError)).To(BeTrue())
	})
})
