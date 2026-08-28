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
	fp                 = tester.Args("ZENLAYERCDN_")
	fTestCertPath      string
	fTestKeyPath       string
	fAccessKeyId       string
	fAccessKeyPassword string
	fDomain            string
	fCertificateId     string
)

func init() {
	fp.DefineString(&fTestCertPath, "TESTCERTPATH")
	fp.DefineString(&fTestKeyPath, "TESTKEYPATH")
	fp.DefineString(&fAccessKeyId, "ACCESSKEYID")
	fp.DefineString(&fAccessKeyPassword, "ACCESSKEYPASSWORD")
	fp.DefineString(&fDomain, "DOMAIN")
	fp.DefineString(&fCertificateId, "CERTIFICATEID")
}

type envDeployTester struct {
	impl               *zenlayerCdnDeployer
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
	if fAccessKeyId == "" || fAccessKeyPassword == "" || fTestCertPath == "" || fTestKeyPath == "" {
		t.Skip("requires ZENLAYERCDN_ credentials and test cert paths")
	}

	accessConfigJSON, err := json.Marshal(map[string]string{
		"accessKeyId":       fAccessKeyId,
		"accessKeyPassword": fAccessKeyPassword,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Deploy_ToDomain", func(t *testing.T) {
		if fDomain == "" {
			t.Skip("requires ZENLAYERCDN_DOMAIN")
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
			impl:               &zenlayerCdnDeployer{},
			accessConfigJSON:   string(accessConfigJSON),
			extendedConfigJSON: string(extendedConfigJSON),
		}, tester.TestDeployArgs{CertPath: fTestCertPath, KeyPath: fTestKeyPath})
	})

	t.Run("Deploy_ToCertificate", func(t *testing.T) {
		if fCertificateId == "" {
			t.Skip("requires ZENLAYERCDN_CERTIFICATEID")
		}

		extendedConfigJSON, err := json.Marshal(map[string]any{
			"deployTarget":  deployTargetCertificate,
			"certificateId": fCertificateId,
		})
		if err != nil {
			t.Fatal(err)
		}

		tester.TestDeploy(t, &envDeployTester{
			impl:               &zenlayerCdnDeployer{},
			accessConfigJSON:   string(accessConfigJSON),
			extendedConfigJSON: string(extendedConfigJSON),
		}, tester.TestDeployArgs{CertPath: fTestCertPath, KeyPath: fTestKeyPath})
	})
}
