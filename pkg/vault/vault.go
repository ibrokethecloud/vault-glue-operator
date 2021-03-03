package vault

type VaultRegister struct {
	SAToken    string //base64 encoded JWT Token
	K8SCACert  string //base64 encoded CA cert
	Insecure   bool
	K8SHost    string
	Mount      string //dynamically generated
	SAName     string
	Namespace  string
	Policy     string
	VaultToken string
}

//RegisterCluster will perform vault auth setup for this cluster
func (v *VaultRegister) RegisterCluster() (err error) {

	return err
}
