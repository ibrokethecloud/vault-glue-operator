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

package controllers

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"os"

	"k8s.io/apimachinery/pkg/types"

	"github.com/go-logr/logr"
	vaultv1alpha1 "github.com/ibrokethecloud/vault-glue-operator/pkg/api/v1alpha1"
	"github.com/ibrokethecloud/vault-glue-operator/pkg/helm"
	"github.com/ibrokethecloud/vault-glue-operator/pkg/vault"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultNamespace = "vault-glue-operator"
	DefaultSecret    = "vault-token"
	finalizer        = "vault-glue-operator"
)

// RegisterReconciler reconciles a Register object
type RegisterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=vault.io,resources=registers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vault.io,resources=registers/status,verbs=get;update;patch
// Reconcile runs the reconilliation loop
func (r *RegisterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("register", req.NamespacedName)
	var requeue bool
	registerRequest := &vaultv1alpha1.Register{}

	if err := r.Get(ctx, req.NamespacedName, registerRequest); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch Register Request")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	registerStatus := registerRequest.Status.DeepCopy()
	if registerRequest.DeletionTimestamp.IsZero() {
		switch status := registerStatus.Status; status {
		case "":
			//Lets check if VaultRegoSecret exists//
			token, err := r.checkVaultSecretExists(ctx)
			if err != nil {
				log.Error(err, "Error during vault registeration secret check")
				registerStatus.Message = err.Error()
			} else {
				registerStatus.Message = ""
				registerRequest.Annotations["token"] = token
				registerStatus.Status = "VaultTokenPresent"
			}
		case "VaultTokenPresent":
			// Create service account
			log.Info("Managing service account")
			err := r.createSA(ctx, registerRequest)
			if err != nil {
				registerStatus.Message = err.Error()
				log.Error(err, "Error during SA creation")
			} else {
				registerStatus.Message = ""
				registerStatus.Status = "ServiceAccountCreated"
			}

		case "ServiceAccountCreated":
			// Perform Vault rego
			log.Info("Managing setting up vault auth")
			v, err := r.prepareVaultRequest(ctx, registerRequest)
			var authEnabled, skipAuth bool
			if err != nil {
				log.Error(err, "Error during Vault setup")
				registerStatus.Message = err.Error()
			} else {
				authStringStatus, ok := registerRequest.Annotations["auth-enabled"]
				if ok {
					skipAuth, err = strconv.ParseBool(authStringStatus)
					if err != nil {
						registerStatus.Message = err.Error()
					}
				}
				authEnabled, err = v.RegisterCluster(skipAuth)
				if err != nil {
					registerStatus.Message = err.Error()
					if strings.Contains(err.Error(), "path is already in use at") {
						delete(registerRequest.Annotations, "mountPath")
					}
				} else {
					registerStatus.Message = ""
					registerStatus.Status = "VaultRegistrationComplete"
					registerStatus.VaultAuthMount = registerRequest.Annotations["mountPath"]
					if authEnabled {
						registerRequest.Annotations["auth-enabled"] = "true"
					}
				}
			}
		case "VaultRegistrationComplete":
			//Lets deploy external secrets helm chart
			if registerRequest.Spec.SkipExternalSecretInstall {
				log.Info("Help chart skipped")
				registerStatus.Message = "External Secret Install Skipped"
				registerStatus.Status = "Processed"
			} else {
				log.Info("Installing helm chart")
				// perform helm install
				output, err := r.installChart(ctx, registerRequest)
				if err != nil {
					log.Error(err, string(output))
					registerStatus.Message = err.Error()
				} else {
					log.Info(string(output))
					registerStatus.Message = ""
					registerStatus.HelmStatus = "Installed"
					registerStatus.Status = "Processed"
				}
			}
		case "Processed":
			return ctrl.Result{}, nil

		}
		registerRequest.Status = *registerStatus
		controllerutil.AddFinalizer(registerRequest, finalizer)
		requeue = true

	} else {
		if containsString(registerRequest.ObjectMeta.Finalizers, finalizer) {
			// lets delete the instance //
			log.Info("Cleaning up associated resources")
			registerStatus = registerRequest.Status.DeepCopy()
			if registerStatus.HelmStatus == "Installed" {
				// lets remove the chart //
				output, err := r.uninstallChart(ctx, registerRequest)
				log.Info(string(output))
				if err != nil {
					registerStatus.Message = err.Error()
					requeue = true
				} else {
					registerStatus.HelmStatus = ""
				}
			}

			if registerStatus.VaultAuthMount != "" {
				v, err := r.prepareVaultRequest(ctx, registerRequest)
				if err != nil {
					registerStatus.Message = err.Error()
					requeue = true
				} else {
					err = v.UnregisterCluster()
					if err != nil {
						registerStatus.Message = err.Error()
						requeue = true
					}
					registerStatus.VaultAuthMount = ""
				}
			}
			registerRequest.Status = *registerStatus
		}
		if registerStatus.HelmStatus == "" && registerStatus.VaultAuthMount == "" {
			controllerutil.RemoveFinalizer(registerRequest, finalizer)
		}
	}

	return ctrl.Result{Requeue: requeue}, r.Update(ctx, registerRequest)
}

