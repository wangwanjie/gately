package auth

import "strings"

// commonPasswordList 是常见弱口令（小写，空白分隔）：公开泄露口令榜单的高频项加上中文用户常用的拼音与数字组合。
// 完整匹配之外，isCommonPassword 还会去掉首尾数字符号、还原 p@ssw0rd 式替换后再比对，
// 因此这里只需要收录基本形态，列表保持精简以免增大二进制。
const commonPasswordList = `
password passw0rd passwd p@ssword p@ssw0rd pass1234 password1 password12 password123 password1234 password12345
admin admin123 admin1234 admin12345 admin123456 administrator root root123 toor user guest test test123 test1234 testing
qwerty qwerty123 qwerty1234 qwerty12345 qwertyuiop qwertyuiop123 qwerasdf qwer1234 qwer12345 qwe123456 qazwsx qazwsxedc qazwsx123
1qaz2wsx 1qaz2wsx3edc 1q2w3e4r 1q2w3e4r5t 1q2w3e 1q2w3e4r5t6y zaq12wsx zaq1xsw2 q1w2e3r4 q1w2e3r4t5 asdf1234 asdfghjkl asdfgh asdf123 asdfasdf
zxcvbnm zxcv1234 zxcvbnm123 abcd1234 abcd123456 abcdefg abcdefgh abc123 abc12345 abc123456 abcabc123 a123456 a1234567 a12345678 a123456789 aa123456 aaa111 aa123456789
letmein welcome welcome1 welcome123 iloveyou iloveyou123 love1234 monkey dragon master sunshine princess football baseball superman trustno1 shadow michael jennifer
login login123 secret secret123 default changeme changeit changeme123 temp temp123 demo demo123 1234 12345 123456 1234567 12345678 123456789 1234567890 12345678910
0987654321 987654321 9876543210 11111111 111111111 1111111111 00000000 000000000 0000000000 88888888 888888888 66666666 666666666 123123 123123123 123321 123321123 112233 112233445566
147258369 147258 159357 159753 1357924680 13579 24680 123654 654321 6543210 12341234 1234abcd 123abc 123abc123 12345qwert 12345abc 1234qwer 1234qwerty
5201314 520520 5201314520 1314520 13145200 woaini woaini1314 woaini520 woaini123 woainima woainima123 aini1314 iloveyou1314 wodemima wodemima123 mima123 mima1234 mima123456
wangwang nihao nihao123 nihao1234 zhongguo zhongguo123 china china123 china1234 beijing shanghai guangzhou shenzhen
qq123456 qq1234567 qq12345678 qq123456789 qqqqqq qqqqqqqq q123456 q1234567 q12345678 wechat weixin weixin123 taobao taobao123 baidu baidu123
000000 0000 111111 222222 333333 444444 555555 666666 777777 888888 999999 121212 131313 123456a 123456q 123456aa 123456abc a123123 a1b2c3 a1b2c3d4 a1b2c3d4e5
football1 baseball1 superman1 batman batman123 starwars pokemon pokemon123 minecraft minecraft123 fuckyou fuckyou123 whatever whatever1 freedom hello hello123 hello1234
internet internet123 computer computer123 server server123 server1234 router router123 openwrt openwrt123 synology synology123 docker docker123 linux linux123 ubuntu ubuntu123 debian debian123
nas nas123 fnos fnos123 feiniu feiniu123 unraid unraid123 truenas truenas123 proxmox proxmox123 pve123 homelab homelab123 mikrotik mikrotik123 ikuai ikuai123 lucky lucky123 lucky666 luckyadmin
google google123 apple apple123 microsoft microsoft123 amazon amazon123 github github123 gitlab gitlab123
january february march april june july august september october november december summer winter spring autumn
2020 2021 2022 2023 2024 2025 2026 2027 20202020 20212021 20222022 20232023 20242024 20252025 20262026
`

// commonWords 是可作为“前缀/后缀 + 少量数字”出现的常见口令词（5 个字符及以上，避免误伤正常口令）。
var commonWords = map[string]struct{}{}

// commonPasswords 是常见口令全集。
var commonPasswords = map[string]struct{}{}

func init() {
	for _, w := range strings.Fields(commonPasswordList) {
		commonPasswords[w] = struct{}{}
	}
	for _, w := range []string{
		"password", "passwd", "admin", "administrator", "qwerty", "qwertyuiop", "letmein", "welcome", "iloveyou", "monkey", "dragon", "master",
		"sunshine", "princess", "football", "baseball", "superman", "trustno", "shadow", "michael", "jennifer", "secret", "changeme", "woaini",
		"wodemima", "zhongguo", "synology", "openwrt", "docker", "linux", "ubuntu", "server", "router", "computer", "internet", "freedom", "hello123",
	} {
		if len(w) >= 5 {
			commonWords[w] = struct{}{}
		}
	}
}
