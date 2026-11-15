package dnsprovider

import (
	"strings"

	"golang.org/x/net/publicsuffix"
)

// 常见的多级公共后缀。完整的公共后缀表过大，这里覆盖国内外常用的情况；
// 无法正确拆分时，可在配置中显式指定主域名。
var multiLabelSuffixes = map[string]bool{
	"com.cn": true, "net.cn": true, "org.cn": true, "gov.cn": true, "edu.cn": true, "ac.cn": true,
	"com.hk": true, "com.tw": true, "com.mo": true, "com.sg": true, "com.my": true,
	"co.uk": true, "org.uk": true, "co.jp": true, "ne.jp": true, "or.jp": true, "co.kr": true,
	"com.au": true, "net.au": true, "co.nz": true, "co.za": true, "com.br": true, "com.mx": true,
	"co.in": true, "co.id": true, "co.th": true, "com.tr": true, "eu.org": true, "us.org": true,
	"cn.com": true, "uk.com": true, "us.com": true,
}

// SplitFQDN 把完整域名拆成主机记录与主域名。
// 支持 "sub:example.com" 显式写法；否则取最后两段为主域名，常见多级后缀取最后三段，其余多级后缀按公共后缀表识别。
// 主域名本身返回主机记录 "@"。
func SplitFQDN(full string) (sub, zone string) {
	full = strings.ToLower(strings.Trim(strings.TrimSpace(full), "."))
	if s, z, ok := strings.Cut(full, ":"); ok {
		return s, z
	}
	labels := strings.Split(full, ".")
	n := 2
	if len(labels) >= 3 && multiLabelSuffixes[strings.Join(labels[len(labels)-2:], ".")] {
		n = 3
	} else if etld1, err := publicsuffix.EffectiveTLDPlusOne(full); err == nil {
		// 内置表之外的多级后缀（如 com.ar、org.br）按公共后缀表（仅 ICANN 部分）取注册域名；
		// 私有后缀（如 dynv6.net、github.io）保持原有的取最后两段，用户可能拥有的是其父域名下的主域名
		if suffix, icann := publicsuffix.PublicSuffix(full); icann && suffix != "" {
			n = len(strings.Split(etld1, "."))
		}
	}
	if len(labels) <= n {
		return "@", full
	}
	return strings.Join(labels[:len(labels)-n], "."), strings.Join(labels[len(labels)-n:], ".")
}

// SubForZone 返回 fqdn 在给定主域名下的主机记录；fqdn 不属于该主域名时返回 false。
func SubForZone(fqdn, zone string) (string, bool) {
	fqdn = strings.ToLower(strings.Trim(fqdn, "."))
	zone = strings.ToLower(strings.Trim(zone, "."))
	if fqdn == zone {
		return "@", true
	}
	if strings.HasSuffix(fqdn, "."+zone) {
		return strings.TrimSuffix(fqdn, "."+zone), true
	}
	return "", false
}