// SetupWithManager will setup the controller to watch objects
func (r *RegisterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vaultv1alpha1.Register{}).
		Complete(r)
}

func (r *RegisterReconciler) checkVaultSecretExists(ctx context.Context) (token string, err error) {
	namespace, ok := os.LookupEnv("NAMESPACE")
	if !ok {
		namespace = DefaultNamespace
	}

	if len(namespace) == 0 {
		namespace = DefaultNamespace
	}

	secret := &v1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: DefaultSecret}, secret)
	if err != nil {
		return token, err
	}
	if tokenByte, ok := secret.Data["token"]; !ok {
		return token, fmt.Errorf("token key not found in secret %s in namespace %s", DefaultSecret, namespace)
	} else {
		token = string(tokenByte)
	}
	return token, err
}

func (r *RegisterReconciler) createSA(ctx context.Context, registerRequest *vaultv1alpha1.Register) (err error) {
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: registerRequest.Spec.Namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, ns, func() error {
		return nil
	})

	sa := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      registerRequest.Spec.ServiceAccount,
			Namespace: registerRequest.Spec.Namespace,
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
		return nil
	})
	return err
}

func (r *RegisterReconciler) prepareVaultRequest(ctx context.Context,
	registerRequest *vaultv1alpha1.Register) (v *vault.VaultRegister, err error) {
	sa := &v1.ServiceAccount{}
	err = r.Get(ctx, types.NamespacedName{Namespace: registerRequest.Spec.Namespace,
		Name: registerRequest.Spec.ServiceAccount}, sa)
	if err != nil {
		return v, err
	}
	var typedSecret types.NamespacedName
	for _, secret := range sa.Secrets {
		typedSecret.Name = secret.Name
		typedSecret.Namespace = registerRequest.Spec.Namespace
	}

	saSecret := &v1.Secret{}
	err = r.Get(ctx, typedSecret, saSecret)
	if err != nil {
		return v, err
	}

	v = &vault.VaultRegister{}
	v.SAToken = string(saSecret.Data["token"])
	v.K8SCACert = string(saSecret.Data["ca.crt"])
	if len(registerRequest.Spec.K8SEndpoint) != 0 {
		v.K8SHost = registerRequest.Spec.K8SEndpoint
	} else {
		masterNode, err := r.findMasterNodes(ctx)
		if err != nil {
			return v, err
		}
		v.K8SHost = fmt.Sprintf("https://%s:6443", masterNode)
	}
	v.SAName = registerRequest.Spec.ServiceAccount
	v.Namespace = registerRequest.Spec.Namespace
	v.Policy = registerRequest.Spec.VaultPolicy
	v.VaultToken = registerRequest.Annotations["token"]
	v.VaultAddress = registerRequest.Spec.VaultAddr
	v.RoleName = registerRequest.Spec.RoleName
	if mount, ok := registerRequest.Annotations["mountPath"]; !ok {
		v.Mount = "k8s" + generateRandomString(10)
	} else {
		v.Mount = mount
	}
	// Add to annotation. Will be needed for helm chart
	registerRequest.Annotations["mountPath"] = v.Mount
	return v, err
}

