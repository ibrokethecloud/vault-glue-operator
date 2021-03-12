package vault

import (
	"github.com/hashicorp/vault/api"
)

type VaultRegister struct {
	SAToken      string //base64 encoded JWT Token
	K8SCACert    string //base64 encoded CA cert
	Insecure     bool
	K8SHost      string
	Mount        string //dynamically generated
	SAName       string
	Namespace    string
	Policy       []string
	VaultToken   string
	VaultAddress string
	RoleName     string
}

//RegisterCluster will perform vault auth setup for this cluster
func (v *VaultRegister) RegisterCluster(skipAuth bool) (authEnabled bool, err error) {
	client, err := v.createClient()
	if err != nil {
		return authEnabled, err
	}

	if !skipAuth {
		err = client.Sys().EnableAuthWithOptions(v.Mount, &api.EnableAuthOptions{Type: "kubernetes"})
		if err != nil {
			return authEnabled, err
		}
	}

	authEnabled = true
	configData := make(map[string]interface{})
	configData["kubernetes_host"] = v.K8SHost
	configData["token_reviewer_jwt"] = v.SAToken
	configData["kubernetes_ca_cert"] = v.K8SCACert
	_, err = client.Logical().Write("auth/"+v.Mount+"/config", configData)

	if err != nil {
		return authEnabled, err
	}

	roleData := make(map[string]interface{})
	roleData["bound_service_account_names"] = v.SAName
	roleData["bound_service_account_namespaces"] = v.Namespace
	roleData["policies"] = v.Policy
	roleData["ttl"] = "24h"

	// perform role binding //
	_, err = client.Logical().Write("auth/"+v.Mount+"/role/"+v.RoleName, roleData)
	return authEnabled, err
}

// UnregisterCluster will disable the associated k8s backend
func (v *VaultRegister) UnregisterCluster() (err error) {
	client, err := v.createClient()
	if err != nil {
		return err
	}

	authMap, err := client.Sys().ListAuth()
	if _, ok := authMap[v.Mount+"/"]; ok {
		err = client.Sys().DisableAuth(v.Mount)
	}

	return err
}

func (v *VaultRegister) createClient() (client *api.Client, err error) {
	config := &api.Config{}
	tlsConfig := &api.TLSConfig{Insecure: true}
	err = config.ConfigureTLS(tlsConfig)
	if err != nil {
		return client, err
	}
	client, err = api.NewClient(config)
	if err != nil {
		return client, err
	}
	err = client.SetAddress(v.VaultAddress)
	if err != nil {
		return client, err
	}
	client.SetToken(v.VaultToken)
	return client, nil
}
