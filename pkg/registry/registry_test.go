package registry

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRegistry(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Registry Suite")
}

//var _ = Describe("registryFromImageURL", func() {
//	DescribeTable("should parse URLs as expected",
//		func(image, expectedHost string) {
//			r := registry{}
//			host, err := r.registryFromImageURL(image)
//			Expect(err).NotTo(HaveOccurred())
//			Expect(host).To(Equal(expectedHost))
//		},
//		Entry(nil, "registry.io/org/repo@sha256:123", "registry.io"),
//		Entry(nil, "//another-registry.io/org/repo@sha256:987", "another-registry.io"),
//	)
//})
//
//var _ = Describe("getImageRegistryCredentials", func() {
//	const (
//		expectedNamespace = "openshift-config"
//		expectedName      = "pull-secret"
//		expectedFile      = ".dockerconfigjson"
//		url               = "registry.io"
//		auth              = "123"
//		email             = "user@" + url
//	)
//
//	config := fmt.Sprintf(`{"auths":{"%s":{"auth":"%s","email":"%s"}}}`, url, auth, email)
//
//	var (
//		kubeClient *clients.MockClientsInterface
//		r          Registry
//	)
//
//	BeforeEach(func() {
//		ctrl := gomock.NewController(GinkgoT())
//		kubeClient = clients.NewMockClientsInterface(ctrl)
//		r = NewRegistry(kubeClient)
//	})
//
//	DescribeTable("should fail in following scenarios",
//		func(secret *v1.Secret, getError error, url string, expectedErrorStr string) {
//			kubeClient.EXPECT().
//				GetSecret(context.Background(), expectedNamespace, expectedName, gomock.Any()).
//				Return(secret, getError)
//
//			_, err := r.(*registry).getImageRegistryCredentials(context.Background(), url)
//			Expect(err).To(HaveOccurred())
//			Expect(err.Error()).To(ContainSubstring(expectedErrorStr))
//		},
//		Entry("no pull-secret in the cluster",
//			nil, errors.New(""), url,
//			"could not retrieve pull secrets"),
//		Entry("pull-secret has no data",
//			&v1.Secret{}, nil, url,
//			"could not find data"),
//		Entry("pull-secret doesn't have an entry for requested host",
//			&v1.Secret{Data: map[string][]byte{expectedFile: []byte(config)}}, nil, "other-registry.io",
//			"does not contain auth for registry"),
//	)
//
//	It("will work for expected scenario", func() {
//		pullSecret := &v1.Secret{
//			Data: map[string][]byte{
//				expectedFile: []byte(config),
//			},
//		}
//		kubeClient.EXPECT().
//			GetSecret(context.Background(), expectedNamespace, expectedName, gomock.Any()).
//			Return(pullSecret, nil)
//
//		da, err := r.(*registry).getImageRegistryCredentials(context.Background(), url)
//		Expect(err).NotTo(HaveOccurred())
//		Expect(da).To(Equal(dockerAuth{Auth: auth, Email: email}))
//	})
//})
