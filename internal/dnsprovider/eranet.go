package dnsprovider

import "net/http"

// 新网互联 Eranet：与时代互联共用同一套独立解析 API，接口地址为 https://www.eranet.com，
// 签名与各接口的参数见 nowcn.go。

func init() {
	register(Meta{
		Name:           "eranet",
		Title:          "新网互联 (Eranet)",
		IDLabel:        "AccessInstanceID",
		SecretLabel:    "AccessKeySecret",
		DocURL:         "https://help.eranet.com/domain/resolve/20260407145050/",
		SupportsACME:   true,
		SupportsDelete: true,
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		api, err := newNowcnAPI("新网互联", "https://www.eranet.com", c, hc)
		if err != nil {
			return nil, err
		}
		return &eranet{nowcnAPI: *api}, nil
	})
}

type eranet struct {
	nowcnAPI
}

// 编译期确认实现了接口
var (
	_ TXTProvider   = (*eranet)(nil)
	_ recordDeleter = (*eranet)(nil)
)
