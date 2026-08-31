package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/certimate-go/certimate/pkg/core"
	tester "github.com/certimate-go/certimate/pkg/core/deployer/testing"
	"github.com/certimate-go/certimate/pkg/plugin"
)

var (
	fp            = tester.Args("BAISHANCDN_")
	fTestCertPath string
	fTestKeyPath  string
	fApiToken     string
	fDomain       string
)

func init() {
	fp.DefineString(&fTestCertPath, "TESTCERTPATH")
	fp.DefineString(&fTestKeyPath, "TESTKEYPATH")
	fp.DefineString(&fApiToken, "APITOKEN")
	fp.DefineString(&fDomain, "DOMAIN")
}

type envDeployTester struct {
	impl               *baishanCdnDeployer
	accessConfigJSON   string
	extendedConfigJSON string
	logger             *slog.Logger
}

var _ core.Deployer = (*envDeployTester)(nil)

func (d *envDeployTester) SetLogger(logger *slog.Logger) {
	d.logger = logger
}

func (d *envDeployTester) Deploy(ctx context.Context, certPEM, privkeyPEM string) (*core.DeployerDeployResult, error) {
	res, err := d.impl.Deploy(ctx, &plugin.DeployRequest{
		AccessConfigJSON:   d.accessConfigJSON,
		ExtendedConfigJSON: d.extendedConfigJSON,
		CertificatePEM:     certPEM,
		PrivateKeyPEM:      privkeyPEM,
	}, d.logger)
	if err != nil {
		return nil, err
	}

	var extendedData map[string]any
	if err := json.Unmarshal([]byte(res.ExtendedDataJSON), &extendedData); err != nil {
		return nil, err
	}
	return &core.DeployerDeployResult{ExtendedData: extendedData}, nil
}

func TestDeploy_EnvGated(t *testing.T) {
	fp.Parse()
	if fApiToken == "" || fTestCertPath == "" || fTestKeyPath == "" {
		t.Skip("requires BAISHANCDN_ credentials and test cert paths")
	}

	accessConfigJSON, err := json.Marshal(map[string]string{
		"apiToken": fApiToken,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Deploy_ToDomain", func(t *testing.T) {
		if fDomain == "" {
			t.Skip("requires BAISHANCDN_DOMAIN")
		}

		extendedConfigJSON, err := json.Marshal(map[string]any{
			"deployTarget":       deployTargetDomain,
			"domainMatchPattern": domainMatchPatternExact,
			"domain":             fDomain,
		})
		if err != nil {
			t.Fatal(err)
		}

		tester.TestDeploy(t, &envDeployTester{
			impl:               &baishanCdnDeployer{},
			accessConfigJSON:   string(accessConfigJSON),
			extendedConfigJSON: string(extendedConfigJSON),
		}, tester.TestDeployArgs{CertPath: fTestCertPath, KeyPath: fTestKeyPath})
	})
}
