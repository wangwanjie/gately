package ssl

import (
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/miekg/dns"
)

// cnameZone 是带 CNAME 的假 DNS：cnames 为显式 CNAME，wildcard 非空时区域内其他不存在的名称都合成指向它的 CNAME，
// txt 记录写入后对应名称真实存在，不再匹配泛解析。
type cnameZone struct {
	zone     string
	wildcard string
	cnames   map[string]string
	mu       sync.Mutex
	txt      map[string]string
}

func (z *cnameZone) setTXT(name, v string) {
	z.mu.Lock()
	if z.txt == nil {
		z.txt = make(map[string]string)
	}
	z.txt[name] = v
	z.mu.Unlock()
}

func (z *cnameZone) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(req)
	q := req.Question[0]
	name := strings.ToLower(q.Name)
	hdr := func(t uint16) dns.RR_Header {
		return dns.RR_Header{Name: name, Rrtype: t, Class: dns.ClassINET, Ttl: 60}
	}
	soa := &dns.SOA{Hdr: dns.RR_Header{Name: z.zone, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 60}, Ns: "ns." + z.zone, Mbox: "admin." + z.zone, Minttl: 60}
	z.mu.Lock()
	txt, hasTXT := z.txt[name]
	z.mu.Unlock()
	switch {
	case q.Qtype == dns.TypeSOA && name == z.zone:
		m.Answer = append(m.Answer, soa)
	case q.Qtype == dns.TypeNS && name == z.zone:
		m.Answer = append(m.Answer, &dns.NS{Hdr: dns.RR_Header{Name: z.zone, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 60}, Ns: "ns." + z.zone})
	case q.Qtype == dns.TypeA && name == "ns."+z.zone:
		m.Answer = append(m.Answer, &dns.A{Hdr: hdr(dns.TypeA), A: net.ParseIP("127.0.0.1")})
	case hasTXT:
		if q.Qtype == dns.TypeTXT {
			m.Answer = append(m.Answer, &dns.TXT{Hdr: hdr(dns.TypeTXT), Txt: []string{txt}})
		} else {
			m.Ns = append(m.Ns, soa)
		}
	case z.cnames[name] != "":
		m.Answer = append(m.Answer, &dns.CNAME{Hdr: hdr(dns.TypeCNAME), Target: z.cnames[name]})
	case z.wildcard != "" && strings.HasSuffix(name, "."+z.zone):
		m.Answer = append(m.Answer, &dns.CNAME{Hdr: hdr(dns.TypeCNAME), Target: z.wildcard})
	case strings.HasSuffix(name, "."+z.zone):
		m.Ns = append(m.Ns, soa)
	default:
		m.Rcode = dns.RcodeRefused
	}
	_ = w.WriteMsg(m)
}

// 显式 CNAME 委派照常跟随；来自泛解析的 CNAME 与关闭跟随时验证记录写在 _acme-challenge 本身（issue #61）
func TestDNSSolverCNAMETarget(t *testing.T) {
	z := &cnameZone{
		zone: "cname.example.",
		cnames: map[string]string{
			"_acme-challenge.deleg.cname.example.": "step.cname.example.",
			"step.cname.example.":                  "_acme-challenge.other.example.",
		},
	}
	addr := startFakeDNS(t, "127.0.0.1:0", z)
	wild := &cnameZone{zone: "wild.example.", wildcard: "wild.example."}
	wildAddr := startFakeDNS(t, "127.0.0.1:0", wild)

	cases := []struct {
		name, server, domain string
		disable              bool
		want                 challengeTarget
	}{
		{"无 CNAME", addr, "plain.cname.example", false, challengeTarget{fqdn: "_acme-challenge.plain.cname.example."}},
		{"跟随委派链", addr, "deleg.cname.example", false, challengeTarget{fqdn: "_acme-challenge.other.example."}},
		{"关闭跟随", addr, "deleg.cname.example", true, challengeTarget{fqdn: "_acme-challenge.deleg.cname.example.", noFollow: true}},
		{"泛解析不跟随", wildAddr, "wild.example", false, challengeTarget{fqdn: "_acme-challenge.wild.example.", noFollow: true}},
		{"通配符域名", wildAddr, "*.wild.example", false, challengeTarget{fqdn: "_acme-challenge.wild.example.", noFollow: true}},
	}
	for _, c := range cases {
		s := &dnsSolver{nameservers: []string{c.server}, disableCNAME: c.disable}
		if got := s.target(c.domain); got != c.want {
			t.Fatalf("%s: 得到 %+v，期望 %+v", c.name, got, c.want)
		}
	}

	// 泛解析下验证记录定位到 _acme-challenge 主机记录，而不是 @
	s := &dnsSolver{zone: "wild.example", nameservers: []string{wildAddr}}
	zone, sub, _, err := s.locate("wild.example", "key")
	if err != nil || zone != "wild.example" || sub != "_acme-challenge" {
		t.Fatalf("得到 %s / %s / %v", zone, sub, err)
	}
}

// 不跟随 CNAME 时生效检查直接查 _acme-challenge 本身，不被泛解析的 CNAME 带到别的名称
func TestNetPreCheckNoCNAME(t *testing.T) {
	z := &cnameZone{zone: "wild.example.", wildcard: "wild.example."}
	addr := startFakeDNS(t, "127.0.0.1:0", z)
	useAuthoritativePort(t, addr)
	fqdn := "_acme-challenge.wild.example."
	// 区域顶点上的同值 TXT 不能让检查误判为已生效
	z.setTXT("wild.example.", "token")
	pc := netPreCheck{nameservers: []string{addr}, authoritative: true, noCNAME: true}
	if ok, err := pc.check("", fqdn, "token", nil); ok || err != nil {
		t.Fatalf("_acme-challenge 未写入时应返回未通过: %v %v", ok, err)
	}
	z.setTXT(fqdn, "token")
	if ok, err := pc.check("", fqdn, "token", nil); !ok || err != nil {
		t.Fatalf("写入后应通过: %v %v", ok, err)
	}
}
