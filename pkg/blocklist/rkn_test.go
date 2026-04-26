package blocklist

import (
	"strings"
	"testing"
)

func TestLoadDomainsAndContains(t *testing.T) {
	s := NewSet(1<<14, 5)
	body := "yandex.ru\nvk.com\nrkn.gov.ru\n# comment\n\n"
	if err := s.LoadDomains(strings.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	if !s.Contains("yandex.ru") {
		t.Errorf("expected yandex.ru in set")
	}
	if !s.Contains("Yandex.RU.") {
		t.Errorf("normalisation should match")
	}
	if !s.Contains("https://vk.com/foo?bar") {
		t.Errorf("URL normalisation should match")
	}
	if s.Contains("not-listed.example") {
		t.Errorf("unexpected positive on unknown host")
	}
	if s.Len() != 3 {
		t.Errorf("len = %d, want 3", s.Len())
	}
}

func TestLoadXML(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<reg:register xmlns:reg="http://rkn.gov.ru">
  <content>
    <decision number="1">
      <domain>blocked.example</domain>
      <ip>1.2.3.4</ip>
    </decision>
  </content>
  <content>
    <decision number="2">
      <domain>also.blocked</domain>
      <url>https://also.blocked/path?q=1</url>
    </decision>
  </content>
</reg:register>`
	s := NewSet(1<<12, 5)
	if err := s.LoadXML(strings.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"blocked.example", "1.2.3.4", "also.blocked"} {
		if !s.Contains(h) {
			t.Errorf("expected %s in set", h)
		}
	}
	if s.Contains("not-blocked.example") {
		t.Errorf("unexpected positive")
	}
}
