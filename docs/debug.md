# How to debug recipes

The Special Resource Operator (SRO) can read the states either from the local,
HTTP or a ConfigMap (CM) resource.

To create the states in the template directory as a CM we can run the
following:

```bash
VERSION=0.0.1 REPO=example SPECIALRESOURCE=multi-build make chart
```

This command will create a CM with all the states and SRO will use them in the
next reconciliation loop. This makes it easy to override manifests during
development without rebuilding SRO.

To update the CR one can run:

```bash
SPECIALRESOURCE=multi-build REPO=example VERSION=0.0.1 make chart
```

```bash
VERSION=0.0.1 REPO=example SPECIALRESOURCE=multi-build make
```

ConfigMap is a protocol handler to Helm. Charts can now also be indexed from a ConfigMap.
The command above creates a CM with the chart embedded as binaryData.

Update the CR to use `cm://` protocol handler in SRO. URL schema is
`cm://<NAMESPACE>/<SPECIALRESOURCE>-chart`

```yaml
 spec:
  chart:
    name: multi-build
    repository:
      caFile: ""
      certFile: ""
      insecure_skip_tls_verify: false
      keyFile: ""
      name: example
      password: ""
      url: cm://multi-build/multi-build-chart
```

Another field was added to the CR, namely `debug` that can be set to true to get
all the manifests, hooks and values printed on the console. Can be valuable for
verifying if all `Values` are set and correctly interpreted.

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
