package test

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Types here are partial copies of github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1

type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata" protobuf:"bytes,1,opt,name=metadata"`
	Spec              ApplicationSpec   `json:"spec" protobuf:"bytes,2,opt,name=spec"`
	Status            ApplicationStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
	Operation         *Operation        `json:"operation,omitempty" protobuf:"bytes,4,opt,name=operation"`
}

type ApplicationSpec struct {
	Source ApplicationSource `json:"source" protobuf:"bytes,1,opt,name=source"`
}

type ApplicationStatus struct {
	Sync           SyncStatus      `json:"sync,omitempty" protobuf:"bytes,2,opt,name=sync"`
	Health         HealthStatus    `json:"health,omitempty" protobuf:"bytes,3,opt,name=health"`
	OperationState *OperationState `json:"operationState,omitempty" protobuf:"bytes,7,opt,name=operationState"`
}

type Operation struct {
}

type SyncStatus struct {
	Status     SyncStatusCode `json:"status" protobuf:"bytes,1,opt,name=status,casttype=SyncStatusCode"`
	ComparedTo ComparedTo     `json:"comparedTo,omitempty" protobuf:"bytes,2,opt,name=comparedTo"`
}

type SyncStatusCode string

const (
	SyncStatusCodeUnknown   SyncStatusCode = "Unknown"
	SyncStatusCodeSynced    SyncStatusCode = "Synced"
	SyncStatusCodeOutOfSync SyncStatusCode = "OutOfSync"
)

type ComparedTo struct {
	Source ApplicationSource `json:"source" protobuf:"bytes,1,opt,name=source"`
}

type ApplicationSource struct {
	RepoURL        string `json:"repoURL" protobuf:"bytes,1,opt,name=repoURL"`
	Path           string `json:"path,omitempty" protobuf:"bytes,2,opt,name=path"`
	TargetRevision string `json:"targetRevision,omitempty" protobuf:"bytes,4,opt,name=targetRevision"`
}

type HealthStatus struct {
	Status HealthStatusCode `json:"status,omitempty" protobuf:"bytes,1,opt,name=status"`
}

type HealthStatusCode string

const (
	// Indicates that health assessment failed and actual health status is unknown
	HealthStatusUnknown HealthStatusCode = "Unknown"
	// Progressing health status means that resource is not healthy but still have a chance to reach healthy state
	HealthStatusProgressing HealthStatusCode = "Progressing"
	// Resource is 100% healthy
	HealthStatusHealthy HealthStatusCode = "Healthy"
	// Assigned to resources that are suspended or paused. The typical example is a
	// [suspended](https://kubernetes.io/docs/tasks/job/automated-tasks-with-cron-jobs/#suspend) CronJob.
	HealthStatusSuspended HealthStatusCode = "Suspended"
	// Degrade status is used if resource status indicates failure or resource could not reach healthy state
	// within some timeout.
	HealthStatusDegraded HealthStatusCode = "Degraded"
	// Indicates that resource is missing in the cluster.
	HealthStatusMissing HealthStatusCode = "Missing"
)

type OperationState struct {
	Phase OperationPhase `json:"phase" protobuf:"bytes,2,opt,name=phase"`
}

type OperationPhase string

const (
	OperationRunning     OperationPhase = "Running"
	OperationTerminating OperationPhase = "Terminating"
	OperationFailed      OperationPhase = "Failed"
	OperationError       OperationPhase = "Error"
	OperationSucceeded   OperationPhase = "Succeeded"
)

type AppProject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata" protobuf:"bytes,1,opt,name=metadata"`
	Spec              AppProjectSpec `json:"spec" protobuf:"bytes,2,opt,name=spec"`
}

type AppProjectSpec struct {
	Destinations []ApplicationDestination `json:"destinations,omitempty" protobuf:"bytes,2,name=destination"`
}

type ApplicationDestination struct {
	Namespace string `json:"namespace,omitempty" protobuf:"bytes,2,opt,name=namespace"`
}
