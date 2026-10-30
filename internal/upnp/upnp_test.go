package upnp

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const descXML = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
 <device>
  <deviceType>urn:schemas-upnp-org:device:InternetGatewayDevice:1</deviceType>
  <deviceList><device>
   <deviceType>urn:schemas-upnp-org:device:WANDevice:1</deviceType>
   <deviceList><device>
    <deviceType>urn:schemas-upnp-org:device:WANConnectionDevice:1</deviceType>
    <serviceList><service>
     <serviceType>urn:schemas-upnp-org:service:WANIPConnection:1</serviceType>
     <controlURL>/ctl/IPConn</controlURL>
    </service></serviceList>
   </device></deviceList>
  </device></deviceList>
 </device>
</root>`

type fakeIGD struct {
	mu       sync.Mutex
	actions  []string
	bodies   []string
	only725  bool
	conflict bool
}

func (f *fakeIGD) handler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/rootDesc.xml" {
		_, _ = w.Write([]byte(descXML))
		return
	}
	b, _ := io.ReadAll(r.Body)
	action := r.Header.Get("SOAPAction")
	f.mu.Lock()
	f.actions = append(f.actions, action)
	f.bodies = append(f.bodies, string(b))
	f.mu.Unlock()
	fault := func(code int) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault><detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0"><errorCode>` +
			strconv.Itoa(code) + `</errorCode><errorDescription>err</errorDescription></UPnPError></detail></s:Fault></s:Body></s:Envelope>`))
	}
	switch {
	case strings.HasSuffix(action, `#AddPortMapping"`):
		if f.conflict {
			fault(718)
			return
		}
		if f.only725 && !strings.Contains(string(b), "<NewLeaseDuration>0</NewLeaseDuration>") {
			fault(725)
			return
		}
		_, _ = w.Write([]byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body/></s:Envelope>`))
	case strings.HasSuffix(action, `#GetExternalIPAddress"`):
		_, _ = w.Write([]byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:GetExternalIPAddressResponse xmlns:u="x"><NewExternalIPAddress>100.64.1.2</NewExternalIPAddress></u:GetExternalIPAddressResponse></s:Body></s:Envelope>`))
	default:
		_, _ = w.Write([]byte(`<ok/>`))
	}
}

func TestDescriptionAndSOAP(t *testing.T) {
	igd := &fakeIGD{}
	srv := httptest.NewServer(http.HandlerFunc(igd.handler))
	defer srv.Close()
	ctx := context.Background()

	c, err := FromDescription(ctx, srv.URL+"/rootDesc.xml")
	if err != nil {
		t.Fatal(err)
	}
	if c.ControlURL != srv.URL+"/ctl/IPConn" || c.ServiceType != "urn:schemas-upnp-org:service:WANIPConnection:1" {
		t.Fatalf("控制地址解析错误: %+v", c)
	}
	if c.LocalIP != "127.0.0.1" {
		t.Fatalf("本机地址应为 127.0.0.1，实际 %q", c.LocalIP)
	}
	lease, err := c.AddPortMapping(ctx, "tcp", 16789, "", 16789, "gately <test>", 3600)
	if err != nil || lease != 3600 {
		t.Fatalf("添加映射失败: %v", err)
	}
	body := igd.bodies[0]
	for _, want := range []string{"<NewExternalPort>16789</NewExternalPort>", "<NewProtocol>TCP</NewProtocol>", "<NewInternalClient>127.0.0.1</NewInternalClient>", "gately &lt;test&gt;"} {
		if !strings.Contains(body, want) {
			t.Errorf("SOAP 请求缺少 %s:\n%s", want, body)
		}
	}
	if ip, err := c.ExternalIP(ctx); err != nil || ip != "100.64.1.2" {
		t.Fatalf("查询外网地址失败: %q %v", ip, err)
	}
	if err := c.DeletePortMapping(ctx, "tcp", 16789); err != nil {
		t.Fatal(err)
	}

	// 只支持永久租期的网关
	igd.only725 = true
	lease, err = c.AddPortMapping(ctx, "udp", 1000, "192.168.1.2", 1000, "x", 3600)
	if err != nil || lease != 0 {
		t.Fatalf("725 回退失败: lease=%d %v", lease, err)
	}
	igd.conflict = true
	_, err = c.AddPortMapping(ctx, "tcp", 1000, "192.168.1.2", 1000, "x", 0)
	if se, ok := err.(*SOAPError); !ok || se.Code != 718 {
		t.Fatalf("应返回 718 错误: %v", err)
	}
}

func TestParseSSDP(t *testing.T) {
	resp := "HTTP/1.1 200 OK\r\nCACHE-CONTROL: max-age=120\r\nST: urn:schemas-upnp-org:device:InternetGatewayDevice:1\r\nLOCATION: http://192.168.1.1:5000/rootDesc.xml\r\n\r\n"
	loc, err := parseSSDPLocation([]byte(resp))
	if err != nil || loc != "http://192.168.1.1:5000/rootDesc.xml" {
		t.Fatalf("got %q %v", loc, err)
	}
	if _, err := parseSSDPLocation([]byte("HTTP/1.1 200 OK\r\n\r\n")); err == nil {
		t.Fatal("缺少 LOCATION 应报错")
	}
}

func TestNATPMP(t *testing.T) {
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	old := NATPMPPort
	NATPMPPort = pc.LocalAddr().(*net.UDPAddr).Port
	defer func() { NATPMPPort = old }()
	go func() {
		buf := make([]byte, 64)
		first := true
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			// 丢弃第一个包，验证重传
			if first {
				first = false
				continue
			}
			if n != 12 {
				continue
			}
			resp := make([]byte, 16)
			resp[1] = 128 + buf[1]
			binary.BigEndian.PutUint32(resp[4:], 1)
			copy(resp[8:10], buf[4:6])
			binary.BigEndian.PutUint16(resp[10:], binary.BigEndian.Uint16(buf[6:])+1)
			copy(resp[12:16], buf[8:12])
			_, _ = pc.WriteTo(resp, addr)
		}
	}()
	ext, life, err := NATPMPMap(context.Background(), "127.0.0.1", "tcp", 16789, 16789, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if ext != 16790 || life != time.Hour {
		t.Fatalf("映射结果错误: %d %v", ext, life)
	}
}

func TestNATPMPExternalIP(t *testing.T) {
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	old := NATPMPPort
	NATPMPPort = pc.LocalAddr().(*net.UDPAddr).Port
	defer func() { NATPMPPort = old }()
	go func() {
		buf := make([]byte, 64)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if n != 2 || buf[1] != 0 {
				continue
			}
			// 操作码 0 响应：版本、128、结果码、纪元秒、外网地址
			resp := make([]byte, 12)
			resp[1] = 128
			binary.BigEndian.PutUint32(resp[4:], 1)
			copy(resp[8:], net.ParseIP("100.70.13.98").To4())
			_, _ = pc.WriteTo(resp, addr)
		}
	}()
	ip, err := NATPMPExternalIP(context.Background(), "127.0.0.1")
	if err != nil || ip != "100.70.13.98" {
		t.Fatalf("外网地址错误: %q %v", ip, err)
	}
}
