// +groupName=home.lab
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=myapis,scope=Namespaced
type MyAPI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MyAPISpec `json:"spec"`
}

// MyAPISpec contains the details of the best API in the world
type MyAPISpec struct {
	// +optional
	MyField string `json:"myField"`
	// +optional
	MyArray []string `json:"myArray"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// MyAPIList contains a list of myapis
type DNSRecordList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MyAPI `json:"items"`
}
