package qiniusslcert_test

import (
	"testing"

	impl "github.com/certimate-go/plugins/internal/qiniusslcert"
	tester "github.com/certimate-go/certimate/pkg/core/certmgr/testing"
)

var (
	fp            = tester.Args("QINIUSSLCERT_")
	fTestCertPath string
	fTestKeyPath  string
	fAccessKey    string
	fSecretKey    string
)

func init() {
	fp.DefineString(&fTestCertPath, "TESTCERTPATH")
	fp.DefineString(&fTestKeyPath, "TESTKEYPATH")
	fp.DefineString(&fAccessKey, "ACCESSKEY")
	fp.DefineString(&fSecretKey, "SECRETKEY")
}

/*
Shell command to run this test:

	go test -v ./qiniu_sslcert_test.go -args \
	--QINIUSSLCERT_TESTCERTPATH="/path/to/your-test-cert.pem" \
	--QINIUSSLCERT_TESTKEYPATH="/path/to/your-test-key.pem" \
	--QINIUSSLCERT_ACCESSKEY="your-access-key" \
	--QINIUSSLCERT_SECRETKEY="your-secret-key"
*/
func TestProvider(t *testing.T) {
	fp.Parse()
	if fAccessKey == "" || fSecretKey == "" || fTestCertPath == "" || fTestKeyPath == "" {
		t.Skip("requires qiniu credentials and test cert paths; see file comment")
	}

	t.Run("Upload", func(t *testing.T) {
		provider, err := impl.NewCertmgr(&impl.CertmgrConfig{
			AccessKey: fAccessKey,
			SecretKey: fSecretKey,
		})
		if err != nil {
			t.Errorf("err: %+v", err)
			return
		}

		tester.TestUpload(t, provider, tester.TestUploadArgs{CertPath: fTestCertPath, KeyPath: fTestKeyPath})
	})
}
