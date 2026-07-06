package node

import (
	"os"
	"testing"

	"github.com/InazumaV/V2bX/conf"
)

func newIntegrationLego(t *testing.T) *Lego {
	t.Helper()
	if os.Getenv("V2BX_RUN_ACME_INTEGRATION_TESTS") != "1" {
		t.Skip("set V2BX_RUN_ACME_INTEGRATION_TESTS=1 to run live ACME integration tests")
	}
	token := os.Getenv("CF_DNS_API_TOKEN")
	if token == "" {
		t.Fatal("CF_DNS_API_TOKEN is required for live ACME integration tests")
	}
	domain := os.Getenv("V2BX_ACME_TEST_DOMAIN")
	if domain == "" {
		domain = "test.test.com"
	}
	l, err := NewLego(&conf.CertConfig{
		CertMode:   "dns",
		Email:      "test@test.com",
		CertDomain: domain,
		Provider:   "cloudflare",
		DNSEnv: map[string]string{
			"CF_DNS_API_TOKEN": token,
		},
		CertFile: "./cert/1.pem",
		KeyFile:  "./cert/1.key",
	})
	if err != nil {
		t.Fatalf("NewLego() error = %v", err)
	}
	return l
}

func TestLego_CreateCertByDns(t *testing.T) {
	l := newIntegrationLego(t)
	err := l.CreateCert()
	if err != nil {
		t.Error(err)
	}
}

func TestLego_RenewCert(t *testing.T) {
	l := newIntegrationLego(t)
	t.Log(l.RenewCert())
}
