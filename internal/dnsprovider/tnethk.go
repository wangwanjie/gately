package dnsprovider

import "net/http"

// Tnet.hk：与时代互联共用同一套独立解析 API，接口地址为 https://www.tnet.hk，
// 签名与各接口的参数见 nowcn.go。

func init() {
	register(Meta{
		Name:           "tnethk",
		Title:          "Tnet.hk",
		IDLabel:        "AccessInstanceID",
		SecretLabel:    "AccessKeySecret",
		DocURL:         "https://help.tnet.hk/domain/resolve/20260403163405/",
		SupportsACME:   true,
		SupportsDelete: true,
	}, func(c Credentials, hc *http.Client) (Provider, error) {
		api, err := newNowcnAPI("Tnet.hk", "https://www.tnet.hk", c, hc)
		if err != nil {
			return nil, err
		}
		return &tnethk{nowcnAPI: *api}, nil
	})
}

type tnethk struct {
	nowcnAPI
}

// 编译期确认实现了接口
var (
	_ TXTProvider   = (*tnethk)(nil)
	_ recordDeleter = (*tnethk)(nil)
)
