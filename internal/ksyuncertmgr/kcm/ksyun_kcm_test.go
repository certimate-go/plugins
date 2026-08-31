package ksyunkcm_test

import (
	"testing"

	tester "github.com/certimate-go/certimate/pkg/core/certmgr/testing"
	impl "github.com/certimate-go/plugins/internal/ksyuncertmgr/kcm"
)

var (
	fp               = tester.Args("KSYUNKCM_")
	fTestCertPath    string
	fTestKeyPath     string
	fAccessKeyId     string
	fSecretAccessKey string
)

func init() {
	fp.DefineString(&fTestCertPath, "TESTCERTPATH")
	fp.DefineString(&fTestKeyPath, "TESTKEYPATH")
	fp.DefineString(&fAccessKeyId, "ACCESSKEYID")
	fp.DefineString(&fSecretAccessKey, "SECRETACCESSKEY")
}

func TestProvider(t *testing.T) {
	fp.Parse()
	if fAccessKeyId == "" || fSecretAccessKey == "" || fTestCertPath == "" || fTestKeyPath == "" {
		t.Skip("requires ksyun credentials and test cert paths")
	}

	t.Run("Upload", func(t *testing.T) {
		provider, err := impl.NewCertmgr(&impl.CertmgrConfig{
			AccessKeyId:     fAccessKeyId,
			SecretAccessKey: fSecretAccessKey,
		})
		if err != nil {
			t.Errorf("err: %+v", err)
			return
		}

		tester.TestUpload(t, provider, tester.TestUploadArgs{CertPath: fTestCertPath, KeyPath: fTestKeyPath})
	})
}
