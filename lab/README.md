# Special Resource Operator Lab
 

The lab assumes that there is an cluster already deployed and it contains the following exercises:


    * ** Exercise 1**
        * SRO will be deployed along with NFD dependency for SRO v1
        * Verify SRO and NFD are running
    * **Exercise 2**
        * Simple kmod example used as a deployment exercise
        * Verify simple kmod is deployed and running
    * **Exercise 3**
        * More advanced recipe will be used along with DTK for the following:
            * Use DTK to build a driver container image
            * Create a recipe for SRO which will deploy the newly created driver image from the local registry
            * Verify the daemonset is running on the needed nodes
    * **Exercise 4**
        * Most advanced recipe to include deploying a kernel module built using DTK and deploying a device plug-in
        * Create a recipe which deployed a kmod along with a kubernetes device plug-in
        * Verify both the daemonset and device plug-in are deployed and running in cluster
    * **Exercise 5**
        * Troubleshoot failure to load a kmod




## EXERCISE 1

** Deploy SRO with necessary dependency NFD and verify both are running

 
### Using command-line
We create a subscription object by adding this yaml to a new file, i.e  **special-resource-operator.yaml**:


```
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: openshift-special-resource-operator
  namespace: openshift-operators
spec:
  channel: "stable"
  installPlanApproval: Automatic
  name: openshift-special-resource-operator
  source: redhat-operators
  sourceNamespace: openshift-marketplace
```


Then we create the subscription in the cluster:


```
oc create -f special-resource-operator.yaml
```


After a while we should see Special Resource Operator controller running and also its dependency Node Feature Discovery :


```
[root@ebelarte lab]# oc get po
NAME                                                   READY   STATUS    RESTARTS   AGE
nfd-controller-manager-8c9585895-8xqmd                 2/2     Running   0          18m
special-resource-controller-manager-56b978fc6d-fhkgs   2/2     Running   0          19m
[root@ebelarte lab]#
```




 
### Using Openshift console
Login the console and go to Operators -> OperatorHub

