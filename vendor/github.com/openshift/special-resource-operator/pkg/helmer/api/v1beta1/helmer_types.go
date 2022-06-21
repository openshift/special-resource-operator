/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

// HelmRepo describe a Helm repository.
type HelmRepo struct {
	// Name is the name of the Helm repository.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// URL is the canonical URL of the Helm repository.
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// Username is used to log in against the Helm repository, if required.
	// +kubebuilder:validation:Optional
	Username string `json:"username"`

	// Password is used to log in against the Helm repository, if required.
	// +kubebuilder:validation:Optional
	Password string `json:"password"`

	// CertFile is the path to the client certificate file to be used to authenticate against the Helm repository,
	// if required.
	// +kubebuilder:validation:Optional
	CertFile string `json:"certFile"`

	// KeyFile is the path to the private key file to be used to authenticate against the Helm repository, if required.
	// +kubebuilder:validation:Optional
	KeyFile string `json:"keyFile"`

	// CertFile is the path to the CA certificate file that was used to sign the Helm repository's certificate.
	// +kubebuilder:validation:Optional
	CAFile string `json:"caFile"`

	// If InsecureSkipTLSverify is true, the server's certificate will not be verified against the local CA
	// certificates.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=false
	InsecureSkipTLSverify bool `json:"insecure_skip_tls_verify"`
}

// HelmChart describes a Helm Chart.
type HelmChart struct {
	// Name is the chart's name.
	Name string `json:"name"`

	// Version is the chart's version.
	Version string `json:"version"`

	// Repository is the chart's repository information.
	// +kubebuilder:validation:Required
	Repository HelmRepo `json:"repository"`

	// Tags is a list of tags for this chart.
	// +kubebuilder:validation:Optional
	Tags []string `json:"tags"`
}

func (in *HelmChart) DeepCopyInto(out *HelmChart) {
	*out = *in
	out.Repository = in.Repository
	if in.Tags != nil {
		in, out := &in.Tags, &out.Tags
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is a manually created deepcopy function, copying the receiver, creating a new HelmChart.
func (in *HelmChart) DeepCopy() *HelmChart {
	if in == nil {
		return nil
	}
	out := new(HelmChart)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is a manually created deepcopy function, copying the receiver, writing into out. in must be nonnil.
func (in *HelmRepo) DeepCopyInto(out *HelmRepo) {
	*out = *in
}

// DeepCopy is a manually created deepcopy function, copying the receiver, creating a new HelmRepo.
func (in *HelmRepo) DeepCopy() *HelmRepo {
	if in == nil {
		return nil
	}
	out := new(HelmRepo)
	in.DeepCopyInto(out)
	return out
}
