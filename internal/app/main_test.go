package app

import (
	"os"
	"testing"

	"github.com/wangwanjie/gately/internal/auth"
	"golang.org/x/crypto/bcrypt"
)

func TestMain(m *testing.M) {
	// 包内几百个用例各自建账号、改密码、登录，-race 下默认代价的 bcrypt 让整包测试逼近 10 分钟的超时
	auth.SetPasswordCostForTesting(bcrypt.MinCost)
	os.Exit(m.Run())
}
