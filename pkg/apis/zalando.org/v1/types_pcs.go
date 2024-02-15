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
	Status PlatformCredentialsStatus `json:"status,omitempty"`
}

// PlatformCredentialsSpec is the spec part of the StackSet.
// +k8s:deepcopy-gen=true
type PlatformCredentialsSpec struct {
	Application  string            `json:"application"`
	Clients      map[string]Client `json:"clients,omitempty"`
	Tokens       map[string]Token  `json:"tokens,omitempty"`
	TokenVersion string            `json:"token_version,omitempty"`
}

// +k8s:deepcopy-gen=true
type Client struct {
	Realm       string `json:"realm,omitempty"`
	Grant       string `json:"grant,omitempty"`
	RedirectURI string `json:"redirectUri,omitempty"`
}

// +k8s:deepcopy-gen=true
type Token struct {
	Privileges []string `json:"privileges"`
}

// PlatformCredentialsStatus is the status part of the Stack.
// +k8s:deepcopy-gen=true
type PlatformCredentialsStatus struct {
	ObservedGeneration int64             `json:"observedGeneration,omitempty"`
	Errors             []string          `json:"errors,omitempty"`
	Problems           []string          `json:"problems,omitempty"`
	Tokens             map[string]Token  `json:"tokens,omitempty"`
	Clients            map[string]Client `json:"clients,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PlatformCredentialsSetList is a list of StackSets.
// +k8s:deepcopy-gen=true
type PlatformCredentialsSetList struct {
	metav1.TypeMeta `json:""`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []PlatformCredentialsSet `json:"items"`
}
