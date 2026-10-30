// Package upnp 实现 UPnP IGD 端口映射（SSDP 发现 + SOAP 控制）与 NAT-PMP 端口映射。
package upnp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const ssdpAddr = "239.255.255.250:1900"

// 支持的 WAN 连接服务类型，按优先级排列
var serviceTypes = []string{
	"urn:schemas-upnp-org:service:WANIPConnection:2",
	"urn:schemas-upnp-org:service:WANIPConnection:1",
	"urn:schemas-upnp-org:service:WANPPPConnection:1",
}

// Client 是一个已发现的 IGD 控制端点。
type Client struct {
	ControlURL  string
	ServiceType string
	// LocalIP 是本机在网关所在网段的地址，作为端口映射的内部地址
	LocalIP string
	hc      *http.Client
}

// Discover 通过 SSDP 发现网关。gatewayIP 非空时同时向网关单播，适配禁用组播的网络。
func Discover(ctx context.Context, gatewayIP string) (*Client, error) {
	pc, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, err
	}
	defer pc.Close()
	req := "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\n" +
		"ST: urn:schemas-upnp-org:device:InternetGatewayDevice:1\r\n\r\n"
	targets := []string{ssdpAddr}
	if gatewayIP != "" {
		targets = append(targets, net.JoinHostPort(gatewayIP, "1900"))
	}
	for _, t := range targets {
		ua, err := net.ResolveUDPAddr("udp4", t)
		if err == nil {
			_, _ = pc.WriteTo([]byte(req), ua)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = pc.SetReadDeadline(deadline)
	buf := make([]byte, 4096)
	var lastErr error
	for {
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, errors.New("未发现支持 UPnP 的网关，请确认路由器已开启 UPnP")
		}
		// 指定了网关时只接受该网关的应答
		if gatewayIP != "" && !strings.HasPrefix(from.String(), gatewayIP+":") {
			continue
		}
		loc, err := parseSSDPLocation(buf[:n])
		if err != nil {
			lastErr = err
			continue
		}
		// LOCATION 必须指向应答设备自身，否则局域网内的恶意主机可借此让本机向任意地址发请求
		if u, perr := url.Parse(loc); perr != nil || u.Hostname() != fromHost(from) {
			lastErr = errors.New("SSDP 应答的 LOCATION 与应答设备地址不一致，已忽略")
			continue
		}
		c, err := FromDescription(ctx, loc)
		if err != nil {
			lastErr = err
			continue
		}
		return c, nil
	}
}

// fromHost 返回 UDP 应答来源的主机部分（不含端口与 IPv6 区域标识）。
func fromHost(a net.Addr) string {
	h, _, err := net.SplitHostPort(a.String())
	if err != nil {
		return a.String()
	}
	if i := strings.IndexByte(h, '%'); i >= 0 {
		h = h[:i]
	}
	return h
}

// parseSSDPLocation 从 SSDP 响应中取 LOCATION 头。
func parseSSDPLocation(b []byte) (string, error) {
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(b)), nil)
	if err != nil {
		return "", fmt.Errorf("SSDP 响应格式错误: %w", err)
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", errors.New("SSDP 响应缺少 LOCATION")
	}
	return loc, nil
}

type deviceDesc struct {
	URLBase string `xml:"URLBase"`
	Device  device `xml:"device"`
}

type device struct {
	Services []struct {
		ServiceType string `xml:"serviceType"`
		ControlURL  string `xml:"controlURL"`
	} `xml:"serviceList>service"`
	Devices []device `xml:"deviceList>device"`
}

// findService 在设备树中按优先级查找 WAN 连接服务。
func (d device) findService(st string) string {
	for _, s := range d.Services {
		if s.ServiceType == st {
			return s.ControlURL
		}
	}
	for _, sub := range d.Devices {
		if u := sub.findService(st); u != "" {
			return u
		}
	}
	return ""
}

// FromDescription 读取设备描述 XML 并定位控制地址。
func FromDescription(ctx context.Context, location string) (*Client, error) {
	hc := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("读取网关描述失败: %w", err)
	}
	defer resp.Body.Close()
	var desc deviceDesc
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&desc); err != nil {
		return nil, fmt.Errorf("解析网关描述失败: %w", err)
	}
	base, _ := url.Parse(location)
	if desc.URLBase != "" {
		if b, err := url.Parse(desc.URLBase); err == nil {
			base = b
		}
	}
	for _, st := range serviceTypes {
		if cu := desc.Device.findService(st); cu != "" {
			ref, err := url.Parse(cu)
			if err != nil {
				continue
			}
			ctrl := base.ResolveReference(ref)
			// 控制地址与描述地址必须是同一主机，防止描述文件里的 URLBase 或绝对 controlURL 指向其他内网服务
			if lu, perr := url.Parse(location); perr == nil && ctrl.Hostname() != lu.Hostname() {
				continue
			}
			c := New(ctrl.String(), st)
			c.LocalIP = localIPFor(base.Hostname())
			return c, nil
		}
	}
	return nil, errors.New("网关描述中没有 WANIPConnection/WANPPPConnection 服务")
}

