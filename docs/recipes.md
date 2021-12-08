# How to build recipes

The Special Resource Operator (SRO) is based on Helm charts. A Helm chart is a recipe
to build a special resource. The integrated Helm support in SRO can use chart repositories
either via HTTP, OCI or file:///.

Most of the time the charts are packaged with SRO. In this way we have tested SRO recipes
for each K8S or OpenShift release.

An SRO recipe consists of an CR and a packaged Helm chart.

Here is a simple CR for an out-of-tree driver build

```yaml
apiVersion: sro.openshift.io/v1beta1
kind: SpecialResource
metadata:
  name: simple-kmod
spec:
  namespace: simple-kmod
  chart:
    name: simple-kmod
    version: 0.0.1
    repository:
      name: example
      url: file:///charts/example
  set:
    kind: Values
    apiVersion: sro.openshift.io/v1beta1
    kmodNames: ["simple-kmod", "simple-procfs-kmod"]
    buildArgs:
    - name: "KMODVER"
      value: "SRO"
  driverContainer:
    source:
      git:
        ref: "master"
        uri: "https://github.com/openshift-psap/kvc-simple-kmod.git"
```

SRO keeps an internal Helm repository of all packaged helm charts, and they are
organized in repositories, the same structure that helm uses for storing charts
online.

The `chart:` section tells SRO in which repository to find  the simple-kmod chart
alongside with a version. This is the same version you would specify in the
Chart.yaml in your helm chart.

SRO charts usually do not have a values.yaml because most of the information that
is needed to build an out-of-tree driver is gathered during runtime. See the next
section for "all" runtime variables.

The `set:` sections can be used to set values in the chart templates, think of it
as a programmatic approach to provide values. Those values will be coalesced with
the values.yaml if it exists.

The simple-kmod BuildConfig uses e.g. the buildArgs supplied in the `set:` section
in the BuildConfig template to populate arguments:

```yaml
        {{- range $arg := .Values.buildArgs }}
        - name: {{ $arg.name }}
          value: {{ $arg.value }}
        {{- end }}
```

The `driverContainer:` section describes how to build the driver-container an
extensive list of options is listed here: <https://github.com/openshift/enhancements/pull/357>

A SpecialResource can also have a dependency, a dependency is expressed again
with a `chart:` and `set:` section. The ping-pong special resource illustrates
this: charts/example/ping-pong-0.0.1/ping-pong.yaml.

We can use externally hosted charts, like the cert-manager helm chart and deploy
it via SRO. As stated above the SRO helm support also supports the file:///
transport, meaning we can of course also refer to charts in the internal
registry.

One can also attach metadata to SRO resources to be created, see: <https://www.openshift.com/blog/part-2-how-to-enable-hardware-accelerators-on-openshift-sro-building-blocks> for
further information.

## Ordering of Resource Creation

Helm per default has a specific ordering in which order resources should be created
when a chart is templated. SRO goes one step further and is using a specific naming
scheme of templates to force ordering between any resource.

A single file (template) in the templates' directory represents a state if the file
starts with a four-digit number. Each of these files are treated as states that
are executed in ascending numbering order. The ping-pong chart illustrates this.

It is also possible to mix states and non states in a chart. The states are executed
first and then the non-state templates.

SRO has also some advanced waiting callbacks for resources, e.g. we can wait for
a specific log in a Pod or wait for any other resource to be in a specific state.

Sometimes we don't want to start another DaemonSet before the DaemonSet in a
previous state is fully rolled out not only created by the services or daemons
inside the Pod/Container fully started.

## Runtime Variables

```yaml
buildArgs:
- name: KMODVER
  value: SRO
clusterUpgradeInfo:
  4.18.0-305.3.1.el8_4.x86_64:
    clusterVersion: "4.8"
    driverToolkit:
      imageURL: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:d07d95029663561dc58560751936dc9569bd77a397206e80fb5ab8778a56d920
      kernelFullVersion: 4.18.0-305.3.1.el8_4.x86_64
      oSVersion: "8.4"
      rTKernelFullVersion: 4.18.0-305.3.1.rt7.75.el8_4.x86_64
    oSVersion: "8.4"
clusterVersion: 4.8.0-fc.8
clusterVersionMajorMinor: "4.8"
driverToolkitImage: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:d07d95029663561dc58560751936dc9569bd77a397206e80fb5ab8778a56d920
groupName:
  csiDriver: csi-driver
  deviceDashboard: device-dashboard
  deviceFeatureDiscovery: device-feature-discovery
  deviceMonitoring: device-monitoring
  devicePlugin: device-plugin
  driverBuild: driver-build
  driverContainer: driver-container
  runtimeEnablement: runtime-enablement
kernelFullVersion: 4.18.0-305.3.1.el8_4.x86_64
kernelPatchVersion: 4.18.0-305

kmodNames:
- simple-kmod
- simple-procfs-kmod
operatingSystemDecimal: "8.4"
operatingSystemMajor: rhel8
operatingSystemMajorMinor: rhel8.4
osImageURL: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4ff8f292fc4f65e812c99b023204eff84d6737ac42dcd198e4792213a1471873
proxy:
  httpProxy: ""
  httpsProxy: ""
  noProxy: ""
  trustedCA: ""
pushSecretName: builder-dockercfg-sd6vp
specialresource:
  apiVersion: sro.openshift.io/v1beta1
  kind: SpecialResource
  metadata:
    creationTimestamp: "2021-06-15T08:24:12Z"
    finalizers:
    - sro.openshift.io/finalizer
    generation: 2
    name: simple-kmod
    resourceVersion: "16169837"
    uid: 76a56824-161b-45a3-b54f-b229a1a3ded8
  spec:
    chart:
      name: simple-kmod
      repository:
        caFile: ""
        certFile: ""
        insecure_skip_tls_verify: false
        keyFile: ""
        name: example
        password: ""
        url: file:///charts/example
        username: ""
      tags: []
      version: 0.0.1
    dependencies: null
    driverContainer:
      artifacts: {}
      source:
        git:
          ref: master
          uri: https://github.com/openshift-psap/kvc-simple-kmod.git
    forceUpgrade: false
    namespace: simple-kmod
    set:
      apiVersion: sro.openshift.io/v1beta1
      buildArgs:
      - name: KMODVER
        value: SRO
      kind: Values
      kmodNames:
      - simple-kmod
      - simple-procfs-kmod
  status:
    state: ""
updateVendor: ""
```
