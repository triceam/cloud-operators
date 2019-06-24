package webhook

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	//	"k8s.io/client-go/rest"
)

// IBMCloudCatalogURI is the uri for IBM cloud service catalog. It is used by validator to verify if a service plan is updateable.
const IBMCloudCatalogURI = "https://globalcatalog.cloud.ibm.com/api/v1"

func getDependencies(kind string, admobj []byte) []Dependent {
	var (
		dependencies []Dependent
		contnrs      []corev1.Container
		volumes      []corev1.Volume
	)
	logt.Info("getDependencies", "kind", kind)
	//parse request sepc and convert to a flat map

	switch strings.ToLower(kind) {
	case "deployment":
		var req1 appsv1.Deployment
		//	var contnrs apps.DeploymentSpec
		json.Unmarshal(admobj, &req1)
		contnrs = req1.Spec.Template.Spec.Containers
		volumes = req1.Spec.Template.Spec.Volumes
	case "statefulset":
		var req2 appsv1.StatefulSet
		json.Unmarshal(admobj, &req2)
		logt.Info("decoded StaefulSet", "request", fmt.Sprintf("%v", req2))
		contnrs = req2.Spec.Template.Spec.Containers
		volumes = req2.Spec.Template.Spec.Volumes

	default:
		logt.Info("unsupported Kind %s\n", "kind", kind)
		return dependencies
	}

	for _, contnr := range contnrs {
		logt.Info("container spec", "container", fmt.Sprintf("%v", contnr))
		env := contnr.Env
		logt.Info("container spec", "env", fmt.Sprintf("%v", env))
		for _, dep := range env {
			if dep.ValueFrom != nil {
				if dep.ValueFrom.SecretKeyRef != nil {
					dependencies = append(dependencies, Dependent{
						Kind: "Secret",
						Name: dep.ValueFrom.SecretKeyRef.Name,
					})
				} else if dep.ValueFrom.ConfigMapKeyRef != nil {
					dependencies = append(dependencies, Dependent{
						Kind: "ConfigMap",
						Name: dep.ValueFrom.ConfigMapKeyRef.Name,
					})
				}
			}
		}
	}
	for _, volume := range volumes {
		logt.Info("container spec", "volume", fmt.Sprintf("%v", volume))
		if volume.Secret != nil {
			dependencies = append(dependencies, Dependent{
				Kind: "Secret",
				Name: volume.Secret.SecretName,
			})
		} else if volume.ConfigMap != nil {
			dependencies = append(dependencies, Dependent{
				Kind: "ConfigMap",
				Name: volume.ConfigMap.Name,
			})
		}
	}
	//logt.Info("got dependencies", "dependencies", fmt.Sprintf("%v", dependencies))
	uniquedependencies := unique(dependencies)
	logt.Info("uniquedependencies", "dependencies", fmt.Sprintf("%v", uniquedependencies))
	return uniquedependencies
}

