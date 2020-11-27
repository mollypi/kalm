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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// for each kalm cluster, there should be only 1 record of ClusterResourceQuota
	ClusterResourceQuotaName = "kalm-cluster-resource-quota"
)

type ClusterResourceQuotaSpec struct {
	CPU                   resource.Quantity `json:"cpu"`
	Memory                resource.Quantity `json:"memory"`
	Storage               resource.Quantity `json:"storage"`
	EphemeralStorage      resource.Quantity `json:"ephemeralStorage"`
	Traffic               resource.Quantity `json:"traffic"`
	ApplicationsCount     resource.Quantity `json:"applicationsCount"`
	ComponentsCount       resource.Quantity `json:"componentsCount"`
	ServicesCount         resource.Quantity `json:"servicesCount"`
	RoleBindingCount      resource.Quantity `json:"roleBindingCount"`
	AccessTokensCount     resource.Quantity `json:"accessTokens"`
	DockerRegistriesCount resource.Quantity `json:"dockerRegistriesCount"`
	HttpRoutesCount       resource.Quantity `json:"httpRoutesCount"`
	HttpsCertsCount       resource.Quantity `json:"httpsCertsCount"`
}

type ClusterResourceQuotaStatus struct {
	UsedResourceQuota ResourceList `json:"usedResourceQuota"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

type ClusterResourceQuota struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterResourceQuotaSpec   `json:"spec,omitempty"`
	Status ClusterResourceQuotaStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ClusterResourceQuotaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterResourceQuota `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterResourceQuota{}, &ClusterResourceQuotaList{})
}
