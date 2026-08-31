package wangsucertificate_test

import (
	"testing"

	tester "github.com/certimate-go/certimate/pkg/core/certmgr/testing"
	impl "github.com/certimate-go/plugins/internal/wangsucertmgr"
)

var (
	fp               = tester.Args("WANGSUCERTIFICATE_")
	fTestCertPath    string
	fTestKeyPath     string
	fAccessKeyId     string
	fAccessKeySecret string
)

func init() {
	fp.DefineString(&fTestCertPath, "TESTCERTPATH")
	fp.DefineString(&fTestKeyPath, "TESTKEYPATH")
	fp.DefineString(&fAccessKeyId, "ACCESSKEYID")
	fp.DefineString(&fAccessKeySecret, "ACCESSKEYSECRET")
}

func TestProvider(t *testing.T) {
	fp.Parse()
	if fAccessKeyId == "" || fAccessKeySecret == "" || fTestCertPath == "" || fTestKeyPath == "" {
		t.Skip("requires wangsu credentials and test cert paths")
	}

	t.Run("Upload", func(t *testing.T) {
		provider, err := impl.NewCertmgr(&impl.CertmgrConfig{
			AccessKeyId:     fAccessKeyId,
			AccessKeySecret: fAccessKeySecret,
		})
		if err != nil {
			t.Errorf("err: %+v", err)
			return
		}

		tester.TestUpload(t, provider, tester.TestUploadArgs{CertPath: fTestCertPath, KeyPath: fTestKeyPath})
	})
}
