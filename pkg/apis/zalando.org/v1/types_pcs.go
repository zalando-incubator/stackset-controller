package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PlatformCredentialsSet describes a platform credentials set
// +k8s:deepcopy-gen=true
type PlatformCredentialsSet struct {
	metav1.TypeMeta   `json:""`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PlatformCredentialsSpec   `json:"spec"`
	Status PlatformCredentialsStatus `json:"status"`
}

// PlatformCredentialsSpec is the spec part of the StackSet.
// +k8s:deepcopy-gen=true
type PlatformCredentialsSpec struct {
	Application  string            `json:"application"`
	Clients      map[string]Client `json:"clients"`
	Tokens       map[string]Token  `json:"tokens"`
	TokenVersion string            `json:"token_version"`
}

// +k8s:deepcopy-gen=true
type Client struct {
	Realm       string `json:"realm"`
	Grant       string `json:"grant"`
	RedirectURI string `json:"redirectUri"`
}

// +k8s:deepcopy-gen=true
type Token struct {
	Privileges []string `json:"privileges"`
}

// PlatformCredentialsStatus is the status part of the Stack.
// +k8s:deepcopy-gen=true
type PlatformCredentialsStatus struct {
	ObservedGeneration int64               `json:"observedGeneration"`
	Errors             []string            `json:"errors"`
	Problems           []string            `json:"problems"`
	Tokens             map[string]struct{} `json:"tokens"`
	Clients            map[string]struct{} `json:"clients"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PlatformCredentialsSetList is a list of StackSets.
// +k8s:deepcopy-gen=true
type PlatformCredentialsSetList struct {
	metav1.TypeMeta `json:""`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []PlatformCredentialsSet `json:"items"`
}
