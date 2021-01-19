package test

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Types here are partial copies of github.com/jetstack/cert-manager/pkg/apis/certmanager/v1beta1

type Certificate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Status            CertificateStatus `json:"status"`
}

type CertificateStatus struct {
	Conditions []CertificateCondition `json:"conditions,omitempty"`
}

type CertificateCondition struct {
	Type               CertificateConditionType `json:"type"`
	Status             ConditionStatus          `json:"status"`
	LastTransitionTime *metav1.Time             `json:"lastTransitionTime,omitempty"`
	Reason             string                   `json:"reason,omitempty"`
	Message            string                   `json:"message,omitempty"`
}

type CertificateConditionType string

const (
	CertificateConditionReady CertificateConditionType = "Ready"
)

type ConditionStatus string

const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)

type CertificateRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Status            CertificateRequestStatus `json:"status"`
}

type CertificateRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CertificateRequest `json:"items"`
}

type CertificateRequestStatus struct {
	Conditions []CertificateRequestCondition `json:"conditions,omitempty"`
}

type CertificateRequestCondition struct {
	Type    CertificateRequestConditionType `json:"type"`
	Status  ConditionStatus                 `json:"status"`
	Reason  string                          `json:"reason,omitempty"`
	Message string                          `json:"message,omitempty"`
}

type CertificateRequestConditionType string

const (
	CertificateRequestConditionReady CertificateRequestConditionType = "Ready"
)

const (
	CertificateRequestReasonPending = "Pending"
	CertificateRequestReasonFailed  = "Failed"
	CertificateRequestReasonIssued  = "Issued"
)
