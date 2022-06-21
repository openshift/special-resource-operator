module github.com/openshift-psap/special-resource-operator

go 1.17

require (
	//FIXME:ybettan: remove?
	//<<<<<<< HEAD
	github.com/go-logr/logr v1.2.2
	//FIXME:ybettan: remove?
	//=======
	//	github.com/go-logr/logr v1.2.2
	//	github.com/golang/mock v1.6.0
	//>>>>>>> 08266589 (Adding support for disconnected clusters. (#226))
	github.com/google/go-containerregistry v0.5.2-0.20210601193515-0ffa4a5c8691
	github.com/mitchellh/hashstructure/v2 v2.0.1
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.19.0
	github.com/openshift/api v0.0.0-20210924154557-a4f696157341
	github.com/openshift/client-go v0.0.0-20210916133943-9acee1a0fb83
	github.com/openshift/library-go v0.0.0-20211103140146-29c9bb8362e2
	github.com/openshift/machine-config-operator v0.0.1-0.20210514234214-c415ce6aed25
	github.com/pkg/errors v0.9.1
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.42.1
	//FIXME:ybettan: remove?
	//<<<<<<< HEAD
	github.com/prometheus/client_golang v1.12.1
	github.com/stretchr/testify v1.7.1 // indirect
	golang.org/x/oauth2 v0.0.0-20210819190943-2bc19b11175f // indirect
	//FIXME:ybettan: remove?
	//=======
	//	github.com/prometheus/client_golang v1.12.1
	//	github.com/prometheus/client_model v0.2.0
	//	github.com/stretchr/testify v1.7.1 // indirect
	//	golang.org/x/oauth2 v0.0.0-20210819190943-2bc19b11175f // indirect
	//>>>>>>> 08266589 (Adding support for disconnected clusters. (#226))
	gopkg.in/yaml.v2 v2.4.0
	helm.sh/helm/v3 v3.7.1
	k8s.io/api v0.23.5
	k8s.io/apimachinery v0.23.5
	k8s.io/cli-runtime v0.23.5
	k8s.io/client-go v0.23.5
	sigs.k8s.io/controller-runtime v0.11.2
	sigs.k8s.io/controller-tools v0.6.1
	sigs.k8s.io/yaml v1.3.0
)

require (
	github.com/containers/image/v5 v5.21.1
	github.com/docker/cli v20.10.7+incompatible
	github.com/golang/mock v1.6.0
	github.com/onsi/ginkgo/v2 v2.1.4
	github.com/openshift/special-resource-operator v0.0.0-20220621204039-5e3ec3e8eaba
)

require (
	github.com/containers/storage v1.40.0 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
)

