package app

import (
	"os"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/auth"
	"golang.org/x/crypto/bcrypt"
)

func TestMain(m *testing.M) {
	// 包内几百个用例各自建账号、改密码、登录，-race 下默认代价的 bcrypt 让整包测试逼近 10 分钟的超时
	auth.SetPasswordCostForTesting(bcrypt.MinCost)
	// 导入或保存设置里的时区不改动进程的 time.Local：避免影响其后的用例，也避免与读取时间的协程竞争
	setLocalZone = func(*time.Location) {}
	os.Exit(m.Run())
}
