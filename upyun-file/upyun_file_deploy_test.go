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
	fp            = tester.Args("UPYUNFILE_")
	fTestCertPath string
	fTestKeyPath  string
	fUsername     string
	fPassword     string
	fBucket       string
	fDomain       string
)

func init() {
	fp.DefineString(&fTestCertPath, "TESTCERTPATH")
	fp.DefineString(&fTestKeyPath, "TESTKEYPATH")
	fp.DefineString(&fUsername, "USERNAME")
	fp.DefineString(&fPassword, "PASSWORD")
	fp.DefineString(&fBucket, "BUCKET")
	fp.DefineString(&fDomain, "DOMAIN")
}

type envDeployTester struct {
	impl               *upyunFileDeployer
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
	if fUsername == "" || fPassword == "" || fTestCertPath == "" || fTestKeyPath == "" || fDomain == "" {
		t.Skip("requires UPYUNFILE_ credentials and test cert paths")
	}

	accessConfigJSON, err := json.Marshal(map[string]string{
		"username": fUsername,
		"password": fPassword,
	})
	if err != nil {
		t.Fatal(err)
	}

	extendedConfigJSON, err := json.Marshal(map[string]any{
		"bucket": fBucket,
		"domain": fDomain,
	})
	if err != nil {
		t.Fatal(err)
	}

	tester.TestDeploy(t, &envDeployTester{
		impl:               &upyunFileDeployer{},
		accessConfigJSON:   string(accessConfigJSON),
		extendedConfigJSON: string(extendedConfigJSON),
	}, tester.TestDeployArgs{CertPath: fTestCertPath, KeyPath: fTestKeyPath})
}
