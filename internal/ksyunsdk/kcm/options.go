package kcm

import (
	common "github.com/certimate-go/plugins/internal/ksyunsdk/zz-shared-common"
)

func WithAkSk(ak, sk string) common.OptionsFunc {
	return common.WithAkSk(ak, sk)
}
