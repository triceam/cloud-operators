package webhook

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// DependencyCheckLlabel is the label in yaml spec indicating it requires all dependencies before amission
const DependencyCheckLlabel = "solsa.ibm.com/dependencyCheck"

//LabelsConfig - a struct for required labels per resource type
type LabelsConfig struct {
	Kind   string   `json:"kind"`
	Labels []string `json:"labels,omitempty"`
}

//ImmutablesConfig - a struct for immutables
type ImmutablesConfig struct {
	Kind       string   `json:"kind"`
	Immutables []string `json:"immutables,omitempty"`
}

//Dependent identifies a dependent resource in cluster
type Dependent struct {
	Kind string
	Name string
}

//AdmissionObj a general struct for kube request object
type AdmissionObj struct {
	TypeMeta   metav1.TypeMeta   `json:",inline"`
	ObjectMeta metav1.ObjectMeta `json:"metadata"`
}

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

// ServiceC represents the service as defined in ibmcloud catalog api
type ServiceC struct {
	PlanUpdateable bool `json:"plan_updateable"`
}

//MetadataC represents the resource metadata as defined in ibmcloud catalog api
type MetadataC struct {
	Service  ServiceC `json:"service"`
	Original string   `json:"original_name"`
}

//EnglishC represents the resource overview_ui.en as defined in ibmcloud catalog api
type EnglishC struct {
	DisplayName string `json:"display_name"`
}

//OverviewC represents the resource overview_ui as defined in ibmcloud catalog api
type OverviewC struct {
	Engish EnglishC `json:"en"`
}

//ResourceC represents the resource as defined in ibmcloud catalog api
type ResourceC struct {
	Metadata MetadataC `json:"metadata"`
	Overview OverviewC `json:"overview_ui"`
	Kind     string    `json:"kind"`
	ID       string    `json:"id"`
	Name     string    `json:"name"`
}

//CloudCatalog represents the calalog listing as defined in ibmcloud catalog api
type CloudCatalog struct {
	Count     float64     `json:"count"`
	Next      string      `json:"next"`
	Resources []ResourceC `json:"resources"`
}

//UpdateableService represents a cloud service that can be upgraded dynamically
type UpdateableService struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	ID          string `json:"id"`
}

// RestResult is a struct for REST call result
type RestResult struct {
	StatusCode int
	Body       string
	ErrorType  string
}
