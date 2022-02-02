
# Special Resource Operator

The Special Resource Operator (SRO) manages the  deployment of software stacks for hardware accelerators on an existing (day 2) OpenShift or Kubernetes cluster. SRO can be used for a case as simple as building and loading a single kernel module, or as complex as deploying the driver, device plugin, and monitoring stack for a hardware accelerator.

For loading kernel modules, SRO is designed around the use of "driver containers." Driver containers are increasingly being used in cloud-native environments, especially when run on pure container operating systems to deliver hardware drivers to the host. 

Driver containers  extend the kernel stack beyond the out-of-box software and hardware features of a specific kernel. Driver containers work on various container capable Linux distributions. With driver containers the host stays "clean" and there will not be any clash between different library versions or binaries on the host.


# Installation
Note: The Special Resource operator has a [dependency](#Node-Feature-Discovery-dependency) on the Node Feature Discovery (NFD) operator. If deploying on OpenShift from OperatorHub, NFD will be installed automatically. If deploying from the CLI, first install NFD.

## From OperatorHub
The Special Resource Operator is available as a community operator on OperatorHub, and as an official Red Hat operator (tech-preview) starting in OpenShift 4.9.

## From the CLI

Deploy to vanilla k8s:
```
$ git clone https://github.com/openshift-psap/special-resource-operator
$ cd special-resource-operator
$ make deploy TAG=master
```

Deploy to OCP:
```
$ git clone https://github.com/openshift-psap/special-resource-operator
$ cd special-resource-operator
$ make deploy TAG=master
```

To build and deploy using a custom operator image:
```
$ make local-image-build
$ make local-image-push
$ make deploy
```
Note: The image TAG will default to the name of the current git branch, but can be overriden by setting the TAG variable. See the `Makefile` for more details.

To deploy the simple-kmod example special resource on OpenShift 4.x:
```
$ oc apply -f charts/example/simple-kmod-0.0.1/simple-kmod.yaml
```


# Creating a special resource recipe

See [docs/recipes.md](docs/recipes.md) for instructions on how to create a recipe for SRO to manage. 

See `charts/example` for some examples. In particular:
* The simple-kmod example shows how to build and deploy two simple kernel modules in a driver container on OpenShift.
* The centos-simple-kmod example uses the same kernel module as simple-kmod, but is written for running on a vanilla kubernetes cluster with CentOS worker nodes.

# Node Feature Discovery dependency

There is a general problem when trying to configure a cluster with a special resource. One does not know which nodes have a special resource and which do not. To address this, SRO relies on the [NFD operator](https://github.com/openshift/cluster-nfd-operator). NFD will label the host with node specific attributes, like PCI cards, kernel or OS version and more. The .yaml template files in a special resource recipe can use these NFD labels in their nodeSelector fields to ensure that the software stack is run only on the nodes with the hardware feature. See [upstream NFD](https://github.com/kubernetes-sigs/node-feature-discovery) for more info. 

