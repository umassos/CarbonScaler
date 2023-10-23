package v1

type CarbonScalerSpec struct {
	MinReplicas int32 `json:"minReplicas,omitempty"`
	MaxReplicas int32 `json:"maxReplicas,omitempty"`
	DeadLine    int32 `json:"deadline,omitempty"`
	// +kubebuilder:default := 0
	Progress    int32  `json:"progress,omitempty"`
	ProfileName string `json:"profileName"`
}
