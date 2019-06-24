/*
Copyright 2019 The Seed team.

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

package webhook

import (
	"os"

	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	k8types "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/builder"
)

var logt = logf.Log.WithName("admission")

// AddToManagerFuncs is a list of functions to add all Controllers to the Manager
var AddToManagerFuncs []func(manager.Manager) error

// LabelsConfigPath is path where the ConfigMap is mounted for the required labels
const LabelsConfigPath = "/etc/config/labels/labels"

// ImmutablesConfigPath is path where the ConfigMap is mounted for the immutable specifiction fields
const ImmutablesConfigPath = "/etc/config/immutables/immutables"

// DependencyMustExistLabel is the label flagging dependency check before approving admission
const DependencyMustExistLabel = "solsa.ibm.com/dependencyCheck"

// AddToManager adds all Controllers to the Manager
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources="",verbs=get;list
func AddToManager(m manager.Manager) error {
	for _, f := range AddToManagerFuncs {
		if err := f(m); err != nil {
			logt.Error(err, "webhook.AddToManagerFuncs error")
			return err
		}
	}

	logt.Info("env variable settings", "ADMISSION_CONTROL_LABELS", os.Getenv("ADMISSION_CONTROL_LABELS"))
	logt.Info("env variable settings", "ADMISSION_CONTROL_IMMUTABLES", os.Getenv("ADMISSION_CONTROL_IMMUTABLES"))
	logt.Info("env variable settings", "ADMISSION_CONTROL_DEPENDENCIES", os.Getenv("ADMISSION_CONTROL_DEPENDENCIES"))

	if os.Getenv("ADMISSION_CONTROL_LABELS") == "true" ||
		os.Getenv("ADMISSION_CONTROL_IMMUTABLES") == "true" ||
		os.Getenv("ADMISSION_CONTROL_DEPENDENCIES") == "true" {
		err := startWebhookServer(m)
		if err != nil {
			return err
		}
	}
	return nil
}

// startWebhookServer starts admission control webhook server
func startWebhookServer(mgr manager.Manager) error {
	//logf.SetLogger(logf.ZapLogger(false))
	//log := logf.Log.WithName("ibmcloud-operators admission control")
	logt.Info("start admission webhook server")

	ruleMutating := admissionregistrationv1beta1.RuleWithOperations{
		Operations: []admissionregistrationv1beta1.OperationType{
			admissionregistrationv1beta1.Create,
			admissionregistrationv1beta1.Update,
		},
		Rule: admissionregistrationv1beta1.Rule{
			APIGroups:   []string{"apps"},
			APIVersions: []string{"v1"},
			Resources:   []string{"deployments"},
		},
	}
	wh1, err := builder.NewWebhookBuilder().
		Mutating().
		Name("ibmcloud-operators.admission.control").
		Rules(ruleMutating).
		Path("/mutate").
		Handlers(&AdmissionMutator{}).
		WithManager(mgr).
		Build()
	if err != nil {
		logt.Error(err, "failed to create Mutating webhook ")
		return err
	}

	rule1 := admissionregistrationv1beta1.RuleWithOperations{
		Operations: []admissionregistrationv1beta1.OperationType{
			admissionregistrationv1beta1.Create,
			admissionregistrationv1beta1.Update,
		},
		Rule: admissionregistrationv1beta1.Rule{
			APIGroups:   []string{"ibmcloud.ibm.com"},
			APIVersions: []string{"v1alpha1"},
			Resources:   []string{"services", "bindings", "esindices", "topics"},
		},
	}
	/*	rule2 := admissionregistrationv1beta1.RuleWithOperations{
		Operations: []admissionregistrationv1beta1.OperationType{
			admissionregistrationv1beta1.Create,
			admissionregistrationv1beta1.Update,
		},
		Rule: admissionregistrationv1beta1.Rule{
			APIGroups:   []string{"apps"},
			APIVersions: []string{"v1"},
			Resources:   []string{"deployments", "statefulsets"},
		},
	}*/
	wh2, err := builder.NewWebhookBuilder().
		Validating().
		Name("ibmcloud-operators.admission.control").
		Rules(rule1).
		//Rules(rule1, rule2).
		Path("/validate").
		WithManager(mgr).
		Handlers(&AdmissionValidator{}).
		Build()
	if err != nil {
		logt.Error(err, "failed to create Validating webhook")
		return err
	}

	namespace := os.Getenv("POD_NAMESPACE")
	if len(namespace) == 0 {
		namespace = "default"
	}
	secretName := os.Getenv("SECRET_NAME")
	if len(secretName) == 0 {
		secretName = "ibmcloud-admissionwebhook-certs"
	}
	serviceName := os.Getenv("WEBHOOK_SERVICE_NAME")
	if len(serviceName) == 0 {
		serviceName = "controller-manager-service"
	}
	certDir := os.Getenv("CERT_DIR")
	if len(certDir) == 0 {
		certDir = "/tmp/cert"
	}
	var webhookServerName = "ibmcloud-operators-admission-webhook"
	logt.Info("webhook server settings", "name", webhookServerName, "namespace", namespace,
		"secret name", secretName, "service name", serviceName, "cert dir", certDir)

	svr, err := webhook.NewServer(webhookServerName, mgr, webhook.ServerOptions{
		Port:    443,
		CertDir: certDir,

		BootstrapOptions: &webhook.BootstrapOptions{
			Secret: &k8types.NamespacedName{
				Namespace: namespace,
				Name:      secretName,
			},

			Service: &webhook.Service{
				Namespace: namespace,
				Name:      serviceName,
				// Selectors should select the pods that runs this webhook server.
				Selectors: map[string]string{
					"control-plane":           "controller-manager",
					"controller-tools.k8s.io": "1.0",
					//"ibmcloud": "admission-control",
				},
			},
		},
	})

	svr.MutatingWebhookConfigName = "ibmcloud-mutating"
	svr.ValidatingWebhookConfigName = "ibmcloud-validating"

	if err != nil {
		logt.Error(err, "failed to start webhook server")
		return err
	}
	logt.Info("webhook server started !!! ")

	err = svr.Register(wh1, wh2)
	if err != nil {
		logt.Error(err, "failed to register webhooks")
		return err
	}
	return nil
}
