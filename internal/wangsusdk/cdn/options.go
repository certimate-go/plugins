package cdn

import (
	common "github.com/certimate-go/plugins/internal/wangsusdk/zz-shared-common"
)

func WithAkSk(ak, sk string) common.OptionsFunc {
	return common.WithAkSk(ak, sk)
}
