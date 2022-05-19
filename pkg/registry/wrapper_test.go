package registry

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift/special-resource-operator/pkg/clients"
	v1 "k8s.io/api/core/v1"
)

var _ = Describe("craneWrapper", func() {
	Describe("getAuthForRegistry", func() {
		const (
			expectedNamespace = "openshift-config"
			expectedName      = "pull-secret"
			expectedFile      = ".dockerconfigjson"
			url               = "registry.io"
			auth              = "dXNlcm5hbWU6cGFzc3dvcmQK" // base64("username:secret")
			email             = "user@" + url
		)

		config := fmt.Sprintf(`{"auths":{"%s":{"auth":"%s","email":"%s"}}}`, url, auth, email)

		var (
			cpg        *MockCertPoolGetter
			kubeClient *clients.MockClientsInterface
			mr         *MockMirrorResolver
			cw         CraneWrapper
		)

		BeforeEach(func() {
			ctrl := gomock.NewController(GinkgoT())
			cpg = NewMockCertPoolGetter(ctrl)
			kubeClient = clients.NewMockClientsInterface(ctrl)
			mr = NewMockMirrorResolver(ctrl)
			cw = NewCraneWrapper(
				cpg,
				kubeClient, mr)
		})

		DescribeTable("should fail in following scenarios",
			func(secret *v1.Secret, getError error, url string, expectedErrorStr string) {
				kubeClient.EXPECT().
					GetSecret(context.Background(), expectedNamespace, expectedName, gomock.Any()).
					Return(secret, getError)

				_, err := cw.(*craneWrapper).getAuthForRegistry(context.Background(), url)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedErrorStr))
			},
			Entry("no pull-secret in the cluster",
				nil, errors.New(""), url,
				"could not retrieve pull secrets"),
			Entry("pull-secret has no data",
				&v1.Secret{}, nil, url,
				"could not find data"),
			Entry("pull-secret doesn't have an entry for requested host",
				&v1.Secret{Data: map[string][]byte{expectedFile: []byte(config)}}, nil, "other-registry.io",
				ErrNoAuthForRegistry.Error()),
		)

		It("will work for expected scenario", func() {
			pullSecret := &v1.Secret{
				Data: map[string][]byte{
					expectedFile: []byte(config),
				},
			}
			kubeClient.EXPECT().
				GetSecret(context.Background(), expectedNamespace, expectedName, gomock.Any()).
				Return(pullSecret, nil)

			_, err := cw.(*craneWrapper).getAuthForRegistry(context.Background(), url)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})