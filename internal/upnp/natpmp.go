package upnp

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// NATPMPPort 是 NAT-PMP 服务端口（RFC 6886）。
var NATPMPPort = 5351

// NATPMPMap 请求网关映射端口，返回网关分配的外部端口与租期。
// lifetime 为 0 表示删除映射。
func NATPMPMap(ctx context.Context, gateway, proto string, intPort, extPort int, lifetime time.Duration) (int, time.Duration, error) {
	var op byte
	switch strings.ToLower(proto) {
	case "udp":
		op = 1
	case "tcp":
		op = 2
	default:
		return 0, 0, fmt.Errorf("不支持的协议 %q", proto)
	}
	req := make([]byte, 12)
	req[0] = 0
	req[1] = op
	binary.BigEndian.PutUint16(req[4:], uint16(intPort))
	binary.BigEndian.PutUint16(req[6:], uint16(extPort))
	binary.BigEndian.PutUint32(req[8:], uint32(lifetime/time.Second))
	buf, err := natpmpRequest(ctx, gateway, req, 16)
	if err != nil {
		return 0, 0, err
	}
	mapped := int(binary.BigEndian.Uint16(buf[10:]))
	life := time.Duration(binary.BigEndian.Uint32(buf[12:])) * time.Second
	return mapped, life, nil
}

// NATPMPExternalIP 查询网关的外网地址（RFC 6886 操作码 0）。
func NATPMPExternalIP(ctx context.Context, gateway string) (string, error) {
	buf, err := natpmpRequest(ctx, gateway, []byte{0, 0}, 12)
	if err != nil {
		return "", err
	}
	return net.IP(buf[8:12]).String(), nil
}

// natpmpRequest 发送请求并等待对应操作码的响应，响应至少 minLen 字节且结果码为 0。
func natpmpRequest(ctx context.Context, gateway string, req []byte, minLen int) ([]byte, error) {
	op := req[1]
	conn, err := net.Dial("udp4", net.JoinHostPort(gateway, fmt.Sprint(NATPMPPort)))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// RFC 6886 建议从 250ms 开始指数退避重传
	wait := 250 * time.Millisecond
	buf := make([]byte, 16)
	for attempt := 0; attempt < 5; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if _, err := conn.Write(req); err != nil {
			return nil, err
		}
		_ = conn.SetReadDeadline(time.Now().Add(wait))
		n, err := conn.Read(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				wait *= 2
				continue
			}
			return nil, err
		}
		if n < minLen || buf[0] != 0 || buf[1] != 128+op {
			continue
		}
		if code := binary.BigEndian.Uint16(buf[2:]); code != 0 {
			return nil, fmt.Errorf("NAT-PMP 错误码 %d", code)
		}
		return buf[:n], nil
	}
	return nil, errors.New("NAT-PMP 网关无响应，请确认路由器已开启 NAT-PMP")
}
