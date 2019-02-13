# Special Resource Operator

## Hard and Soft Partitioning
The operator has example CR's how to create a hard or soft partitioning scheme for the worker nodes where on has special resources. Hard partitioning is realized with taints and tolerations where soft partitioning is priority and preemption. 

### Hard Partitioning
If one wants to repel Pods from nodes that have special resources without the corresponding toleration, the following CR can be used to instantiate the operator with taints for the nodes: [sro_cr_sched_taints_tolerations.yaml](https://github.com/zvonkok/special-resource-operator/blob/5973d6fea1985c425f5c36733fbc8e693e2c3821/manifests/sro_cr_sched_taints_tolerations.yaml#L1).  

The `nvidia.com/gpu` is an extended resource, which is exposed by the DevicePlugin, there is no need to add a  toleration to Pods that request extended resources. The ExtendedResourcesAdmissionController will add a toleration to each Pod that tries to allocate an extended resource.  

A Pod that does not request a extended resource e.g. a CPU only Pod will be repelled from the node. The taint will make sure that only special resource workloads are deployed to those specific nodes.





