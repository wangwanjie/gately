package auth

import (
	"os"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestMain(m *testing.M) {
	// -race 下默认代价的 bcrypt 每次要数百毫秒，用最小代价缩短整包测试时间
	passwordCost = bcrypt.MinCost
	os.Exit(m.Run())
}