func (r *RegisterReconciler) findMasterNodes(ctx context.Context) (masterNode string, err error) {
	nodeList := &v1.NodeList{}
	err = r.List(ctx, nodeList)
	if err != nil {
		return masterNode, err
	}

	for _, node := range nodeList.Items {
		if isMaster(node.GetLabels()) {
			masterNode = getAddress(node)
		}
	}
	return masterNode, err
}

func (r *RegisterReconciler) installChart(ctx context.Context,
	registerRequest *vaultv1alpha1.Register) (output []byte, err error) {
	var vaultCertPresent bool
	if len(registerRequest.Spec.VaultCACert) != 0 {
		vaultCertPresent = true
	}

	helmWrapper := prepareHelmWrapper(registerRequest, vaultCertPresent)
	if vaultCertPresent {
		// need to create the secret with the ca cert chain
		err = r.createCASecret(ctx, registerRequest)
		if err != nil {
			return output, err
		}
	}

	output, err = helmWrapper.InstallChart()
	return output, err
}

func (r *RegisterReconciler) uninstallChart(ctx context.Context,
	registerRequest *vaultv1alpha1.Register) (output []byte, err error) {
	helmWrapper := prepareHelmWrapper(registerRequest, false)
	output, err = helmWrapper.UninstallChart()
	return output, err
}

func prepareHelmWrapper(registerRequest *vaultv1alpha1.Register, vaultCertPresent bool) (helmWrapper helm.Wrapper) {

	helmWrapper = helm.Wrapper{
		Namespace:       registerRequest.Spec.Namespace,
		ServiceAccount:  registerRequest.Spec.ServiceAccount,
		VaultAddress:    registerRequest.Spec.VaultAddr,
		VaultSkipVerify: registerRequest.Spec.SSLDisable,
		VaultCACert:     vaultCertPresent,
		MountName:       registerRequest.Status.VaultAuthMount,
		RoleName:        registerRequest.Spec.RoleName,
	}
	return helmWrapper
}

func (r *RegisterReconciler) createCASecret(ctx context.Context, registerRequest *vaultv1alpha1.Register) (err error) {
	stringData := make(map[string]string)
	stringData["ca.pem"] = registerRequest.Spec.VaultCACert
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vault-ca",
			Namespace: registerRequest.Spec.Namespace,
		},
		StringData: stringData,
	}

	err = r.Create(ctx, secret)
	return err
}

func isMaster(labels map[string]string) (ok bool) {
	for key, value := range labels {
		if strings.Contains(key, "controlplane") || strings.Contains(key, "master") {
			if value == "true" {
				ok = true
			}
		}
	}
	return ok
}

func getAddress(node v1.Node) (address string) {
	var externalIP, internalIP string
	for _, nodeAddress := range node.Status.Addresses {
		if nodeAddress.Type == v1.NodeInternalIP {
			internalIP = nodeAddress.Address
		}
		if nodeAddress.Type == v1.NodeExternalIP {
			externalIP = nodeAddress.Address
		}
	}

	if externalIP != "" {
		address = externalIP
	} else {
		address = internalIP
	}

	return address
}

func generateRandomString(size int) (random string) {
	var letters = []rune("abcdefghijklmnopqrstuvwxyz")
	b := make([]rune, size)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	random = string(b)
	return random
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
