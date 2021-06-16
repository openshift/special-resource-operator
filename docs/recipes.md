# How to build recipes

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
    - finalizer.sro.openshift.io
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
