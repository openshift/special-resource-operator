package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// operator-sdk generate k8s         -> API DeepCopy generation
// operator-sdk generate openapi     -> CRD update with openapi

// SpecialResourceImages defines the observed state of SpecialResource
// +k8s:openapi-gen=true
type SpecialResourceImages struct {
	Name       string                 `json:"name"`
	Kind       string                 `json:"kind"`
	Namespace  string                 `json:"namespace"`
	PullSecret string                 `json:"pullsecret"`
	Paths      []SpecialResourcePaths `json:"path"`
}

// SpecialResourceClaims defines the observed state of SpecialResource
// +k8s:openapi-gen=true
type SpecialResourceClaims struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
}

// SpecialResourcePaths defines the observed state of SpecialResource
// +k8s:openapi-gen=true
type SpecialResourcePaths struct {
	SourcePath     string `json:"sourcePath"`
	DestinationDir string `json:"destinationDir"`
}

// SpecialResourceArtifacts defines the observed state of SpecialResource
// +k8s:openapi-gen=true
type SpecialResourceArtifacts struct {
	HostPaths []SpecialResourcePaths  `json:"hostPaths,omitempty"`
	Images    []SpecialResourceImages `json:"images,omitempty"`
	Claims    []SpecialResourceClaims `json:"claims,omitempty"`
}

// SpecialResourceNode defines the observed state of SpecialResource
// +k8s:openapi-gen=true
type SpecialResourceNode struct {
	Selector string `json:"selector"`
}

// SpecialResourceRunArgs defines the observed state of SpecialResource
// +k8s:openapi-gen=true
type SpecialResourceRunArgs struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// SpecialResourceBuilArgs defines the observed state of SpecialResource
// +k8s:openapi-gen=true
type SpecialResourceBuilArgs struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// SpecialResourceGit defines the observed state of SpecialResource
// +k8s:openapi-gen=true
type SpecialResourceGit struct {
	Ref string `json:"ref"`
	Uri string `json:"uri"`
}

// SpecialResourceSource defines the observed state of SpecialResource
// +k8s:openapi-gen=true
type SpecialResourceSource struct {
	Git SpecialResourceGit `json:"git,omitempty"`
}

// SpecialResourceDriverContainer defines the desired state of SpecialResource
// +k8s:openapi-gen=true
type SpecialResourceDriverContainer struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	Source    SpecialResourceSource     `json:"source,omitempty"`
	BuildArgs []SpecialResourceBuilArgs `json:"buildArgs,omitempty"`
	RunArgs   []SpecialResourceRunArgs  `json:"runArgs,omitempty"`
	Artifacts SpecialResourceArtifacts  `json:"artifacts,omitempty"`
}

// SpecialResourceDependsOn defines the desired state of SpecialResource
// +k8s:openapi-gen=true
type SpecialResourceDependsOn struct {
	Name []string `json:"name"`
}

// SpecialResourceSpec defines the desired state of SpecialResource
// +k8s:openapi-gen=true
type SpecialResourceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	DriverContainer SpecialResourceDriverContainer `json:"driverContainer,omitempty"`
	Node            SpecialResourceNode            `json:"node,omitempty"`
	DependsOn       SpecialResourceDependsOn       `json:"dependsOn,omitempty"`
}

// SpecialResourceStatus defines the observed state of SpecialResource
// +k8s:openapi-gen=true
type SpecialResourceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	State string `json:"state"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SpecialResource is the Schema for the specialresources API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type SpecialResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SpecialResourceSpec   `json:"spec,omitempty"`
	Status SpecialResourceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SpecialResourceList contains a list of SpecialResource
type SpecialResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SpecialResource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SpecialResource{}, &SpecialResourceList{})
}
