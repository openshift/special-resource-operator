# Hub & spokes OOT driver deployment using SRO, ACM and MCO.

## Table of Contents
* [Resources](#resources)
* [Introduction](#introduction)
* [How?](#how)
  * [Minimum cluster setup](#minimum-cluster-setup)
  * [SRO](#sro)
  * [Helm charts](#helm-charts)
* [Example](#example)
  * [ACM-ICE Recipe](#acm-ice-recipe)
  * [Connected vs disconnected](#connected-vs-disconnected)
  * [Checks](#checks)

## Resources
* [Demo recording](https://drive.google.com/file/d/1jFqWMpqaX5jWrdGTCbpYemxhIke_XM06/view?usp=sharing)
* [Slides](https://docs.google.com/presentation/d/1FUJTVV10i4uuP2bxYhHL-JAgqflS3XBIlFIFly-uEWE/edit?usp=sharing)

## Introduction
This proof of concept demonstrates a working proposal of delivering out of tree drivers to spoke clusters by:
* Building driver containers in the hub.
* Losing NFD dependency.
* Watching hub cluster resources to extract how many OCP versions we need to handle in our helm charts.
* Rely on SRO helm charts (aka recipes) to create needed resources in spokes.
* Use ACM for delivery and MCO for activation.
* Example includes building, delivery and activation as a systemd service of Intel's ICE driver.

## How?
### Minimum cluster setup
For this PoC we need a setup including at least:
* 1 hub.
* 2 spokes.

Please refer to ACM docs to see how to import clusters from the web console.

```bash
$ KUBECONFIG=~/hub/auth/kubeconfig oc get managedclusters 
NAME            HUB ACCEPTED   MANAGED CLUSTER URLS                                     JOINED   AVAILABLE   AGE
local-cluster   true           https://api.acm-hub.edge-sro.rhecoeng.com:6443           True     True        15d
spoke1          true           https://api.sro-upstream-ci.edge-sro.rhecoeng.com:6443   True     True        2d4h
spoke2          true           https://api.acm-spoke2.edge-sro.rhecoeng.com:6443        True     True        2d1h
```

If we have a connected cluster we can have any kind of variance in OCP versions in spokes, as these will be checked against `api.openshift.com` for the associated OS images. To extract this info:
```bash
$ KUBECONFIG=~/hub/auth/kubeconfig oc get managedclusters -o json | jq -r '.items[].metadata.labels.openshiftVersion'
4.9.15
4.9.17
4.9.15
```

### SRO
Special Resource Operator (SRO) is an OCP component intended for out of tree driver support in OCP clusters. 

SRO was changed to include a new CRD: `SpecialResourceModule`. This was done in order to not pollute the original source code, plus easier development when adding/removing features. For example, the NFD dependency removal was easy because of this fact. The downside is that we can only ignore NFD if we are not using any `SpecialResource` manifests. Fortunately, this is the case for the PoC, since we are not going to be deploying any `SpecialResource`s in the hub.

SRO used in the PoC is the modified one from `acm` branch. Since these modifications have been implemented as a PoC, there is no automatic image build nor OLM support.

To deploy the operator we need to perform this step in the hub:
```
make local-image-build local-image-push deploy
```

The new CRD `SpecialResourceModule` is almost exactly the same as a `SpecialResource` resource. The only added part is a `watch` section, where we specify which resources we care about to grab information about how many different OCP versions we need to handle.
```yaml
apiVersion: sro.openshift.io/v1beta1
kind: SpecialResourceModule
metadata:
  name: acm-ice
spec:
  namespace: acm-ice
  chart:
    name: acm-ice
    version: 0.0.1
    repository:
      name: chart
      url: file:///charts/example
  set:
    kind: Values
    apiVersion: sro.openshift.io/v1beta1
    buildArgs:
      - name: DRIVER_VER
        value: "1.6.4"
    registry: quay.io/pacevedo
  watch:
    # Select label openshiftVersion from ManagedCluster resource spoke1.
    - path: "$.metadata.labels.openshiftVersion"
      apiVersion: cluster.open-cluster-management.io/v1
      kind: ManagedCluster
      name: spoke1
    # Select label openshiftVersion from all entries defined in ManagedCluster.
    - path: "$.metadata.labels.openshiftVersion"
      apiVersion: cluster.open-cluster-management.io/v1
      kind: ManagedCluster
    # Select label openshiftVersion from any ManagedCluster resource that does not have
    # a label called example set to some-value.
    - path: "$.metadata.labels.openshiftVersion"
      apiVersion: cluster.open-cluster-management.io/v1
      kind: ManagedCluster
      selector:
        - path: "$.metadata.labels.example"
          value: some-value
          exclude: true
```

If we take a close look at the new section we see we can specify lists of resources. Each of these includes:
* `path`: A jsonpath pointing to a string or a list of strings where we can find OCP versions or base OS images. Mandatory.
* `apiVersion`: The `apiVersion` of the resource we want to watch. Mandatory.
* `kind`: The `kind` of the resource we want to watch. Mandatory.
* `name`: The `name` of the resource we want to watch. If not provided it will watch on all resources sharing `apiVersion` and `kind`, irrespective of names. Optional.
* `namespace`: The `namespace` of the resource we want to watch. Optional.
* `selector`: Filter which of the resources defined above will be selected to read its `path`. List format, entries are ORed.
  * `path`: Jsonpath pointing to a valid value in the resource. Mandatory.
  * `value`: The value that the `path` should hold. Mandatory.
  * `exclude`: Whether a match should be selected or excluded. Optional.

With this we are able to watch any resource in the cluster only needing list permissions in RBAC. This means there is no specific code/support for any of the resources we can watch in SRO.

The idea behind the watch resource is to get the kernel version + OCP version we need to set up when building the driver containers. This can be obtained in two different ways:
* Using only the OCP version following semver2: `<major>.<minor>.<z stream>`. With this, SRO will check on [OCP API](https://api.openshift.com/api/upgrades_info) to retrieve all the images associated to that version. Once there, pulling the last layer of the associated DTK will render the kernel version we should be using. This is the connected environment approach.
* Specify a base image for the nodes. This will be in an image registry format and must match the OS running in the nodes where we want to deploy our driver container. Since we are reading this data from the hub we need the base images because there is no other way of looking up which DTK a node is using. A possible way of doing it when using ACM is to use `ClusterClaims` in the spokes. An example can be seen in the next section. This is the disconnected environment approach.

Whatever approach we are following we always need to extract information in the same order:
* Get the OS image for an OCP version.
* Extract layers in the OS image and look for the driver toolkit image associated with this version.
* Grab kernel and RHEL version from this driver toolkit.
* Repeat for each OCP version we find in the watched resources.

### Helm charts
Helm charts are known as SRO recipes, and its what SRO uses to deploy manifests associated to a `SpecialResource` or `SpecialResourceModule`. In the case of this PoC, these manifests need to include:
* A `BuildConfig` to specify how to build a driver container.
* A `Policy` for ACM, containing a `MachineConfig`. This will drive which files are created in the spokes.
* A `PlacementRule` for ACM, containing matching rules for selecting which spokes get changes we need.
* A `PlacementBinding` for ACM, linking the `Policies` with the `PlacementRules`.
* Any other additional resource may be added, as for example a `ConfigMap` containing scripts to handle the loading and unloading of modules when not using kmods-via-containers.

The way the helm charts are written plays an important role in how SRO works when reconciling. The list of helm variables SRO is setting can be seen in SRO documentation.

## Example
### ACM-ICE recipe
We included the Intel ICE driver deployment as the example recipe to have something real that we can relate to. This is what the chart looks like:
```bash
$ tree charts/example/acm-ice-0.0.1
charts/example/acm-ice-0.0.1
├── acm-ice.yaml
├── Chart.yaml
├── README.md
└── templates
    ├── 0001-buildconfig.yaml
    ├── 0003-policy.yaml
    └── 0004-placement.yaml

1 directory, 6 files
```

The `SpecialResourceModule` defines what to watch for OCP versions, assuming we are in a connected environment:
```yaml
  watch:
    - path: "$.metadata.labels.openshiftVersion"
      apiVersion: cluster.open-cluster-management.io/v1
      kind: ManagedCluster
      name: spoke1
    - path: "$.metadata.labels.openshiftVersion"
      apiVersion: cluster.open-cluster-management.io/v1
      kind: ManagedCluster
      name: spoke2
```

The `BuildConfig` is using the OCP version in its name:
```yaml
apiVersion: build.openshift.io/v1
kind: BuildConfig
metadata:
  labels:
    app: {{.Values.specialResourceModule.metadata.name}}-{{.Values.clusterVersion}}
  name: {{.Values.specialResourceModule.metadata.name}}-{{.Values.clusterVersion}}
  annotations:
    specialresource.openshift.io/wait: "true"
```

This can be improved by using kernel version with some string replacement to not face forbidden characters/length issues. It would mean that different OCP versions sharing the same kernel would not be rebuilt.

The `Policy` encloses a `MachineConfig` template holding a systemd service:
```yaml
                   systemd:
                     units:
                     - contents: |
                         [Unit]
                         Description=out-of-tree driver loader
                         # Start after the network is up
                         Wants=network-online.target
                         After=network-online.target
                         # Also after docker.service (no effect on systems without docker)
                         After=docker.service
                         # Before kubelet.service (no effect on systems without kubernetes)
                         Before=kubelet.service
 
                         [Service]
                         Type=oneshot
                         RemainAfterExit=true
                         # Use bash to workaround https://github.com/coreos/rpm-ostree/issues/1936
                         ExecStart=/usr/bin/bash -c "while ! /usr/local/bin/{{.Values.specialResourceModule.metadata.name}} load {{.Values.registry}}/{{.Values.specialResourceModule.metadata.name}}-{{.Values.groupName.driverContainer}}; do sleep 10; done"
                         ExecStop=/usr/bin/bash -c "/usr/local/bin/{{.Values.specialResourceModule.metadata.name}} unload {{.Values.registry}}/{{.Values.specialResourceModule.metadata.name}}-{{.Values.groupName.driverContainer}}"
                         StandardOutput=journal+console
 
                         [Install]
                         WantedBy=default.target
                       enabled: true
                       name: "{{.Values.specialResourceModule.metadata.name}}.service"
```

And this is the launching script:
```yaml
#!/bin/bash
set -eu

ACTION=$1; shift
IMAGE=$1; shift
KERNEL=`uname -r`

podman pull --authfile /var/lib/kubelet/config.json ${IMAGE}:${KERNEL} 2>&1

load_kmods() {

    podman run -i --privileged -v /lib/modules/${KERNEL}/kernel/drivers/:/lib/modules/${KERNEL}/kernel/drivers/ ${IMAGE}:${KERNEL} load.sh
}
unload_kmods() {
    podman run -i --privileged -v /lib/modules/${KERNEL}/kernel/drivers/:/lib/modules/${KERNEL}/kernel/drivers/ ${IMAGE}:${KERNEL} unload.sh
}

case "${ACTION}" in
    load)
        load_kmods
    ;;
    unload)
        unload_kmods
    ;;
    *)
        echo "Unknown command. Exiting."
        echo "Usage:"
        echo ""
        echo "load        Load kernel module(s)"
        echo "unload      Unload kernel module(s)"
        exit 1
esac
```

The service is using `podman` to pull and run the image (`load.sh` and `unload.sh` come from a `ConfigMap` defined in the same file as the `BuildConfig`. They are convenience scripts to load/unload the module). This could be easily swapped with `crictl` if needed.

Then we can see the `PlacementBinding`, which ties the `Policy` to every node but the cluster:
```yaml
apiVersion: apps.open-cluster-management.io/v1
kind: PlacementRule
metadata:
 name: {{.Values.specialResourceModule.metadata.name}}-placement
spec:
  clusterConditions:
  - status: "True"
    type: ManagedClusterConditionAvailable
  clusterSelector:
    matchExpressions:
    - key: name
      operator: NotIn
      values:
      - local-cluster
```

This can be improved to use the same OCP version label to produce gradual rollouts that follow the upgrade pace in the spokes.

### Connected vs disconnected
Having connected and disconnected environments differs in the way SRO is able to retrieve the information about a concrete OCP version.

When running on connected environments, SRO is able to check base images online while sourcing on OCP versions from the `watch` section in the `SpecialResourceModule`. An example:
```yaml
watch:
  - path: "$.metadata.labels.openshiftVersion"
    apiVersion: cluster.open-cluster-management.io/v1
    kind: ManagedCluster
    name: spoke1
  - path: "$.metadata.labels.openshiftVersion"
    apiVersion: cluster.open-cluster-management.io/v1
    kind: ManagedCluster
    name: spoke2
```
Here we see the OCP versions are fetched from the ACM `ManagedCluster` resource as a label, which is automatically filled in by the ACM operators. This will later on turn into queries to `api.openshift.com` to retrieve the base images for those versions.

When running on disconnected environments we dont have `api.openshift.com` available. Given that we can not check the images associated to an OCP version from the hub, we need to either have the version available, meaning the hub should be at the highest available versions taken from the spokes. This is not a feasible requirement, as we would need to upgrade the hub prior to all spokes and take unnecessary risks while doing so. We rely instead on a feature from ACM called `ClusterClaim` resources. These are claims in the form of `name: value` the spokes can make and inform the hub, which ingests this data and incorporates it into the `status` for the `ManagedCluster` resource.

By defining a `ClusterClaim` in the spokes referencing the base image in use:
```yaml
$ KUBECONFIG=~/spoke2/auth/kubeconfig oc get clusterclaims
NAME                                            AGE
consoleurl.cluster.open-cluster-management.io   5d21h
controlplanetopology.openshift.io               5d21h
id.k8s.io                                       5d21h
id.openshift.io                                 5d21h
infrastructure.openshift.io                     5d21h
kubeversion.open-cluster-management.io          5d21h
name                                            5d21h
osimage.openshift.io                            17h
platform.open-cluster-management.io             5d21h
product.open-cluster-management.io              5d21h
region.open-cluster-management.io               5d21h
version.openshift.io                            5d21h

$ KUBECONFIG=~/spoke2/auth/kubeconfig oc get clusterclaims osimage.openshift.io -o yaml
apiVersion: cluster.open-cluster-management.io/v1alpha1
kind: ClusterClaim
metadata:
  creationTimestamp: "2022-02-07T15:47:38Z"
  generation: 2
  name: osimage.openshift.io
  resourceVersion: "430538"
  uid: 490ce31b-2025-474d-9cb5-025ff716d9d5
spec:
  value: quay.io/openshift-release-dev/ocp-release@sha256:bb1987fb718f81fb30bec4e0e1cd5772945269b77006576b02546cf84c77498e
```

We get the result in the hub for the `ManagedCluster` resource:
```yaml
$ KUBECONFIG=~/hub/auth/kubeconfig oc get managedclusters spoke2 -o yaml
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  annotations:
    open-cluster-management/created-via: other
  creationTimestamp: "2022-02-02T11:31:32Z"
  finalizers:
  - managedcluster-import-controller.open-cluster-management.io/cleanup
  - managedclusterinfo.finalizers.open-cluster-management.io
  - open-cluster-management.io/managedclusterrole
  - managedcluster-import-controller.open-cluster-management.io/manifestwork-cleanup
  - cluster.open-cluster-management.io/api-resource-cleanup
  - agent.open-cluster-management.io/klusterletaddonconfig-cleanup
  generation: 2
  labels:
    cloud: Amazon
    clusterID: d777499d-bf5a-4016-8b34-6343befc8f6e
    feature.open-cluster-management.io/addon-application-manager: available
    feature.open-cluster-management.io/addon-cert-policy-controller: available
    feature.open-cluster-management.io/addon-iam-policy-controller: available
    feature.open-cluster-management.io/addon-policy-controller: available
    feature.open-cluster-management.io/addon-search-collector: available
    feature.open-cluster-management.io/addon-work-manager: available
    name: spoke2
    openshiftVersion: 4.9.15
    vendor: OpenShift
  name: spoke2
  resourceVersion: "14893773"
  uid: 5fa34b27-58e4-46e6-a4a7-65162c9a394b
spec:
  ...
status:
  [...]
  clusterClaims:
  - name: id.k8s.io
    value: spoke2
  - name: kubeversion.open-cluster-management.io
    value: v1.22.3+e790d7f
  - name: platform.open-cluster-management.io
    value: AWS
  - name: product.open-cluster-management.io
    value: OpenShift
  - name: consoleurl.cluster.open-cluster-management.io
    value: https://console-openshift-console.apps.acm-spoke2.edge-sro.rhecoeng.com
  - name: controlplanetopology.openshift.io
    value: HighlyAvailable
  - name: id.openshift.io
    value: d777499d-bf5a-4016-8b34-6343befc8f6e
  - name: infrastructure.openshift.io
    value: '{"infraName":"acm-spoke2-xmkpw"}'
  - name: osimage.openshift.io
    value: quay.io/openshift-release-dev/ocp-release@sha256:bb1987fb718f81fb30bec4e0e1cd5772945269b77006576b02546cf84c77498e
  - name: region.open-cluster-management.io
    value: eu-central-1
  - name: version.openshift.io
    value: 4.9.15
```

So now we have to watch on those claims from `SpecialResourceModule`:
```yaml
watch:
  - path: $.status.clusterClaims[?(@.name == 'osimage.openshift.io')].value
    apiVersion: cluster.open-cluster-management.io/v1
    kind: ManagedCluster
    name: spoke1
  - path: $.status.clusterClaims[?(@.name == 'osimage.openshift.io')].value
    apiVersion: cluster.open-cluster-management.io/v1
    kind: ManagedCluster
    name: spoke2
```

### Checks
After setting up the cluster and installing both the modified SRO and the ACM-ICE template using the following commands:
```bash
make local-image-build local-image-push deploy
oc apply -f charts/examples/acm-ice-0.0.1/acm-ice.yaml
```

We need to wait until the `BuildConfig` pods have finished:
```bash
$ KUBECONFIG=~/hub/auth/kubeconfig oc get pod -n acm-ice
NAME                     READY   STATUS      RESTARTS   AGE
acm-ice-4.9.15-1-build   0/1     Completed   0          4d16h
acm-ice-4.9.17-1-build   0/1     Completed   0          4d16h
```

Once this is over, the ACM policies should have been created:
```bash
$ KUBECONFIG=~/hub/auth/kubeconfig oc get placementrules,placementbindings,policies -n acm-ice
NAME                                                              AGE     REPLICAS
placementrule.apps.open-cluster-management.io/acm-ice-placement   4d16h   

NAME                                                                 AGE
placementbinding.policy.open-cluster-management.io/acm-ice-binding   4d16h

NAME                                                         REMEDIATION ACTION   COMPLIANCE STATE   AGE
policy.policy.open-cluster-management.io/policy-acm-ice-mc   enforce                                 4d16h
```

So we can turn to the nodes to see if everything is ok. First thing to check is the presence of the `MachineConfig`:
```bash
$ KUBECONFIG=~/spoke1/auth/kubeconfig oc get machineconfig
NAME                                               GENERATEDBYCONTROLLER                      IGNITIONVERSION   AGE
00-master                                          23d93af42378eefe48c8457dd21a2e23f53b2a94   3.2.0             110d
00-worker                                          23d93af42378eefe48c8457dd21a2e23f53b2a94   3.2.0             110d
01-master-container-runtime                        23d93af42378eefe48c8457dd21a2e23f53b2a94   3.2.0             110d
01-master-kubelet                                  23d93af42378eefe48c8457dd21a2e23f53b2a94   3.2.0             110d
01-worker-container-runtime                        23d93af42378eefe48c8457dd21a2e23f53b2a94   3.2.0             110d
01-worker-kubelet                                  23d93af42378eefe48c8457dd21a2e23f53b2a94   3.2.0             110d
10-acm-ice                                                                                    3.2.0             4d17h
99-master-generated-registries                     23d93af42378eefe48c8457dd21a2e23f53b2a94   3.2.0             110d
99-master-ssh                                                                                 3.2.0             110d
99-worker-generated-registries                     23d93af42378eefe48c8457dd21a2e23f53b2a94   3.2.0             110d
99-worker-ssh                                                                                 3.2.0             110d
rendered-master-126e6213e2e39bb5ab10284ff5617e23   d2d236b1952843821602ec36cd5817e72fd0a407   3.2.0             82d
rendered-master-1d83ff1fb917180f603146b8be8401c1   23d93af42378eefe48c8457dd21a2e23f53b2a94   3.2.0             4d19h
rendered-master-4e44b068e923683e0a84da82d0a3eb97   1be39964b45df10cbd2b9e99d0fe4b268bf9a42f   3.2.0             4d23h
rendered-master-8fb5bbe42d4aff952e9b2041cc7953df   d2d236b1952843821602ec36cd5817e72fd0a407   3.2.0             82d
rendered-master-af5395901d47dc55c1445fd91d2bad9c   d2d236b1952843821602ec36cd5817e72fd0a407   3.2.0             110d
rendered-worker-1429e7eecfcb5cf463875473ab59b695   d2d236b1952843821602ec36cd5817e72fd0a407   3.2.0             82d
rendered-worker-1e6f63c70789b445b454673064bbd575   23d93af42378eefe48c8457dd21a2e23f53b2a94   3.2.0             4d17h
rendered-worker-4c23e17fe8a9060fe96bc3f68525a758   1be39964b45df10cbd2b9e99d0fe4b268bf9a42f   3.2.0             4d23h
rendered-worker-824ebf9a01cb95e8c10bb255a17e2f24   d2d236b1952843821602ec36cd5817e72fd0a407   3.2.0             110d
rendered-worker-b0f243ebe5dc5e9424b5a379c4613ecb   d2d236b1952843821602ec36cd5817e72fd0a407   3.2.0             82d
rendered-worker-cea83b421b3631e95996f85e267e4b2c   1be39964b45df10cbd2b9e99d0fe4b268bf9a42f   3.2.0             4d20h
rendered-worker-f88004c074e5e13d83bf3bc750dcb777   23d93af42378eefe48c8457dd21a2e23f53b2a94   3.2.0             4d18h
```

So we can now debug nodes to fulfill the rest of the checks:
```bash
$ KUBECONFIG=~/spoke1/auth/kubeconfig oc debug node/ip-10-0-129-79.ec2.internal -t -- bash
Starting pod/ip-10-0-129-79ec2internal-debug ...
To use host binaries, run `chroot /host`
Pod IP: 10.0.129.79
If you don't see a command prompt, try pressing enter.
[root@ip-10-0-129-79 /]# chroot /host
sh-4.4#  # Check files have been created
sh-4.4# ls /usr/local/bin/acm-ice 
/usr/local/bin/acm-ice
sh-4.4# ls /etc/systemd/system/acm-ice.service 
/etc/systemd/system/acm-ice.service
sh-4.4# # Check systemd service status
sh-4.4# systemctl status acm-ice
● acm-ice.service - out-of-tree driver loader
   Loaded: loaded (/etc/systemd/system/acm-ice.service; enabled; vendor preset: disabled)
   Active: active (exited) since Mon 2022-02-07 08:39:53 UTC; 4min 35s ago
  Process: 1417 ExecStart=/usr/bin/bash -c while ! /usr/local/bin/acm-ice load quay.io/pacevedo/acm-ice-driver-container; do sleep 10; done (code=exited, status=0/SUCCESS)
 Main PID: 1417 (code=exited, status=0/SUCCESS)
    Tasks: 0 (limit: 50354)
   Memory: 78.1M
      CPU: 1.175s
   CGroup: /system.slice/acm-ice.service

Feb 07 08:39:51 ip-10-0-129-79 bash[1417]: Copying blob sha256:e00839e47ff24d62051a6927fc050b8bed80cc174adea934ebc4ae15c89064a7
Feb 07 08:39:51 ip-10-0-129-79 bash[1417]: Copying blob sha256:673f651b3f25d937d5d1d6daa41582e1a61f8bf9bc817f5e3db46d16b25069fa
Feb 07 08:39:51 ip-10-0-129-79 bash[1417]: Copying config sha256:440a96972eea98227f3e76841fa50cf5e99384221b67614f63896f4a0f3a5d84
Feb 07 08:39:51 ip-10-0-129-79 bash[1417]: Writing manifest to image destination
Feb 07 08:39:51 ip-10-0-129-79 bash[1417]: Storing signatures
Feb 07 08:39:52 ip-10-0-129-79 bash[1417]: 440a96972eea98227f3e76841fa50cf5e99384221b67614f63896f4a0f3a5d84
Feb 07 08:39:52 ip-10-0-129-79 bash[1417]: rmmod: ERROR: Module ice is not currently loaded
Feb 07 08:39:53 ip-10-0-129-79 bash[1417]: Loaded out-of-tree ICE
Feb 07 08:39:53 ip-10-0-129-79 bash[1417]: ice                  1019904  0
Feb 07 08:39:53 ip-10-0-129-79 systemd[1]: Started out-of-tree driver loader.
sh-4.4# # Check the module has been loaded
sh-4.4# lsmod | grep ice
ice                  1019904  0
sh-4.4# # Check how the container was started to load the module
sh-4.4# podman ps -a
CONTAINER ID  IMAGE                                                                   COMMAND     CREATED        STATUS                    PORTS       NAMES
7a5272a43e7a  quay.io/pacevedo/acm-ice-driver-container:4.18.0-305.30.1.el8_4.x86_64  load.sh     9 minutes ago  Exited (0) 9 minutes ago              determined_borg
```

And the final check would be to see what the status in the CR looks like. There is an entry per OCP reconciled version, stating which resource is being reconciled at every moment. When all of them have been reconciled we should see a `complete` boolean value:
```bash
$ KUBECONFIG=~/hub/auth/kubeconfig oc get specialresourcemodule acm-ice -o json | jq -r '.status'
{
  "versions": {
    "4.9.15": {
      "complete": true
    },
    "4.9.17": {
      "complete": true
    }
  }
}
```