![OperatorHub](https://github.com/enriquebelarte/sro-lab/blob/main/images/image3.png)


In the search box, type special:

![OperatorHub](https://github.com/enriquebelarte/sro-lab/blob/main/images/image2.png)


![OperatorHub](https://github.com/enriquebelarte/sro-lab/blob/main/images/image4.png)



Choose Special Resource Operator provided by Red Hat and click the Install button:


![OperatorHub](https://github.com/enriquebelarte/sro-lab/blob/main/images/image5.png)


![OperatorHub](https://github.com/enriquebelarte/sro-lab/blob/main/images/image6.png)


![OperatorHub](https://github.com/enriquebelarte/sro-lab/blob/main/images/image7.png)





## EXERCISE 2


**Deploy simple-kmod example

First, we will create a folder to save the charts we are making:


```
mkdir -p chart/simple-kmod-0.0.1/templates
```


In this example we are going to use simple-kmod which is a light “Hello World” kernel module for testing purposes.

Then create two yaml files inside templates folder:

**0000-buildconfig.yaml**


```
apiVersion: image.openshift.io/v1
kind: ImageStream
metadata:
  labels:
    app: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
spec: {}
---
apiVersion: build.openshift.io/v1
kind: BuildConfig
metadata:
  labels:
    app: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverBuild}}
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverBuild}}
  annotations:
    specialresource.openshift.io/wait: "true"
    specialresource.openshift.io/driver-container-vendor: simple-kmod
    specialresource.openshift.io/kernel-affine: "true"
spec:
  nodeSelector:
    node-role.kubernetes.io/worker: ""
  runPolicy: "Serial"
  triggers:
    - type: "ConfigChange"
    - type: "ImageChange"
  source:
    dockerfile: |
      FROM {{ .Values.driverToolkitImage  }} as builder
      WORKDIR /build/
      RUN git clone -b {{.Values.specialresource.spec.driverContainer.source.git.ref}} {{.Values.specialresource.spec.driverContainer.source.git.uri}} 
      WORKDIR /build/simple-kmod
      RUN make all install KVER={{ .Values.kernelFullVersion }}

      FROM registry.redhat.io/ubi8/ubi-minimal

      RUN microdnf -y install kmod

      COPY --from=builder /etc/driver-toolkit-release.json /etc/
      COPY --from=builder /lib/modules/{{ .Values.kernelFullVersion }}/* /lib/modules/{{ .Values.kernelFullVersion }}/

  strategy:
    dockerStrategy:
      buildArgs:
        - name: "IMAGE"
          value: {{ .Values.driverToolkitImage  }}
        {{- range $arg := .Values.buildArgs }}
        - name: {{ $arg.name }}
          value: {{ $arg.value }}
        {{- end }}
        - name: KVER
          value: {{ .Values.kernelFullVersion }}
  output:
    to:
      kind: ImageStreamTag
      name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}:v{{.Values.kernelFullVersion}}
```


**1000-driver-container.yaml**


```
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
rules:
- apiGroups:
  - security.openshift.io
  resources:
  - securitycontextconstraints
  verbs:
  - use
  resourceNames:
  - privileged
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
subjects:
- kind: ServiceAccount
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
  namespace: {{.Values.specialresource.spec.namespace}}
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  labels:
    app: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
  annotations:
    specialresource.openshift.io/wait: "true"
    specialresource.openshift.io/state: "driver-container"
    specialresource.openshift.io/driver-container-vendor: simple-kmod
    specialresource.openshift.io/kernel-affine: "true"
spec:
  updateStrategy:
    type: OnDelete
  selector:
    matchLabels:
      app: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
  template:
    metadata:
      # Mark this pod as a critical add-on; when enabled, the critical add-on scheduler
      # reserves resources for critical add-on pods so that they can be rescheduled after
      # a failure.  This annotation works in tandem with the toleration below.
      annotations:
        scheduler.alpha.kubernetes.io/critical-pod: ""
      labels:
        app: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
    spec:
      serviceAccount: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
      serviceAccountName: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
      containers:
      - image: image-registry.openshift-image-registry.svc:5000/{{.Values.specialresource.spec.namespace}}/{{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}:v{{.Values.kernelFullVersion}}
        name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
        imagePullPolicy: Always
        command: [sleep, infinity]
        lifecycle:
          postStart:
            exec:
              command: ["modprobe", "-v", "-a" , "simple-kmod", "simple-procfs-kmod"]
          preStop:
            exec:
              command: ["modprobe", "-r", "-a" , "simple-kmod", "simple-procfs-kmod"]
        securityContext:
          privileged: true
      nodeSelector:
        node-role.kubernetes.io/worker: ""
        feature.node.kubernetes.io/kernel-version.full: "{{.Values.KernelFullVersion}}"
```


First file contains an ImageStream ( Registry Operator state should be set to Managed in the cluster ) and a BuildConfig to build the image.

Second file defines a ServiceAccount, a Role, a RoleBinding and a DaemonSet which will run the driver container with those specific RBAC settings.

Then at the _simple-kmod-0.0.1_ folder create a file with the definition of the chart.

**Chart.yaml**


```
apiVersion: v2
name: simple-kmod
description: Simple kmod will deploy a simple kmod driver-container
icon: https://avatars.githubusercontent.com/u/55542927
type: application
version: 0.0.1
appVersion: 1.0.0
```


Now, from the _chart_ directory use helm to package the chart:


```
[root@ebelarte chart]# helm package simple-kmod-0.0.1/
Successfully packaged chart and saved it to: /opt/lab/chart/simple-kmod-0.0.1.tgz
[root@ebelarte chart]# 
```


After this create a folder to store the _configmap_ files and copy the above chart to it:


```
[root@ebelarte chart]# mkdir cm && cp simple-kmod-0.0.1.tgz cm/

```


Create index file specifying the helm repo:


```
[root@ebelarte chart]# helm repo index cm --url=cm://simple-kmod/simple-kmod-chart
[root@ebelarte chart]#
```


Create a namespace:


```
[root@ebelarte chart]# oc create ns simple-kmod
namespace/simple-kmod created
[root@ebelarte chart]#
```


Create the ConfigMap:


```
[root@ebelarte chart]# oc create cm simple-kmod-chart --from-file=cm/index.yaml --from-file=cm/simple-kmod-0.0.1.tgz -n simple-kmod
configmap/simple-kmod-chart created
[root@ebelarte chart]# 
```


A user can just leverage the ./scripts/make-cm-recipe in order to do it for him. \
See [https://github.com/openshift/special-resource-operator/blob/e01ece2dc8958d7c7174985d5d85b0ef5ccb07c4/Makefile.helper.mk#L41](https://github.com/openshift/special-resource-operator/blob/e01ece2dc8958d7c7174985d5d85b0ef5ccb07c4/Makefile.helper.mk#L41) 

Create the SpecialResource** simple-kmod-sr.yaml**:


```
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
      name: simple-kmod
      url: cm://simple-kmod/simple-kmod-chart
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
        uri: "https://github.com/openshift-psap/simple-kmod.git"
```


Finally we should create the above SR in the cluster:


```
[root@ebelarte chart]# oc create -f simple-kmod-sr.yaml 
specialresource.sro.openshift.io/simple-kmod created

```


After a few minutes we could check if pods are running in simple-kmod project. As we can see below the build was done and  the driver container is running:


```
[root@ebelarte chart]# oc get po
NAME                                                  READY   STATUS      RESTARTS   AGE
simple-kmod-driver-build-e383247e62b56585-1-build     0/1     Completed   0          9m1s
simple-kmod-driver-container-e383247e62b56585-jkh65   1/1     Running     0          9m56s
[root@ebelarte chart]# 
```


In order to make sure that the kernel modules are effectively loaded we could run a _lsmod_ command:


```
[root@ebelarte chart]# oc exec -it simple-kmod-driver-container-e383247e62b56585-jkh65 -- lsmod | grep simple
simple_procfs_kmod     16384  0
simple_kmod            16384  0
[root@ebelarte chart]# 
```




## EXERCISE 3


**Use DTK to build a driver container image

Driver Toolkit a.k.a. DTK is a container image which can be used as a base image to build out-of-tree driver containers as it has all the required dependencies to do so. The only previous requirement is knowing in advance which DTK image is needed to use. This image will depend on your Openshift Cluster version and architecture type. We could find out version and use data accordingly i.e for an x86 architecture image:


```
[root@ebelarte driver-toolkit-tests]# OCV=$(oc version | grep "Server" | awk {'print $3'})
[root@ebelarte driver-toolkit-tests]# oc adm release info quay.io/openshift-release-dev/ocp-release:${OCV}-x86_64 --image-for=driver-toolkit
quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:54bd5d99cb2e4332c63f194a2f0d10f0535adf53e4562a3fdb408c67b1599d27
```




In this example we are going to use the Intel Ethernet ICE driver.

Now that we know that the image we should use is [quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256](mailto:quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256):54bd5d99cb2e4332c63f194a2f0d10f0535adf53e4562a3fdb408c67b1599d27 we create a new Namespace, ImageStream and BuildConfig which will output the resulting image to our local openshift registry:

**0000-buildconfig-ice.yaml**


```
---
apiVersion: v1
kind: Namespace
metadata:
  name: ice-kmod
---
apiVersion: image.openshift.io/v1
kind: ImageStream
metadata:
  labels:
    app: ice-kmod-driver-container
  name: ice-kmod-driver-container
  namespace: ice-kmod
spec: {}
---
apiVersion: build.openshift.io/v1
kind: BuildConfig
metadata:
  labels:
    app: ice-kmod-driver-build
  name: ice-kmod-driver-build
  namespace: ice-kmod
spec:
  nodeSelector:
    node-role.kubernetes.io/worker: ""
  runPolicy: "Serial"
  triggers:
    - type: "ConfigChange"
    - type: "ImageChange"
  source:
    dockerfile: |
      
      FROM quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:54bd5d99cb2e4332c63f194a2f0d10f0535adf53e4562a3fdb408c67b1599d27 as builder # DTK IMAGE 

      WORKDIR /build/

      RUN curl -L https://sourceforge.net/projects/e1000/files/ice%20stable/1.8.9/ice-1.8.9.tar.gz/download > ice-1.8.9.tar.gz

      RUN tar xfvz ice-1.8.9.tar.gz

      WORKDIR /build/ice-1.8.9/src

      RUN make install 

      FROM registry.redhat.io/ubi8/ubi-minimal

      COPY —from=builder /usr/bin/kmod /usr/bin/

      COPY --from=builder /etc/driver-toolkit-release.json /etc/ 
      COPY --from=builder /usr/lib/modules/4.18.0-305.40.2.el8_4.x86_64/ /usr/lib/modules/4.18.0-305.40.2.el8_4.x86_64/
  strategy:
    dockerStrategy:
      buildArgs:
        - name: KMODVER
          value: 1.8.9

  output:
    to:
      kind: ImageStreamTag
      name: ice-kmod-driver-container:1.8.9
```


`oc create -f 0000-buildconfig-ice.yaml` will build the driver and create the driver container image which will be available at the local registry:


```
[root@ebelarte lab]# oc get is
NAME                        IMAGE REPOSITORY                                                                                                        TAGS    UPDATED
ice-kmod-driver-container   default-route-openshift-image-registry.apps.test-infra-cluster-6feca3c4.redhat.com/ice-kmod/ice-kmod-driver-container   1.8.9   10 minutes ago
[root@ebelarte lab]# 
```




    3. 
Create a recipe for SRO which will deploy the newly created driver image from the local registry
To use this driver image with Special Resource Operator, we can make a recipe for it consisting in a chart and a template which we will package and will include in a new ConfigMap object that will be used by the Special Resource.

Using the same folder _chart_ as in previous example, create Chart.yaml:


```
[root@ebelarte chart]# mkdir -p ice-kmod-1.8.9/templates
[root@ebelarte chart]# cd ice-kmod-1.8.9
[root@ebelarte ice-kmod-1.8.9]# vim Chart.yaml
```


**Chart.yaml**


```
apiVersion: v2
name: ice-kmod
description: Intel ice driver deploy in a driver-container
icon: https://avatars.githubusercontent.com/u/55542927
type: application
version: 1.8.9
appVersion: 1.0.0
```


Next create the template inside templates folder.

**1000-driver-container.yaml**


```
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
rules:
- apiGroups:
  - security.openshift.io
  resources:
  - securitycontextconstraints
  verbs:
  - use
  resourceNames:
  - privileged
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
subjects:
- kind: ServiceAccount
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
  namespace: {{.Values.specialresource.spec.namespace}}
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  labels:
    app: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
  annotations:
    specialresource.openshift.io/wait: "true"
    specialresource.openshift.io/state: "driver-container"
    specialresource.openshift.io/driver-container-vendor: ice-kmod
    specialresource.openshift.io/kernel-affine: "true"
spec:
  updateStrategy:
    type: OnDelete
  selector:
    matchLabels:
      app: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
  template:
    metadata:
      annotations:
        scheduler.alpha.kubernetes.io/critical-pod: ""
      labels:
        app: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
    spec:
      serviceAccount: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
      serviceAccountName: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
      containers:
      - image: image-registry.openshift-image-registry.svc:5000/{{.Values.specialresource.spec.namespace}}/{{.Values.specialresource.spec.set.driverContainerImage}}:{{.Values.specialresource.spec.set.driverContainerVersion}}
        name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
        imagePullPolicy: Always
        command: [sleep, infinity]
        lifecycle:
          postStart:
            exec:
              command: ["modprobe", "-v", "-a" , "ice"]
          preStop:
            exec:
              command: ["modprobe", "-r", "-a" , "ice"]
        securityContext:
          privileged: true
      nodeSelector:
        node-role.kubernetes.io/worker: ""
        feature.node.kubernetes.io/kernel-version.full: "{{.Values.KernelFullVersion}}"
```


Then let’s create the Configmap that will be used later in the Special Resource. In the _chart_ root folder:


```
[root@ebelarte chart]# helm package ice-kmod-1.8.9/
Successfully packaged chart and saved it to: /opt/lab/chart/ice-kmod-1.8.9.tgz
[root@ebelarte chart]# cp ice-kmod-1.8.9.tgz cm/
[root@ebelarte chart]# helm repo index cm --url=cm://ice-kmod/ice-kmod-chart
[root@ebelarte chart]#  oc create cm ice-kmod-chart --from-file=cm/index.yaml --from-file=cm/ice-kmod-1.8.9.tgz -n ice-kmod
```


Create the Special Resource definition:

**ice-kmod-sr.yaml**


```
apiVersion: sro.openshift.io/v1beta1
kind: SpecialResource
metadata:
  name: ice-kmod
spec:
  namespace: ice-kmod
  chart:
    name: ice-kmod
    version: 1.8.9
    repository:
      name: ice-kmod
      url: cm://ice-kmod/ice-kmod-chart
  set:
    kind: Values
    apiVersion: sro.openshift.io/v1beta1
    kmodNames: ["ice"]
    driverContainerImage: ice-kmod-driver-container
    driverContainerVersion: 1.8.9
    buildArgs:
    - name: "KMODVER"
      value: "1.8.9"
```


Then create the Special Resource:


```
[root@ebelarte chart]# oc create -f ice-kmod-sr.yaml 
specialresource.sro.openshift.io/ice-kmod created
[root@ebelarte chart]# 
```


And finally make sure that driver container is running:


```
[root@ebelarte chart]# oc get po 
NAME                                               READY   STATUS      RESTARTS   AGE
ice-kmod-driver-build-1-build                      0/1     Completed   0          53m
ice-kmod-driver-container-e383247e62b56585-pphbs   1/1     Running     0          45m
```




## EXERCISE 4


 Make a recipe to include deploying a kernel module built using DTK and deploying a device plug-in
In this exercise we will create a dummy device plug-in which will simulate the detection of a specific device type and will provide a way to use a previous build kernel module and load it on the nodes in which our dummy device is detected. 

In production environments these device plug-ins could be used by a GPU or other devices. Related software to initialize and/or setup that kind of hardware to be used by containers it’s usually provided by vendors.

Our dummy device plug-in is based on [https://github.com/redhat-nfvpe/k8s-dummy-device-plugin](https://github.com/redhat-nfvpe/k8s-dummy-device-plugin) but updating some minor things to make it work on Openshift 4.10 and this is what it does:

“It works as a kind of echo device. One specifies the (albeit pretend) devices in a JSON file, and the plugin operates on those, and allocates the devices to containers that request them -- it does this by setting those devices into environment variables in those containers.”

For this recipe we’ll use a prebuilt Docker image of the [dummy device-plug-in](http://quay.io/ebelarte/oc-dummy-device-plugin:0.1). which will pretend to set a state for 4 different devices based on this JSON:


```
[
    {
        "name": "dev_1",
        "state": "Up"
    },
    {
        "name": "dev_2",
        "state": "Up"
    },
    {
        "name": "dev_3",
        "state": "Up"
    },
    {
        "name": "dev_4",
        "state": "Up"
    }
]
```


To begin with this recipe we will use the same simple-kmod driver that we know from previous examples, so in the _chart_ folder create a new one:


```
mkdir -p dp-simple-kmod-0.0.1/templates
```


Then create a new **Chart.yaml**:


```
apiVersion: v2
name: dp-simple-kmod
description: DP Simple kmod will deploy a simple kmod driver-container using DTK and deploy a device plug-in
icon: https://avatars.githubusercontent.com/u/55542927
type: application
version: 0.0.1
appVersion: 1.0
```


And inside _templates_ folder create three yaml files.

**0000-buildconfig.yaml**


```
apiVersion: image.openshift.io/v1
kind: ImageStream
metadata:
  labels:
    app: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
spec: {}
---
apiVersion: build.openshift.io/v1
kind: BuildConfig
metadata:
  labels:
    app: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverBuild}}
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverBuild}}
  annotations:
    specialresource.openshift.io/wait: "true"
    specialresource.openshift.io/driver-container-vendor: dp-simple-kmod
    specialresource.openshift.io/kernel-affine: "true"
spec:
  nodeSelector:
    node-role.kubernetes.io/worker: ""
  runPolicy: "Serial"
  triggers:
    - type: "ConfigChange"
    - type: "ImageChange"
  source:
    dockerfile: |
      FROM {{ .Values.driverToolkitImage  }} as builder
      WORKDIR /build/
      RUN git clone -b {{.Values.specialresource.spec.driverContainer.source.git.ref}} {{.Values.specialresource.spec.driverContainer.source.git.uri}} 
      WORKDIR /build/simple-kmod
      RUN make all install KVER={{ .Values.kernelFullVersion }}

      FROM registry.redhat.io/ubi8/ubi-minimal

      COPY —from=builder /usr/bin/kmod /usr/bin/


      COPY --from=builder /etc/driver-toolkit-release.json /etc/
      COPY --from=builder /lib/modules/{{ .Values.kernelFullVersion }}/* /lib/modules/{{ .Values.kernelFullVersion }}/

  strategy:
    dockerStrategy:
      buildArgs:
        - name: "IMAGE"
          value: {{ .Values.driverToolkitImage  }}
        {{- range $arg := .Values.buildArgs }}
        - name: {{ $arg.name }}
          value: {{ $arg.value }}
        {{- end }}
        - name: KVER
          value: {{ .Values.kernelFullVersion }}
  output:
    to:
      kind: ImageStreamTag
      name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}:v{{.Values.kernelFullVersion}}
```


**0500-device-plugin.yaml**


```
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
rules:
- apiGroups:
  - security.openshift.io
  resources:
  - securitycontextconstraints
  verbs:
  - use
  resourceNames:
  - privileged
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
subjects:
- kind: ServiceAccount
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
  namespace: {{.Values.specialresource.spec.namespace}}
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  labels:
    app: {{.Values.specialresource.metadata.name}}-deviceplugin
  name: {{.Values.specialresource.metadata.name}}-deviceplugin
  annotations:
    specialresource.openshift.io/wait: "true"
    specialresource.openshift.io/state: "device-plugin"
    specialresource.openshift.io/driver-container-vendor: dp-simple-kmod
    specialresource.openshift.io/kernel-affine: "true"
spec:
  updateStrategy:
    type: OnDelete
  selector:
    matchLabels:
      app: {{.Values.specialresource.metadata.name}}-deviceplugin
  template:
    metadata:
      # Mark this pod as a critical add-on; when enabled, the critical add-on scheduler
      # reserves resources for critical add-on pods so that they can be rescheduled after
      # a failure.  This annotation works in tandem with the toleration below.
      annotations:
        scheduler.alpha.kubernetes.io/critical-pod: ""
      labels:
        app: {{.Values.specialresource.metadata.name}}-deviceplugin
    spec:
      serviceAccount: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
      serviceAccountName: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
      hostNetwork: true
      containers:
      - name: dummy-device-plugin
        image: quay.io/ebelarte/oc-dummy-device-plugin:0.1
        #args: ["-log-level", "debug"]
        securityContext:
          privileged: true
        volumeMounts:
          - name: device-plugin
            mountPath: /var/lib/kubelet/device-plugins
      volumes:
        - name: device-plugin
          hostPath:
            path: /var/lib/kubelet/device-plugins
      nodeSelector:
        node-role.kubernetes.io/worker: ""
        feature.node.kubernetes.io/kernel-version.full: "{{.Values.KernelFullVersion}}"
```


**1000-drivercontainer.yaml**


```
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  labels:
    app: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
  name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
  annotations:
    specialresource.openshift.io/wait: "true"
    specialresource.openshift.io/state: "driver-container"
    specialresource.openshift.io/driver-container-vendor: dp-simple-kmod
    specialresource.openshift.io/kernel-affine: "true"
spec:
  updateStrategy:
    type: OnDelete
  selector:
    matchLabels:
      app: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
  template:
    metadata:
      # Mark this pod as a critical add-on; when enabled, the critical add-on scheduler
      # reserves resources for critical add-on pods so that they can be rescheduled after
      # a failure.  This annotation works in tandem with the toleration below.
      annotations:
        scheduler.alpha.kubernetes.io/critical-pod: ""
      labels:
        app: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
    spec:
      serviceAccount: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
      serviceAccountName: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
      containers:
      - image: image-registry.openshift-image-registry.svc:5000/{{.Values.specialresource.spec.namespace}}/{{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}:v{{.Values.kernelFullVersion}}
        resources:
          limits:
            dummy/dummyDev: 1  
        name: {{.Values.specialresource.metadata.name}}-{{.Values.groupName.driverContainer}}
        imagePullPolicy: Always
        command: [sleep, infinity]
        lifecycle:
          postStart:
            exec:
              command: ["modprobe", "-v", "-a" , "simple-kmod", "simple-procfs-kmod"]
          preStop:
            exec:
              command: ["modprobe", "-r", "-a" , "simple-kmod", "simple-procfs-kmod"]
        securityContext:
          privileged: true
      nodeSelector:
        node-role.kubernetes.io/worker: ""
        feature.node.kubernetes.io/kernel-version.full: "{{.Values.KernelFullVersion}}"
```


What we basically are creating in these 3 templates is:

1) A **BuildConfig **object which will use Driver Toolkit Container (DTK) to retreive a driver from a git source and then compile it and build a driver-container image which will be pushed to the Openshift local registry in the cluster.

2) A **Daemonset** object which will deploy a pod with the device plug-in image.

3) A **Daemonset** object which will run the driver container itself, but this time we will use resources.limits to set the device plug-in settings.

Once created these files go back to the root _chart_ folder and let’s create the **SpecialResource** object to deploy all the 3 previous templates.

**dp-simple-kmod-sr.yaml**


```
apiVersion: sro.openshift.io/v1beta1
kind: SpecialResource
metadata:
  name: dp-simple-kmod
spec:
  namespace: dp-simple-kmod
  chart:
    name: dp-simple-kmod
    version: 0.0.1
    repository:
      name: dp-simple-kmod
      url: cm://dp-simple-kmod/dp-simple-kmod-chart
  set:
    kind: Values
    apiVersion: sro.openshift.io/v1beta1
    kmodNames: ["simple-kmod", "simple-procfs-kmod"]
    buildArgs:
    - name: "KMODVER"
      value: "SROP3"
 
```


Finally let’s create the **Configmap** for our chart as we did in all other exercises:


```
oc project dp-simple-kmod 
helm package dp-simple-kmod-0.0.1/ 
cp dp-simple-kmod-0.0.1.tgz cm/ 
helm repo index cm --url=cm://dp-simple-kmod/dp-simple-kmod-chart 
oc create cm dp-simple-kmod-chart --from-file=cm/index.yaml --from-file=cm/dp-simple-kmod-0.0.1.tgz -n dp-simple-kmod
```


And deploy the new **SpecialResource** object:


```
oc create -f dp-simple-kmod-sr.yaml
```



### Verify both the daemonset and device plug-in are deployed and running in cluster

After a while we can check that the driver was built in-cluster by the **BuildConfig**, and the device-plugin and driver-container pods are running:


```
[root@ebelarte chart]# oc get po
NAME                                                     READY   STATUS      RESTARTS   AGE
dp-simple-kmod-deviceplugin-e383247e62b56585-qn2fb       1/1     Running     0          48m
dp-simple-kmod-driver-build-e383247e62b56585-1-build     0/1     Completed   0          50m
dp-simple-kmod-driver-container-e383247e62b56585-wrktm   1/1     Running     0          48m
```


And in our very specific example we could check if our “dummy” devices are present in the driver-container:


```
[root@ebelarte chart]# oc exec -it dp-simple-kmod-driver-container-e383247e62b56585-wrktm -- printenv | grep DUMMY
DUMMY_DEVICES=dev_1
```




## EXERCISE 5


Troubleshoot failure to load a kmod
There are some different places we could look for logs and traces regarding the load or build of our modules with SRO.

Different examples:

- Driver container not being created. No pod running. If we inspect the cluster events we could see that our serviceaccount is not allowed to use some of the settings in the deployment.


```
oc get events
90m         Warning   FailedCreate       daemonset/dp-simple-kmod-driver-container-e383247e62b56585   Error creating: pods "dp-simple-kmod-driver-container-e383247e62b56585-" is forbidden: unable to validate against any security context constraint: [provider "anyuid": Forbidden: not usable by user or serviceaccount, provider restricted: .spec.securityContext.hostNetwork: Invalid value: true: Host network is not allowed to be used, spec.volumes[0]: Invalid value: "hostPath": hostPath volumes are not allowed to be used, spec.containers[0].securityContext.privileged: Invalid value: true: Privileged containers are not allowed, spec.containers[0].securityContext.hostNetwork: Invalid value: true: Host network is not allowed to be used, provider "nonroot": Forbidden: not usable by user or serviceaccount, provider "ootmodprobe": Forbidden: not usable by user or serviceaccount, provider "hostmount-anyuid": Forbidden: not usable by user or serviceaccount, provider "machine-api-termination-handler": Forbidden: not usable by user or serviceaccount, provider "hostnetwork": Forbidden: not usable by user or serviceaccount, provider "hostaccess": Forbidden: not usable by user or serviceaccount, provider "node-exporter": Forbidden: not usable by user or serviceaccount, provider "privileged": Forbidden: not usable by user or serviceaccount]
```




* BuildConfig not running. We could inspect the logs in the special-resource-controller pod (manager container) and look for possible issues:

    ```
2022-06-20T16:04:40.461Z	INFO	dp-simple-kmod  	RECONCILE REQUEUE: Could not reconcile chart	{"error": "cannot reconcile hardware states: failed to create state templates/0000-buildconfig.yaml: after CRUD hooks failed: could not wait for resource: Waiting too long for resource: timed out waiting for the condition "}

…
2022-06-20T16:06:55.286Z	INFO	warning  	OnError: node Conflict Label specialresource.openshift.io/state-dp-simple-kmod-0000 err %!s(<nil>)

…


```


* Driver-container pod is created but STATUS is different from Running:

    ```
NAME                                                  READY   STATUS             RESTARTS   AGE
simple-kmod-driver-container-e383247e62b56585-2gx7r   0/1     ImagePullBackOff   0          16s

```



Registry is not accesible or BuildConfig did not end successfully. We could inspect logs of the pod or describe it. In this example if we describe the pod we could easily confirm that 

image is not in our local registry:


```
  Warning  Failed          4m25s (x4 over 5m52s)  kubelet            Failed to pull image "image-registry.openshift-image-registry.svc:5000/simple-kmod/simple-kmod-driver-container:v4.18.0-305.40.2.el8_4.x86_64": rpc error: code = Unknown desc = reading manifest v4.18.0-305.40.2.el8_4.x86_64 in image-registry.openshift-image-registry.svc:5000/simple-kmod/simple-kmod-driver-container: manifest unknown: manifest unknown


```


Most probably cause could be BuildConfig did not push the image correctly so we can go and look for possible issues at the **simple-kmod-driver-build** pod:


```
oc logs -f simple-kmod-driver-build-e383247e62b56585-1-build

…
Pulling image registry.redhat.io/ubi8/ubi-minimal2 ...
Trying to pull registry.redhat.io/ubi8/ubi-minimal2:latest...
time="2022-06-22T09:01:35Z" level=warning msg="failed, retrying in 1s ... (1/3). Error: initializing source docker://registry.redhat.io/ubi8/ubi-minimal2:latest: reading manifest latest in registry.redhat.io/ubi8/ubi-minimal2: unknown: Not Found"
…
```


And we can confirm that there’s a typo in the image url used for the template to build the driver-container.
