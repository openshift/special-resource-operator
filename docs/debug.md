# How to debug recipes

The Special Resource Operatore (SRO) can read the states either from the local,
http or a ConfigMap resource.

To create the states in the template directory as a CM we can run the
following:

```bash
VERSION=0.0.1 REPO=example SPECIALRESOURCE=multi-build make assets
```

This command will create a CM with all the states and SRO will use them in the
next reconcilation loop. This makes it easy to override manifests during
development without rebuilding SRO.

To update the CR one can run:

```bash
VERSION=0.0.1 REPO=example SPECIALRESOURCE=multi-build make
```

For this to work one has to create a `kustomization.yaml` that will be processed
during a make run. One cannot just override one state, all states are
read from one source (http, local, or ConfigMap). Here is an `kustomization.yaml` as an example from the
`multi-build` recipe.

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

generatorOptions:
  disableNameSuffixHash: true

configMapGenerator:
- files:
  - templates/0000-buildconfig.yaml
  - templates/1000-buildconfig.yaml
  - templates/2000-imagesign.yaml
  name: multi-build
namespace: multi-build
```

Another field was added to the CR, namely `debug` that can be set to true to get
all the manifests, hooks and values printed on the console. Can be valuable for
verifying if all `Values` are set and correclty interpreted.

```yaml
apiVersion: sro.openshift.io/v1beta1
kind: SpecialResource
metadata:
  name: multi-build
spec:
  debug: false
  namespace: multi-build
  chart:
    name: multi-build
    version: 0.0.1
    repository:
      name: example
      url: file:///charts/example
  set:
    kind: Values
    apiVersion: sro.openshift.io/v1beta1
    pushSecret: openshift-psap-multibuild-pull-secret
    imageToSign: docker.io/zvonkok/{{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}:v{{.Values.kernelFullVersion}}
    cosignPassword: strongpassword
    buildArgs:
    - name: "KMODVER"
      value: "{{ .Values.kernelFullVersion }}"
  driverContainer:
    source:
      git:
        ref: "master"
        uri: "https://github.com/openshift-psap/kvc-simple-kmod.git"
```

SRO will print each complete state the corresponding values.
