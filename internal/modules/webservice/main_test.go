package webservice

import (
	"github.com/wangwanjie/gately/internal/fwopen"
	"os"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/wangwanjie/gately/internal/i18n"
)

// 测试断言基于简体中文文案，固定服务端语言避免受运行环境 LANG 影响。
func TestMain(m *testing.M) {
	i18n.SetServerLang("zh-CN")
	// 自动放行防火墙的测试不能改动宿主机防火墙
	fwopen.ForTesting()
	// 站点用户密码保存时会做 bcrypt，测试中用最低成本以免拖慢大量启动监听的用例
	siteUserCost = bcrypt.MinCost
	os.Exit(m.Run())
}
