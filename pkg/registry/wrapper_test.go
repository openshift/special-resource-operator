package registry

import (
	context "context"
	"errors"
	"os"
	"strings"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	v1 "k8s.io/api/core/v1"

	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/special-resource-operator/pkg/clients"
)

var _ = Describe("Manifest", func() {

	const (
		registriesConfFile     = "testdata/registries.conf"
		expectedNamespace      = "openshift-config"
		expectedName           = "pull-secret"
		pullData               = `{"auths":{"registry0.com":{"auth":"dXNlcm5hbWU6cGFzc3dvcmQK","email":"user@gmail.com"}}}`
		img0                   = "registry0.com/org/img"
		img1                   = "registry1.com/org/img"
		registry0              = "registry0.com"
		mirrorRegistry0        = "mirror-registry0.com"
		additionalTrustedCAsCM = "disconnected-edge"
		certFileName           = "testdata/custom-ca-cert.pem"
	)

	var (
		ctrl       *gomock.Controller
		kubeClient *clients.MockClientsInterface
		cw         CraneWrapper
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		kubeClient = clients.NewMockClientsInterface(ctrl)
		cw = NewCraneWrapper(kubeClient, registriesConfFile)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Context("getPullSourcesForImageReference - dependency method", func() {

		It("should return the mirror image before the origin image", func() {
			ps, err := cw.(*craneWrapper).getPullSourcesForImageReference("registry0.com/org/img")

			Expect(err).NotTo(HaveOccurred())
			Expect(len(ps)).To(Equal(2))
			Expect(ps[0].Reference.String()).To(Equal("mirror-registry0.com/mirror-org/img"))
			Expect(ps[1].Reference.String()).To(Equal("registry0.com/org/img"))
		})

		It("should return the origin non-digest image if mirror is set for digest only", func() {
			ps, err := cw.(*craneWrapper).getPullSourcesForImageReference("registry1.com/org/img")

			Expect(err).NotTo(HaveOccurred())
			Expect(len(ps)).To(Equal(1))
			Expect(ps[0].Reference.String()).To(Equal("registry1.com/org/img"))
		})

		It("should return the mirror digest image if mirror is set for digest only", func() {
			ps, err := cw.(*craneWrapper).getPullSourcesForImageReference("registry1.com/org/img@sha256:0661d0560654e7e2d1761e883ffdd6c482c8c8f37e60608bb59c44fa81a3f0bb")

			Expect(err).NotTo(HaveOccurred())
			Expect(len(ps)).To(Equal(2))
			Expect(ps[0].Reference.String()).To(Equal("mirror-registry1.com/mirror-org/img@sha256:0661d0560654e7e2d1761e883ffdd6c482c8c8f37e60608bb59c44fa81a3f0bb"))
			Expect(ps[1].Reference.String()).To(Equal("registry1.com/org/img@sha256:0661d0560654e7e2d1761e883ffdd6c482c8c8f37e60608bb59c44fa81a3f0bb"))
		})

		It("should return the origin image if there is no mirror config", func() {
			ps, err := cw.(*craneWrapper).getPullSourcesForImageReference("non-mirror-registry.com/org/img")

			Expect(err).NotTo(HaveOccurred())
			Expect(len(ps)).To(Equal(1))
			Expect(ps[0].Reference.String()).To(Equal("non-mirror-registry.com/org/img"))
		})

		It("should fail if registries.conf doesn't exist on the host", func() {
			cw = NewCraneWrapper(kubeClient, "/non/existance/registries.conf")
			_, err := cw.(*craneWrapper).getPullSourcesForImageReference("registry0.com/org/img")

			Expect(err).To(HaveOccurred())
		})
	})

	Context("getAuthForRegistry - dependency method", func() {

		It("should fail if no pull-secret in the cluster", func() {
			kubeClient.EXPECT().GetSecret(context.Background(), expectedNamespace, expectedName, gomock.Any()).Return(nil, errors.New("some error"))

			_, err := cw.(*craneWrapper).getAuthForRegistry(context.Background(), registry0)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("could not retrieve pull secrets"))
		})

		It("should fail if pull-secret has no data", func() {
			pullSecret := &v1.Secret{
				Data: map[string][]byte{
					".NONdockerconfigjson": []byte(pullData),
				},
			}
			kubeClient.EXPECT().GetSecret(context.Background(), expectedNamespace, expectedName, gomock.Any()).Return(pullSecret, nil)

			_, err := cw.(*craneWrapper).getAuthForRegistry(context.Background(), registry0)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("could not find data"))
		})

		It("should fail if pull-secret doesn't have an entry for requested host", func() {
			pullSecret := &v1.Secret{
				Data: map[string][]byte{
					".dockerconfigjson": []byte(pullData),
				},
			}
			kubeClient.EXPECT().GetSecret(context.Background(), expectedNamespace, expectedName, gomock.Any()).Return(pullSecret, nil)

			_, err := cw.(*craneWrapper).getAuthForRegistry(context.Background(), "other-registry.com")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(ErrNoAuthForRegistry.Error()))
		})

		It("will work for expected scenario", func() {
			pullSecret := &v1.Secret{
				Data: map[string][]byte{
					".dockerconfigjson": []byte(pullData),
				},
			}
			kubeClient.EXPECT().GetSecret(context.Background(), expectedNamespace, expectedName, gomock.Any()).Return(pullSecret, nil)

			_, err := cw.(*craneWrapper).getAuthForRegistry(context.Background(), registry0)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("getClusterCustomCertPool - dependency method", func() {

		It("should fail if we can't access the image.config.openshift.io", func() {

			kubeClient.EXPECT().GetImage(context.TODO(), imageClusterName, gomock.Any()).Return(nil, errors.New("some error"))

			_, err := cw.(*craneWrapper).getClusterCustomCertPool(context.TODO())
			Expect(err).To(HaveOccurred())
		})

		It("should add a certificate to the pool if the user had configured custom certificates in the cluster", func() {

			img := &configv1.Image{}
			kubeClient.EXPECT().GetImage(context.TODO(), imageClusterName, gomock.Any()).Return(img, nil)

			certPool, err := cw.(*craneWrapper).getClusterCustomCertPool(context.TODO())
			Expect(err).NotTo(HaveOccurred())
			systemCertInPool := len(certPool.Subjects())

			img = &configv1.Image{
				Spec: configv1.ImageSpec{
					AdditionalTrustedCA: configv1.ConfigMapNameReference{
						Name: additionalTrustedCAsCM,
					},
				},
			}
			data, err := os.ReadFile(certFileName)
			Expect(err).NotTo(HaveOccurred())
			cm := &v1.ConfigMap{
				Data: map[string]string{"some-registry": string(data)},
			}
			kubeClient.EXPECT().GetImage(context.TODO(), imageClusterName, gomock.Any()).Return(img, nil)
			kubeClient.EXPECT().GetConfigMap(context.TODO(), configNamespace, additionalTrustedCAsCM, gomock.Any()).Return(cm, nil)

			certPool, err = cw.(*craneWrapper).getClusterCustomCertPool(context.TODO())
			Expect(err).NotTo(HaveOccurred())
			totalCertInPool := len(certPool.Subjects())

			Expect(totalCertInPool).To(Equal(systemCertInPool + 1))
		})
	})

	It("is redirected to the mirrored image", func() {
		img := &configv1.Image{}
		pullSecret := &v1.Secret{
			Data: map[string][]byte{
				".dockerconfigjson": []byte(pullData),
			},
		}
		kubeClient.EXPECT().GetImage(context.TODO(), imageClusterName, gomock.Any()).Return(img, nil)
		kubeClient.EXPECT().GetSecret(context.TODO(), configNamespace, pullSecretName, gomock.Any()).Return(pullSecret, nil)

		_, err := cw.Manifest(context.TODO(), img0)
		// That registry doesn't exist so we will fail. We just want to make
		// sure we try to access the mirror registry
		Expect(err.Error()).To(ContainSubstring(mirrorRegistry0))
	})

	It("is accessing the correct image if no mirror was set", func() {
		img := &configv1.Image{}
		pullSecret := &v1.Secret{
			Data: map[string][]byte{
				".dockerconfigjson": []byte(pullData),
			},
		}
		kubeClient.EXPECT().GetImage(context.TODO(), imageClusterName, gomock.Any()).Return(img, nil)
		kubeClient.EXPECT().GetSecret(context.TODO(), configNamespace, pullSecretName, gomock.Any()).Return(pullSecret, nil)

		_, err := cw.Manifest(context.TODO(), img1)
		// That registry doesn't exist so we will fail. We just want to make
		// sure there is no redirection to some mirror registry
		Expect(err.Error()).To(ContainSubstring(strings.Split(img1, "/")[0]))
		Expect(err.Error()).NotTo(ContainSubstring("mirror"))
	})
})
