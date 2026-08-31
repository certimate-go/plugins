package baishancdn_test

import (
	"testing"

	tester "github.com/certimate-go/certimate/pkg/core/certmgr/testing"
	impl "github.com/certimate-go/plugins/internal/baishancertmgr"
)

var (
	fp            = tester.Args("BAISHANCDN_")
	fTestCertPath string
	fTestKeyPath  string
	fApiToken     string
)

func init() {
	fp.DefineString(&fTestCertPath, "TESTCERTPATH")
	fp.DefineString(&fTestKeyPath, "TESTKEYPATH")
	fp.DefineString(&fApiToken, "APITOKEN")
}

func TestProvider(t *testing.T) {
	fp.Parse()
	if fApiToken == "" || fTestCertPath == "" || fTestKeyPath == "" {
		t.Skip("requires baishan credentials and test cert paths")
	}

	t.Run("Upload", func(t *testing.T) {
		provider, err := impl.NewCertmgr(&impl.CertmgrConfig{
			ApiToken: fApiToken,
		})
		if err != nil {
			t.Errorf("err: %+v", err)
			return
		}

		tester.TestUpload(t, provider, tester.TestUploadArgs{CertPath: fTestCertPath, KeyPath: fTestKeyPath})
	})
}
