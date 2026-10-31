package protoroute

import (
	"bytes"
	"strings"
)

// 以文本请求行开头的协议：HTTP、RTSP、SIP 的请求行都是「方法 目标 版本」，方法名还有重叠（如 OPTIONS），
// 因此读完整个请求行，按最后的版本号区分；IRC 客户端先发送 CAP、NICK、PASS 或 USER 命令。

// maxRequestLine 是请求行的最大长度。超过仍没有换行时，方法名属于 HTTP 的按 HTTP 处理（超长 URL），其余判为否。
const maxRequestLine = 8192

// maxMethodLen 是方法名的最大长度（最长的 HTTP 方法 BASELINE-CONTROL 为 16 个字符）。
const maxMethodLen = 20

// httpMethods 是 HTTP 的方法名，包括 WebDAV/CalDAV 等扩展方法与 HTTP/2 明文直连的连接前言 PRI。
var httpMethods = []string{
	"GET", "POST", "HEAD", "PUT", "DELETE", "OPTIONS", "PATCH", "CONNECT", "TRACE", "PRI",
	"PROPFIND", "PROPPATCH", "MKCOL", "COPY", "MOVE", "LOCK", "UNLOCK", "REPORT", "SEARCH",
	"MKCALENDAR", "ACL", "BIND", "UNBIND", "REBIND", "MKACTIVITY", "CHECKOUT", "CHECKIN",
	"UNCHECKOUT", "MERGE", "UPDATE", "LABEL", "VERSION-CONTROL", "MKWORKSPACE",
	"BASELINE-CONTROL", "ORDERPATCH", "LINK", "UNLINK", "PURGE",
}

// rtspMethods 是 RTSP 1.0/2.0 的方法名。
var rtspMethods = []string{
	"OPTIONS", "DESCRIBE", "ANNOUNCE", "SETUP", "PLAY", "PAUSE", "TEARDOWN",
	"GET_PARAMETER", "SET_PARAMETER", "REDIRECT", "RECORD", "PLAY_NOTIFY",
}

// sipMethods 是 SIP 的请求方法名。
var sipMethods = []string{
	"REGISTER", "INVITE", "ACK", "BYE", "CANCEL", "OPTIONS", "SUBSCRIBE", "NOTIFY",
	"PUBLISH", "INFO", "REFER", "MESSAGE", "UPDATE", "PRACK",
}

// isMethodChar 判断字符能否出现在方法名中：大写字母、下划线与短横线。
func isMethodChar(c byte) bool {
	return c >= 'A' && c <= 'Z' || c == '_' || c == '-'
}

// methodPrefix 检查已收到的方法名部分能否是 methods 中某个方法的开头。b 已确认只含方法名字符。
func methodPrefix(b []byte, methods []string) bool {
	for _, m := range methods {
		if strings.HasPrefix(m, string(b)) {
			return true
		}
	}
	return false
}

// requestLine 解析「方法 目标 版本」形式的请求行。
// state 为 probeMore 表示方法名或请求行尚未读完；probeNo 表示开头不是合法的方法名；
// probeYes 时 method 有效，version 为请求行最后一个空格之后的部分（HTTP/0.9 等没有版本时为空），
// tooLong 表示读到 maxRequestLine 仍没有换行。
// methods 用于在方法名读完之前尽早排除，使其他协议不必等到超时。
func requestLine(b []byte, methods []string) (method, version string, tooLong bool, state probeResult) {
	i := 0
	for i < len(b) && i < maxMethodLen && isMethodChar(b[i]) {
		i++
	}
	if !methodPrefix(b[:i], methods) {
		return "", "", false, probeNo
	}
	if i == len(b) {
		return "", "", false, probeMore
	}
	if i == 0 || b[i] != ' ' {
		return "", "", false, probeNo
	}
	method = string(b[:i])
	if !contains(methods, method) {
		return "", "", false, probeNo
	}
	nl := bytes.IndexByte(b, '\n')
	if nl < 0 {
		if len(b) >= maxRequestLine {
			return method, "", true, probeYes
		}
		return "", "", false, probeMore
	}
	line := strings.TrimRight(string(b[i+1:nl]), "\r")
	if sp := strings.LastIndexByte(line, ' '); sp >= 0 {
		version = line[sp+1:]
	}
	return method, version, false, probeYes
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// probeHTTP 检查 HTTP 请求行：方法名为 HTTP 方法，版本为 HTTP/x 或没有版本（HTTP/0.9）；请求行超长时同样按 HTTP 处理。
func probeHTTP(b []byte) probeResult {
	_, version, tooLong, state := requestLine(b, httpMethods)
	if state != probeYes {
		return state
	}
	if tooLong || version == "" || strings.HasPrefix(version, "HTTP/") {
		return probeYes
	}
	return probeNo
}

// probeRTSP 检查 RTSP 请求行（摄像头、流媒体服务器），版本为 RTSP/1.0 或 RTSP/2.0。
func probeRTSP(b []byte) probeResult {
	_, version, tooLong, state := requestLine(b, rtspMethods)
	if state != probeYes {
		return state
	}
	if !tooLong && strings.HasPrefix(version, "RTSP/") {
		return probeYes
	}
	return probeNo
}

// probeSIP 检查 TCP 上的 SIP 请求行，版本为 SIP/2.0。
func probeSIP(b []byte) probeResult {
	_, version, tooLong, state := requestLine(b, sipMethods)
	if state != probeYes {
		return state
	}
	if !tooLong && version == "SIP/2.0" {
		return probeYes
	}
	return probeNo
}

// probeIRC 检查 IRC 客户端注册时最先发送的命令：CAP（能力协商）、PASS、NICK 或 USER。
func probeIRC(b []byte) probeResult {
	res := probeNo
	for _, cmd := range []string{"CAP ", "PASS ", "NICK ", "USER "} {
		switch hasPrefixPartial(b, cmd) {
		case probeYes:
			return probeYes
		case probeMore:
			res = probeMore
		}
	}
	return res
}
