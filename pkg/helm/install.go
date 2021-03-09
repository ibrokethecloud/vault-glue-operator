package helm

import (
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
)

type Wrapper struct {
	Namespace       string
	ServiceAccount  string
	VaultAddress    string
	VaultSkipVerify bool
	VaultCACert     bool
	MountName       string
	RoleName        string
}

// ChartVersion variable is passed via build flags when a new version is available
var ChartVersion = "6.4.0"

const (
	HelmCommand = "helm"
	ChartPath   = "/data/"
	ValuesYaml  = `
env:
  VAULT_ADDR: {{ .VaultAddress }}
  {{if .VaultCACert -}}NODE_EXTRA_CA_CERTS: "/usr/local/share/ca-certificates/ca.pem"{{- end}}
  VAULT_SKIP_VERIFY: {{ .VaultSkipVerify }}
  DEFAULT_VAULT_MOUNT_POINT: {{ .MountName }}
  DEFAULT_VAULT_ROLE: {{ .RoleName }}

{{if .VaultCACert -}}
filesFromSecret:
  certificate-authority:
    secret: vault-ca
    mountPath: /usr/local/share/ca-certificates
{{- end }}

serviceAccount:
  create: false
  name: {{ .ServiceAccount }}
`
)

// InstallChart is used by the operator to manage helm chart install for external secrets
func (w *Wrapper) InstallChart() (cmdOutput []byte, err error) {
	chartPath, ok := os.LookupEnv("CHART_PATH")
	if !ok {
		chartPath = ChartPath
	}
	output, err := w.generateValues()
	if err != nil {
		return cmdOutput, err
	}

	tmpValues, err := ioutil.TempFile("/tmp", "values")
	if err != nil {
		return cmdOutput, err
	}

	if _, err = fmt.Fprintln(tmpValues, output.String()); err != nil {
		return cmdOutput, err
	}

	if err = tmpValues.Close(); err != nil {
		return cmdOutput, err
	}

	defer os.Remove(tmpValues.Name())

	installArgsStr := fmt.Sprintf("upgrade --install glue-external-secrets %s/kubernetes-external-secrets-%s.tgz -n %s  -f %s",
		chartPath, ChartVersion, w.Namespace, tmpValues.Name())
	installArgs := strings.Fields(installArgsStr)
	helmCommand := exec.Command(HelmCommand, installArgs...)
	cmdOutput, err = helmCommand.CombinedOutput()
	return cmdOutput, err
}

func (w *Wrapper) generateValues() (output bytes.Buffer, err error) {
	valuesTemplate := template.Must(template.New("ValuesYaml").Parse(ValuesYaml))
	err = valuesTemplate.Execute(&output, w)
	return output, err
}
