# vault-glue-operator

A simple k8s operator to simplify integration with vault using the k8s auth method.

To get started the vault admin would issue a token with a short ttl. This is used to seed a k8s secret.

```
vault token create -ttl=1h -renewable=false
kubectl create secret generic vault-token --from-literal=token="s.hEHPq50qOyd9Rv5YDXUFFVmN" -n vault-glue-operator
```

The operator looks for a Register request crd like the one below:

```yaml
apiVersion: vault.cattle.io/v1alpha1
kind: Register
metadata:
  name: external-secrets
spec:
  vaultAddr: "https://vaultAddress"
  serviceAccount: external-secrets-kubernetes-external-secrets
  namespace: kube-external-secrets
  sslDisable: true
  vaultPolicy:
    - fleet-demo
  roleName: fleet-demo
```

The operator uses this spec, to create service account in the defined namespace and then setup vault k8s auth on a randomly generate mount path. 

This service account is then subsequently used to install the [external-secrets helm chart](https://github.com/external-secrets/kubernetes-external-secrets)

The helm chart is configured to use the newly minted vault auth endpoint and role.

```
▶ kubectl get register
NAME               REGISTERSTATUS   HELMSTATUS   VAULTMOUNT      MESSAGE
external-secrets   Processed        Installed    k8shctcuaxhxk
```

The user can start fetching secrets from vault using the external secrets crd:

```yaml
apiVersion: 'kubernetes-client.io/v1'
kind: ExternalSecret
metadata:
  name: dummy
  namespace: default
spec:
  backendType: vault
  kvVersion: 1
  data:
    - name: name
      key: secret/fleet/dummy
      property: name
```     

external-secrets operator will process this request, fetch the secret from vault and create a k8s secret.

```
kubectl get externalsecret -n default
NAME    LAST SYNC   STATUS    AGE
dummy   7s          SUCCESS   13m
```

```
kubectl get secret dummy -n default
NAME    TYPE     DATA   AGE
dummy   Opaque   1      18m
```

Now the k8s workloads can start using this secret.