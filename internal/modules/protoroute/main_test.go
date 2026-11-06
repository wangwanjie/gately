package protoroute

import (
	"os"
	"testing"

	"github.com/wangwanjie/gately/internal/fwopen"
)

func TestMain(m *testing.M) {
	// 自动放行防火墙的测试不能改动宿主机防火墙
	fwopen.ForTesting()
	os.Exit(m.Run())
}
