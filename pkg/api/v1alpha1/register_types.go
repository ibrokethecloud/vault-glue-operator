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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RegisterSpec defines the desired state of Register
type RegisterSpec struct {
	VaultAddr                    string   `json:"vaultAddr"`
	ServiceAccount               string   `json:"serviceAccount"`
	Namespace                    string   `json:"namespace"`
	VaultPolicy                  []string `json:"vaultPolicy"`
	VaultCACert                  string   `json:"vaultCACert,omitempty"`
	SkipExternalSecretInstall    bool     `json:"skipExternalSecretInstall,omitempty"`
	ExternalSecretNamespaceWatch []string `json:"externalSecretNamespaceWatch,omitempty"`
	SSLDisable                   bool     `json:"sslDisable,omitempty"`
	K8SEndpoint                  string   `json:"k8sEndpoint,omitempty"` //to provide an externally loadbalanced k8s endpoint
	RoleName                     string   `json:"roleName"`
}

// RegisterStatus defines the observed state of Register
type RegisterStatus struct {
	Status         string `json:"status"`
	VaultAuthMount string `json:"vaultAuthPath"`
	HelmStatus     string `json:"helmStatus"`
	Message        string `json:"message"`
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="RegisterStatus",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="HelmStatus",type=string,JSONPath=`.status.helmStatus`
// +kubebuilder:printcolumn:name="VaultMount",type=string,JSONPath=`.status.vaultAuthPath`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.message`
// Register is the Schema for the registers API
type Register struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RegisterSpec   `json:"spec,omitempty"`
	Status RegisterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RegisterList contains a list of Register
type RegisterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Register `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Register{}, &RegisterList{})
}