require (
	cloud.google.com/go v0.81.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/BurntSushi/toml v1.1.0 // indirect
	github.com/MakeNowJust/heredoc v0.0.0-20170808103936-bb23615498cd // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.1.1 // indirect
	github.com/Masterminds/sprig/v3 v3.2.2 // indirect
	github.com/Masterminds/squirrel v1.5.0 // indirect
	github.com/PuerkitoBio/purell v1.1.1 // indirect
	github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578 // indirect
	github.com/asaskevich/govalidator v0.0.0-20200428143746-21a406dcc535 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	//FIXME:ybettan: remove?
	//<<<<<<< HEAD
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/containerd/containerd v1.6.1 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.11.4 // indirect
	github.com/cyphar/filepath-securejoin v0.2.3 // indirect
	//FIXME:ybettan: remove?
	//=======
	//	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	//	github.com/containerd/containerd v1.6.1 // indirect
	//	github.com/containerd/stargz-snapshotter/estargz v0.11.4 // indirect
	//	github.com/containers/storage v1.40.0 // indirect
	//	github.com/cyphar/filepath-securejoin v0.2.3 // indirect
	//>>>>>>> 08266589 (Adding support for disconnected clusters. (#226))
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/docker/distribution v2.8.1+incompatible // indirect
	github.com/docker/docker v20.10.14+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.6.4 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	//FIXME:ybettan: remove?
	//<<<<<<< HEAD
	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	github.com/exponent-io/jsonpath v0.0.0-20151013193312-d6023ce2651d // indirect
	github.com/fatih/color v1.12.0 // indirect
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	//FIXME:ybettan: remove?
	//=======
	//	github.com/elazarl/goproxy v0.0.0-20190911111923-ecfe977594f1 // indirect
	//	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	//	github.com/exponent-io/jsonpath v0.0.0-20151013193312-d6023ce2651d // indirect
	//	github.com/fatih/color v1.12.0 // indirect
	//	github.com/fsnotify/fsnotify v1.5.1 // indirect
	//>>>>>>> 08266589 (Adding support for disconnected clusters. (#226))
	github.com/go-errors/errors v1.0.1 // indirect
	github.com/go-logr/zapr v1.2.3 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.19.5 // indirect
	github.com/go-openapi/swag v0.19.14 // indirect
	github.com/gobuffalo/flect v0.2.3 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/btree v1.0.1 // indirect
	github.com/google/go-cmp v0.5.6 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/googleapis/gnostic v0.5.5 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/gosuri/uitable v0.0.4 // indirect
	github.com/gregjones/httpcache v0.0.0-20180305231024-9cad4c3443a7 // indirect
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/jmoiron/sqlx v1.3.1 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	//FIXME:ybettan: remove?
	//<<<<<<< HEAD
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.15.2 // indirect
	//FIXME:ybettan: remove?
	//=======
	//	github.com/json-iterator/go v1.1.12 // indirect
	//	github.com/klauspost/compress v1.15.2 // indirect
	//>>>>>>> 08266589 (Adding support for disconnected clusters. (#226))
	github.com/lann/builder v0.0.0-20180802200727-47ae307949d0 // indirect
	github.com/lann/ps v0.0.0-20150810152359-62de8c46ede0 // indirect
	github.com/lib/pq v1.10.0 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/mattn/go-colorable v0.1.8 // indirect
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/mitchellh/copystructure v1.1.1 // indirect
	github.com/mitchellh/go-wordwrap v1.0.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.1 // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/moby/sys/mountinfo v0.6.1 // indirect
	github.com/moby/term v0.0.0-20210610120745-9d4ed1856297 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.3-0.20211202193544-a5463b7f9c84 // indirect
	github.com/opencontainers/runc v1.1.1 // indirect
	github.com/opencontainers/runtime-spec v1.0.3-0.20210326190908-1c3f411f0417 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	//FIXME:ybettan: remove?
	//<<<<<<< HEAD
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.32.1 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	//FIXME:ybettan: remove?
	//=======
	//	github.com/prometheus/common v0.32.1 // indirect
	//	github.com/prometheus/procfs v0.7.3 // indirect
	//	github.com/rivo/uniseg v0.2.0 // indirect
	//>>>>>>> 08266589 (Adding support for disconnected clusters. (#226))
	github.com/rubenv/sql-migrate v0.0.0-20210614095031-55d5740dbbcc // indirect
	github.com/russross/blackfriday v1.6.0 // indirect
	github.com/shopspring/decimal v1.2.0 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/spf13/cast v1.3.1 // indirect
	github.com/spf13/cobra v1.4.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/syndtr/gocapability v0.0.0-20200815063812-42c35b437635 // indirect
	github.com/vbatts/tar-split v0.11.2 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/xlab/treeprint v0.0.0-20181112141820-a009c3971eca // indirect
	go.starlark.net v0.0.0-20200306205701-8dd3e2ee1dd5 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.19.1 // indirect
	golang.org/x/crypto v0.0.0-20211215153901-e495a2d5b3d3 // indirect
	golang.org/x/mod v0.6.0-dev.0.20220106191415-9b9b3d81d5e3 // indirect
	//FIXME:ybettan: remove?
	//<<<<<<< HEAD
	golang.org/x/net v0.0.0-20220225172249-27dd8689420f // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
	golang.org/x/sys v0.0.0-20220422013727-9388b58f7150 // indirect
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211 // indirect
	golang.org/x/text v0.3.7 // indirect
	//FIXME:ybettan: remove?
	//=======
	//	golang.org/x/net v0.0.0-20220225172249-27dd8689420f // indirect
	//	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
	//	golang.org/x/sys v0.0.0-20220422013727-9388b58f7150 // indirect
	//	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211 // indirect
	//	golang.org/x/text v0.3.7 // indirect
	//>>>>>>> 08266589 (Adding support for disconnected clusters. (#226))
	golang.org/x/time v0.0.0-20210723032227-1f47c861a9ac // indirect
	golang.org/x/tools v0.1.10 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	gomodules.xyz/jsonpatch/v2 v2.2.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20220304144024-325a89244dc8 // indirect
	google.golang.org/grpc v1.44.0 // indirect
	google.golang.org/protobuf v1.27.1 // indirect
	gopkg.in/gorp.v1 v1.7.2 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
	k8s.io/apiextensions-apiserver v0.23.5 // indirect
	k8s.io/apiserver v0.23.5 // indirect
	k8s.io/component-base v0.23.5 // indirect
	k8s.io/klog/v2 v2.30.0 // indirect
	k8s.io/kube-openapi v0.0.0-20211115234752-e816edb12b65 // indirect
	k8s.io/kubectl v0.22.1 // indirect
	k8s.io/utils v0.0.0-20211116205334-6203023598ed // indirect
	oras.land/oras-go v0.4.0 // indirect
	sigs.k8s.io/json v0.0.0-20211020170558-c049b76a60c6 // indirect
	sigs.k8s.io/kustomize/api v0.10.1 // indirect
	sigs.k8s.io/kustomize/kyaml v0.13.0 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.1 // indirect
)
