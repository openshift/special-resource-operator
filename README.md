# Special Resource Operator (SRO)


## Operation Breakdown
The special resource operator implements a simple state machine, where each state has a validation step. The validation step for each state is different and relies on the functionality to be tested. 

The following descriptions of the states will describe how e.g. the SRO handles GPUs in a cluster. 

### General State Breakdown
Assets like ServiceAccount, RBAC, DaemonSet, ConfigMap yaml files for each state are saved in the container under `/opt/sro/state-{driver,device-plugin,monitoring}`. The SRO will take each of these assest and assign a control function to each of them. Those control functions have hooks for preprocessing the yaml files or hooks to preprocess the decoded API runtime objects. Those hooks are used to add runtime information from the cluster like kernel-version, nodeselectors based on the discovered hardware etc. 

After the assests were decoded preprocessed and transformed into API runtime objects, the control funcs take care of CRUD operations on those. 

The SRO is easily extended just by creating another directory under `/opt/sro/state-new` and adding this new state to the operator [addState(...)](https://github.com/zvonkok/special-resource-operator/blob/012020bb04922737d1f9eb5e703d3b931a053bd4/pkg/controller/specialresource/specialresource_state.go#L79). 







## Hard and Soft Partitioning
The operator has example CR's how to create a hard or soft partitioning scheme for the worker nodes where on has special resources. Hard partitioning is realized with taints and tolerations where soft partitioning is priority and preemption. 

### Hard Partitioning
If one wants to repel Pods from nodes that have special resources without the corresponding toleration, the following CR can be used to instantiate the operator with taints for the nodes: [sro_cr_sched_taints_tolerations.yaml](https://github.com/zvonkok/special-resource-operator/blob/5973d6fea1985c425f5c36733fbc8e693e2c3821/manifests/sro_cr_sched_taints_tolerations.yaml#L1). The CR accepts an array of taints. 

The `nvidia.com/gpu` is an extended resource, which is exposed by the DevicePlugin, there is no need to add a  toleration to Pods that request extended resources. The ExtendedResourcesAdmissionController will add a toleration to each Pod that tries to allocate an extended resource on a node with the corresponding taint.

A Pod that does not request a extended resource e.g. a CPU only Pod will be repelled from the node. The taint will make sure that only special resource workloads are deployed to those specific nodes.

### Soft Partitioning
Compared to the hard partioning scheme with taints, soft partitioning will allow any Pod on the node but will preempt low priority Pods with high priority Pods. High priority Pods could be with special resource workloads and low priority CPU only Pods. The following CR can be used to instantiate the operator with priority and preemption: [sro_cr_sched_priority_preemption.yaml](https://github.com/zvonkok/special-resource-operator/blob/052d7ad0cd4255ab9b0595f17d4914b61927d18f/manifests/sro_cr_sched_priority_preemption.yaml#L1). The CR accepts an array of priorityclasses, here the operator creates two classes, namely: `gpu-high-priority` and `gpu-low-priority`. 

One can use the low priority class to keep the node busy and as soon as a high priority class Pod is created that allocates a extended resource, the scheduler will try to preempt the low priority Pod to make schedulig of the pending Pod possible. 