// New 直接使用已知的控制地址创建客户端（Docker 等无法组播发现的环境）。
func New(controlURL, serviceType string) *Client {
	if serviceType == "" {
		serviceType = serviceTypes[1]
	}
	c := &Client{ControlURL: controlURL, ServiceType: serviceType, hc: &http.Client{Timeout: 8 * time.Second}}
	if u, err := url.Parse(controlURL); err == nil {
		c.LocalIP = localIPFor(u.Hostname())
	}
	return c
}

// localIPFor 返回访问 host 时使用的本机地址（UDP 连接不发包，只做路由选择）。
func localIPFor(host string) string {
	c, err := net.Dial("udp4", net.JoinHostPort(host, "1900"))
	if err != nil {
		return ""
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).IP.String()
}

// SOAPError 是网关返回的 UPnP 错误。
type SOAPError struct {
	Code        int
	Description string
}

func (e *SOAPError) Error() string {
	return fmt.Sprintf("UPnP 错误 %d: %s", e.Code, e.Description)
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func (c *Client) soap(ctx context.Context, action string, args [][2]string) ([]byte, error) {
	var body strings.Builder
	body.WriteString(`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>`)
	body.WriteString(`<u:` + action + ` xmlns:u="` + c.ServiceType + `">`)
	for _, a := range args {
		body.WriteString("<" + a[0] + ">" + xmlEscape(a[1]) + "</" + a[0] + ">")
	}
	body.WriteString(`</u:` + action + `></s:Body></s:Envelope>`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ControlURL, strings.NewReader(body.String()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", `"`+c.ServiceType+"#"+action+`"`)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		var fault struct {
			Code int    `xml:"Body>Fault>detail>UPnPError>errorCode"`
			Desc string `xml:"Body>Fault>detail>UPnPError>errorDescription"`
		}
		if xml.Unmarshal(data, &fault) == nil && fault.Code != 0 {
			return nil, &SOAPError{Code: fault.Code, Description: fault.Desc}
		}
		return nil, fmt.Errorf("UPnP %s 返回 HTTP %d", action, resp.StatusCode)
	}
	return data, nil
}

// AddPortMapping 添加端口映射。lease 为租期秒数，0 表示永久；
// 网关只支持永久租期（错误 725）时自动改用 0。
func (c *Client) AddPortMapping(ctx context.Context, proto string, extPort int, intClient string, intPort int, desc string, lease int) (int, error) {
	if intClient == "" {
		intClient = c.LocalIP
	}
	args := func(l int) [][2]string {
		return [][2]string{
			{"NewRemoteHost", ""},
			{"NewExternalPort", strconv.Itoa(extPort)},
			{"NewProtocol", strings.ToUpper(proto)},
			{"NewInternalPort", strconv.Itoa(intPort)},
			{"NewInternalClient", intClient},
			{"NewEnabled", "1"},
			{"NewPortMappingDescription", desc},
			{"NewLeaseDuration", strconv.Itoa(l)},
		}
	}
	_, err := c.soap(ctx, "AddPortMapping", args(lease))
	var se *SOAPError
	if errors.As(err, &se) && se.Code == 725 && lease != 0 {
		_, err = c.soap(ctx, "AddPortMapping", args(0))
		return 0, err
	}
	return lease, err
}

// DeletePortMapping 删除端口映射。
func (c *Client) DeletePortMapping(ctx context.Context, proto string, extPort int) error {
	_, err := c.soap(ctx, "DeletePortMapping", [][2]string{
		{"NewRemoteHost", ""},
		{"NewExternalPort", strconv.Itoa(extPort)},
		{"NewProtocol", strings.ToUpper(proto)},
	})
	return err
}

// ExternalIP 查询网关 WAN 口地址。
func (c *Client) ExternalIP(ctx context.Context) (string, error) {
	data, err := c.soap(ctx, "GetExternalIPAddress", nil)
	if err != nil {
		return "", err
	}
	var r struct {
		IP string `xml:"Body>GetExternalIPAddressResponse>NewExternalIPAddress"`
	}
	if err := xml.Unmarshal(data, &r); err != nil {
		return "", err
	}
	return r.IP, nil
}