// unique removes duplicated dependencies
func unique(input []Dependent) []Dependent {
	keys := make(map[Dependent]bool)
	list := []Dependent{}
	for _, entry := range input {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

// getDependencyMustExistLabel checks if DependencyCheckLlabel exists and its value is true/yes
func getDependencyMustExistLabel(availableLabels map[string]string) bool {
	for _, v := range availableLabels {
		if v == DependencyCheckLlabel {
			if strings.ToLower(availableLabels[v]) == "yes" || strings.ToLower(availableLabels[v]) == "true" {
				logt.Info("getDependencyMustExistLabel() returns true ")
				return true
			}
		}
	}
	logt.Info("getDependencyMustExistLabel() returns false ")
	return false
}

// getImmutablesConfig reads immutables from configmap
func getImmutablesConfig(pathname string) ([]ImmutablesConfig, error) {

	var immutablesConf []ImmutablesConfig
	dat, err := ioutil.ReadFile(pathname)
	if err != nil {
		logt.Error(err, "Immutable config read file error")
		return immutablesConf, err
	}

	if err := json.Unmarshal(dat, &immutablesConf); err != nil {
		logt.Error(err, "Immutables config unmarshal error")
		return immutablesConf, err
	}
	return immutablesConf, nil

}

// getLabelsConfig reads required labels  from configmap
func getLabelsConfig(pathname string) ([]LabelsConfig, error) {

	var labelsConf []LabelsConfig
	logt.Info("configmap labels", "filepath", pathname)
	dat, err := ioutil.ReadFile(pathname)
	if err != nil {
		logt.Error(err, "Labels config read file error")
		return labelsConf, err
	}

	if err := json.Unmarshal(dat, &labelsConf); err != nil {
		logt.Error(err, "Labels configmap unmarshal erro")
		return labelsConf, err
	}
	logt.Info("Labels config data", "labels", fmt.Sprintf("%v", labelsConf))
	return labelsConf, nil

}

// validateLabels validates if the availableLabels contains the required labels for the resource kind
func validateLabels(kind string, availableLabels map[string]string) (bool, string) {
	allowed := true
	var message string
	var requiredLabels []string

	// get labels config
	labelsConfig, err := getLabelsConfig(LabelsConfigPath)
	if err != nil {
		return allowed, "no required labels are found"
	}

	for _, lconfig := range labelsConfig {
		logt.Info("look up labels requirement", "config_kind", lconfig.Kind, "request_kind", kind)
		if kind == lconfig.Kind {
			requiredLabels = lconfig.Labels
			break
		}
	}
	logt.Info("check required labels", "required", fmt.Sprintf("%v", requiredLabels), "available", fmt.Sprintf("%v", availableLabels))
	for _, rl := range requiredLabels {
		if _, ok := availableLabels[rl]; !ok {
			allowed = false
			message = "required labels are not set: " + rl
			break
		}
	}
	return allowed, message
}

// Parse parses a json map and returns a flat map[string] with all keys in lowercase
func Parse(key string, value interface{}) (map[string]interface{}, error) {
	newjson := make(map[string]interface{})

	if jsonobj, ok := value.(map[string]interface{}); ok {
		//fmt.Printf("%s is a map[string]: %v\n", key, jsonobj)
		for key1, value1 := range jsonobj {
			//	fmt.Printf("  key1=%v value1=%v\n", key1, value1)
			newkey := key + "." + key1
			newjson[strings.ToLower(newkey)] = value1
			if _, ok2 := value1.(map[string]interface{}); ok2 {
				newjson2, _ := Parse(newkey, value1)
				for k, v := range newjson2 {
					newjson[strings.ToLower(k)] = v
				}
			}
		}
		return newjson, nil
	}
	return nil, nil
}

// isUpdateable calls ibmcloud catalog to check if a service is plan updateable
func isUpdateable(servicename string) bool {
	encoded := &url.URL{Path: servicename}
	//logt.Info("in isUpdateable()", "encoded_servicename", encoded.String())
	uri := IBMCloudCatalogURI + "?q=" + encoded.String()
	logt.Info("calling ibmcloud catalog", "uri", uri)
	resp, err := restCallFunc(uri, nil, "GET", "", "", true)
	if err != nil || resp.StatusCode != 200 {
		logt.Error(err, "call to ibmcloud catalog failed", "servicename", servicename)
		return false
	}
	//fmt.Printf("resp:  %v", resp)
	mybyte := []byte(resp.Body)
	mycatalog := CloudCatalog{}
	json.Unmarshal(mybyte, &mycatalog)
	upgradeableServices := getPlanUpdateables(mycatalog.Resources)
	if len(upgradeableServices) > 0 {
		return true
	}
	return false
}

// getPlanUpdateables parses the resources for services whose plan is updateable and returns them as an UpdateableService array
func getPlanUpdateables(resources []ResourceC) []UpdateableService {
	var upgradeableServices = []UpdateableService{}
	for i := range resources {
		if resources[i].Kind == "service" {
			logt.Info("in isUpdateable()", "name", resources[i].Name, "id", resources[0].ID, "display_name", resources[0].Overview.Engish.DisplayName, "plan_updateable", resources[0].Metadata.Service.PlanUpdateable)
			if resources[0].Metadata.Service.PlanUpdateable {
				service := UpdateableService{resources[i].Name, resources[0].Overview.Engish.DisplayName, resources[0].ID}
				upgradeableServices = append(upgradeableServices, service)
			}
		}
	}
	return upgradeableServices
}

// restCallFunc makes a rest call
func restCallFunc(rsString string, postBody []byte, method string, header string, token string, expectReturn bool) (RestResult, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	restClient := http.Client{
		Timeout:   time.Second * 15,
		Transport: tr,
	}
	u, _ := url.ParseRequestURI(rsString)
	urlStr := u.String()
	var req *http.Request
	if postBody != nil {

		req, _ = http.NewRequest(method, urlStr, bytes.NewBuffer(postBody))
	} else {
		req, _ = http.NewRequest(method, urlStr, nil)
	}

	if token != "" {
		if header == "" {
			req.Header.Set("Authorization", token)
		} else {
			req.Header.Set(header, token)
		}
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := restClient.Do(req)
	if err != nil {
		return RestResult{}, err
	}
	defer res.Body.Close()

	if expectReturn {
		body, err := ioutil.ReadAll(res.Body)
		result := RestResult{StatusCode: res.StatusCode, Body: string(body[:])}
		return result, err
	}
	return RestResult{}, nil
}
