//go:build tester

package upyuncertmgr_test

import (
	"testing"

	impl "github.com/certimate-go/plugins/internal/upyuncertmgr"
	tester "github.com/certimate-go/certimate/pkg/core/certmgr/providers-tester"
)

var (
	fp            = tester.InitArgs("UPYUNSSL_")
	fTestCertPath string
	fTestKeyPath  string
	fUsername     string
	fPassword     string
)

func init() {
	fp.DefineString(&fTestCertPath, "TESTCERTPATH")
	fp.DefineString(&fTestKeyPath, "TESTKEYPATH")
	fp.DefineString(&fUsername, "USERNAME")
	fp.DefineString(&fPassword, "PASSWORD")
}

/*
Shell command to run this test:

	go test -v ./upyun_ssl_test.go -args \
		--UPYUNSSL_TESTCERTPATH="/path/to/your-test-cert.pem" \
		--UPYUNSSL_TESTKEYPATH="/path/to/your-test-key.pem" \
		--UPYUNSSL_USERNAME="your-username" \
		--UPYUNSSL_PASSWORD="your-password"
*/
func TestProvider(t *testing.T) {
	fp.Parse()
	if fUsername == "" || fPassword == "" || fTestCertPath == "" || fTestKeyPath == "" {
		t.Skip("requires upyun credentials and test cert paths; see file comment")
	}

	t.Run("Upload", func(t *testing.T) {
		provider, err := impl.NewCertmgr(&impl.CertmgrConfig{
			Username: fUsername,
			Password: fPassword,
		})
		if err != nil {
			t.Errorf("err: %+v", err)
			return
		}

		tester.Upload(t, provider, tester.UploadInput{CertPath: fTestCertPath, KeyPath: fTestKeyPath})
	})
}
