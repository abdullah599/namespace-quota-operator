/*
Copyright 2025.

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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// QuotaProfileFinalizer is the name of the finalizer added to QuotaProfile resources
	QuotaProfileFinalizer = "quota.dev.operator/finalizer"
	
	// QuotaProfileLabelKey is the label key used to identify quota profiles
	QuotaProfileLabelKey = "quota.dev.operator/profile"

	// QuotaProfileLastUpdateTimestamp is used to track when the namespace quota configuration was last updated. Label is added to the namespace when the quota profile is applied.
	QuotaProfileLastUpdateTimestamp = "quota.dev.operator/profile/last-update-timestamp"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// QuotaProfileSpec defines the desired state of QuotaProfile.
type QuotaProfileSpec struct {
	NamespaceSelector  NamespaceSelector      `json:"namespaceSelector"`
	Precedence         uint16                 `json:"precedence"`
	ResourceQuotaSpecs []v1.ResourceQuotaSpec `json:"resourceQuotaSpecs,omitempty"`
	LimitRangeSpecs    []v1.LimitRangeSpec    `json:"limitRangeSpecs,omitempty"`
}

type NamespaceSelector struct {

	//NOTE: only one the these selectors can be used
	// All of the labels mentioned in this field will be required to select the namespace
	MatchLabels map[string]string `json:"matchLabels,omitempty"`

	// ResourceQuota will be applied to the namespace with the specified name
	MatchName *string `json:"matchNameRegex,omitempty"`
}

// QuotaProfileStatus defines the observed state of QuotaProfile.
type QuotaProfileStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// QuotaProfile is the Schema for the quotaprofiles API.
type QuotaProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   QuotaProfileSpec   `json:"spec,omitempty"`
	Status QuotaProfileStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// QuotaProfileList contains a list of QuotaProfile.
type QuotaProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []QuotaProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&QuotaProfile{}, &QuotaProfileList{})
}
