package cms

import (
	common "github.com/certimate-go/plugins/internal/cloudsdk/ctyun/zz-shared-common"
)

func WithAkSk(ak, sk string) common.OptionsFunc {
	return common.WithAkSk(ak, sk)
}
