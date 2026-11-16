package smb

import (
	"fmt"
	"strings"

	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/gosamba/userdb"
)

// account 是 SMB 服务实际使用的一个账号：本地用户，或开启共享用户后的文件服务共享用户。
type account struct {
	User
	// Shared 表示来自文件服务共享用户
	Shared bool
}

// 共享用户无法登录 SMB 的原因，界面按代码显示对应语言的说明。
const (
	IssueName   = "name"   // 用户名含有 SMB 不允许的字符
	IssueBcrypt = "bcrypt" // 密码是 bcrypt 哈希，NTLM 需要明文
	IssueShare  = "share"  // 某个目录的名称不能作为共享名或重复
)

// Issue 是一个无法登录 SMB 的共享用户及原因代码，显示在共享用户页面、SMB 页面与运行日志中。
type Issue = fileusers.Issue

// issueLogText 返回写入运行日志的说明。
func issueLogText(i Issue) string {
	switch i.Reason {
	case IssueName:
		return fmt.Sprintf("共享用户 %s 的用户名不能用于 SMB，已跳过", i.Name)
	case IssueBcrypt:
		return fmt.Sprintf("共享用户 %s 的密码是 bcrypt 哈希，SMB 需要明文密码，已跳过", i.Name)
	default:
		return fmt.Sprintf("共享用户 %s 的目录名称不能作为 SMB 共享名或重复，已跳过", i.Name)
	}
}

// sharedIssue 检查共享用户能否用于 SMB：共享用户的通用校验比 SMB 宽松，用户名、共享名的字符与 bcrypt 密码需要在这里排除。
func sharedIssue(u fileusers.User) string {
	if !validName(u.Name) {
		return IssueName
	}
	// NTLM 认证需要由明文密码计算 NT 哈希，bcrypt 哈希无法使用
	if strings.HasPrefix(u.Password, "$2") && len(u.Password) == 60 {
		return IssueBcrypt
	}
	names := map[string]bool{}
	for _, mt := range u.Mounts {
		if !validName(mt.Name) || names[strings.ToLower(mt.Name)] {
			return IssueShare
		}
		names[strings.ToLower(mt.Name)] = true
	}
	return ""
}

// buildAccounts 生成 SMB 账号：先是本地用户，开启共享用户时再加入共享用户。
// 与本地用户同名（不区分大小写）的共享用户只按本地用户校验，不列为问题；无法用于 SMB 的共享用户列在 issues 中。
func buildAccounts(s Settings, shared []fileusers.User) (out []account, issues []Issue) {
	local := map[string]bool{}
	for _, u := range s.Users {
		// 与配置目录重叠的目录不提供，见 usable
		out = append(out, account{User: usable(u, nil)})
		local[strings.ToLower(u.Name)] = true
	}
	if !s.SharedUsers {
		return out, nil
	}
	for _, su := range shared {
		if local[strings.ToLower(su.Name)] {
			continue
		}
		if reason := sharedIssue(su); reason != "" {
			issues = append(issues, Issue{Name: su.Name, Reason: reason})
			continue
		}
		out = append(out, account{User: User(su), Shared: true})
	}
	return out, issues
}

// accountsKey 返回账号集合的指纹，共享用户变化后只有指纹不同时才需要重启服务。
func accountsKey(accounts []account) string {
	var b strings.Builder
	for _, a := range accounts {
		nt := userdb.NTHash(a.Password)
		fmt.Fprintf(&b, "%s\x00%x\x00", a.Name, nt)
		for _, sh := range a.Shares() {
			fmt.Fprintf(&b, "%s\x01%s\x01%t\x00", sh.Name, sh.Path, sh.ReadOnly)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
