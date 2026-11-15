package dnsprovider

import "testing"

func TestEranet(t *testing.T) {
	testNowcnFamily(t, "eranet", func(p Provider, url string) { p.(*eranet).endpoint = url })
}

func TestEranetEndpoint(t *testing.T) {
	p, err := New("eranet", Credentials{ID: "i", Secret: "s"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.(*eranet).endpoint != "https://www.eranet.com" {
		t.Fatalf("接口地址错误: %s", p.(*eranet).endpoint)
	}
}
