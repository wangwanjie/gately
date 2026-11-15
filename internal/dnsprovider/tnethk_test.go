package dnsprovider

import "testing"

func TestTnethk(t *testing.T) {
	testNowcnFamily(t, "tnethk", func(p Provider, url string) { p.(*tnethk).endpoint = url })
}

func TestTnethkEndpoint(t *testing.T) {
	p, err := New("tnethk", Credentials{ID: "i", Secret: "s"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.(*tnethk).endpoint != "https://www.tnet.hk" {
		t.Fatalf("接口地址错误: %s", p.(*tnethk).endpoint)
	}
	n, _ := New("nowcn", Credentials{ID: "i", Secret: "s"}, nil)
	if n.(*nowcn).endpoint != "https://www.now.cn" {
		t.Fatalf("接口地址错误: %s", n.(*nowcn).endpoint)
	}
}
