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
	fp                 = tester.Args("ZENLAYERGA_")
	fTestCertPath      string
	fTestKeyPath       string
	fAccessKeyId       string
	fAccessKeyPassword string
	fAcceleratorId     string
	fCertificateId     string
)

func init() {
	fp.DefineString(&fTestCertPath, "TESTCERTPATH")
	fp.DefineString(&fTestKeyPath, "TESTKEYPATH")
	fp.DefineString(&fAccessKeyId, "ACCESSKEYID")
	fp.DefineString(&fAccessKeyPassword, "ACCESSKEYPASSWORD")
	fp.DefineString(&fAcceleratorId, "ACCELERATORID")
	fp.DefineString(&fCertificateId, "CERTIFICATEID")
}

type envDeployTester struct {
	impl               *zenlayerGaDeployer
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
		t.Skip("requires ZENLAYERGA_ credentials and test cert paths")
	}

	accessConfigJSON, err := json.Marshal(map[string]string{
		"accessKeyId":       fAccessKeyId,
		"accessKeyPassword": fAccessKeyPassword,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Deploy_ToAccelerator", func(t *testing.T) {
		if fAcceleratorId == "" {
			t.Skip("requires ZENLAYERGA_ACCELERATORID")
		}

		extendedConfigJSON, err := json.Marshal(map[string]any{
			"deployTarget":  deployTargetAccelerator,
			"acceleratorId": fAcceleratorId,
		})
		if err != nil {
			t.Fatal(err)
		}

		tester.TestDeploy(t, &envDeployTester{
			impl:               &zenlayerGaDeployer{},
			accessConfigJSON:   string(accessConfigJSON),
			extendedConfigJSON: string(extendedConfigJSON),
		}, tester.TestDeployArgs{CertPath: fTestCertPath, KeyPath: fTestKeyPath})
	})

	t.Run("Deploy_ToCertificate", func(t *testing.T) {
		if fCertificateId == "" {
			t.Skip("requires ZENLAYERGA_CERTIFICATEID")
		}

		extendedConfigJSON, err := json.Marshal(map[string]any{
			"deployTarget":  deployTargetCertificate,
			"certificateId": fCertificateId,
		})
		if err != nil {
			t.Fatal(err)
		}

		tester.TestDeploy(t, &envDeployTester{
			impl:               &zenlayerGaDeployer{},
			accessConfigJSON:   string(accessConfigJSON),
			extendedConfigJSON: string(extendedConfigJSON),
		}, tester.TestDeployArgs{CertPath: fTestCertPath, KeyPath: fTestKeyPath})
	})
}
