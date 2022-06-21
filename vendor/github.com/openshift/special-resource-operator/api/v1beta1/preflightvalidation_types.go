/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	VerificationTrue    string = "True"
	VerificationFalse   string = "False"
	VerificationError   string = "Error"
	VerificationUnknown string = "Unknown"
)

// PreflightValidationSpec describes the desired state of the resource, such as the OCP image
// that SR CRs need to be verified against and the debug configuration of the logs
// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
// +kubebuilder:validation:Required
type PreflightValidationSpec struct {
	// Debug enables additional logging.
	// +kubebuilder:validation:Optional
	Debug bool `json:"debug"`

	// UpdateImage describe the OCP image that all SR CRs need to be checked against.
	// +kubebuilder:validation:Required
	UpdateImage string `json:"updateImage"`
}

type SRStatus struct {
	// Name of SR CR being checked
	// +required
	Name string `json:"name"`

	// Status of SR CR verification: true (verified), false (verification failed),
	// error (error during verification process), unknown (verification has not started yet)
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=True;False;Error;Unknown
	VerificationStatus string `json:"verificationStatus"`

	// StatusReason contains a string describing the status source.
	// +optional
	StatusReason string `json:"statusReason,omitempty"`

	// LastTransitionTime is the last time the CR status transitioned from one status to another.
	// This should be when the underlying status changed.  If that is not known, then using the time when the API field changed is acceptable.
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=date-time
	LastTransitionTime metav1.Time `json:"lastTransitionTime" protobuf:"bytes,4,opt,name=lastTransitionTime"`
}

// PreflightValidationStatus is the most recently observed status of the PreflightValidation.
// It is populated by the system and is read-only.
// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
type PreflightValidationStatus struct {
	// CRStatuses contain observations about each SpecialResource's preflight upgradability validation
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +optional
	SRStatuses []SRStatus `json:"srStatuses,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// PreflightValidation initiates a preflight validations for all SpecialResources on the current Kuberentes cluster.
// +kubebuilder:resource:path=preflightvalidations,scope=Cluster
// +kubebuilder:resource:path=preflightvalidations,scope=Cluster,shortName=pv
type PreflightValidation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +kubebuilder:validation:Required

	Spec   PreflightValidationSpec   `json:"spec,omitempty"`
	Status PreflightValidationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PreflightValidationList is a list of PreflightValidation objects.
type PreflightValidationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// List of PreflightValidation. More info:
	// https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md
	Items []PreflightValidation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PreflightValidation{}, &PreflightValidationList{})
}
