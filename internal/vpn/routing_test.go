package vpn

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseCommonRoutingConvertsSupportedRules(t *testing.T) {
	content := `{
  "domainStrategy": "IPIfNonMatch",
  "rules": [
    {"type":"field","domain":["domain:example.com"],"outboundTag":"proxy"},
    {"type":"field","ip":["geoip:private"],"outboundTag":"direct"},
    {"type":"field","ip":["1.1.1.1/32"],"outboundTag":"block"}
  ]
}`

	routing, err := parseCommonRouting([]byte(content))
	if err != nil {
		t.Fatalf("parseCommonRouting() error = %v", err)
	}
	if routing.DomainStrategy != "IPIfNonMatch" || len(routing.Rules) != 3 {
		t.Fatalf("routing = %#v", routing)
	}
	lines, err := routing.RuleLines("clash", "PROXY")
	if err != nil {
		t.Fatalf("RuleLines() error = %v", err)
	}
	want := []string{
		"DOMAIN-SUFFIX,example.com,PROXY",
		"GEOIP,PRIVATE,DIRECT",
		"IP-CIDR,1.1.1.1/32,REJECT",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("RuleLines() = %#v, want %#v", lines, want)
	}
}

func TestParseCommonRoutingRejectsMalformedJSONAndUnknownTargets(t *testing.T) {
	for name, content := range map[string]string{
		"malformed":      `{`,
		"unknown target": `{"rules":[{"type":"field","domain":["domain:example.com"],"outboundTag":"unknown"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseCommonRouting([]byte(content)); err == nil {
				t.Fatal("parseCommonRouting() error = nil, want error")
			}
		})
	}
}

func TestCommonRoutingJSONReturnsValidDocument(t *testing.T) {
	routing := commonRouting{
		DomainStrategy: "IPIfNonMatch",
		Rules:          []routingRule{{Type: "field", Domain: []string{"domain:example.com"}, OutboundTag: "direct"}},
	}
	body, err := routing.JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if decoded["domainStrategy"] != "IPIfNonMatch" {
		t.Fatalf("decoded domainStrategy = %#v", decoded["domainStrategy"])
	}
}

func TestReplaceRuleSectionReplacesOnlyRequestedSection(t *testing.T) {
	content := "[General]\nloglevel = notify\n\n[Rule]\nOLD,DIRECT\n\n[MITM]\nhostname = example.com\n"
	updated, err := replaceRuleSection(content, "Rule", []string{"DOMAIN-SUFFIX,example.com,PROXY", "FINAL,DIRECT"})
	if err != nil {
		t.Fatalf("replaceRuleSection() error = %v", err)
	}
	if strings.Contains(updated, "OLD,DIRECT") || !strings.Contains(updated, "loglevel = notify") || !strings.Contains(updated, "hostname = example.com") {
		t.Fatalf("updated content lost or retained unrelated sections: %q", updated)
	}
	if !strings.Contains(updated, "[Rule]\nDOMAIN-SUFFIX,example.com,PROXY\nFINAL,DIRECT") {
		t.Fatalf("updated Rule section = %q", updated)
	}
}

func TestReplaceRuleSectionAppendsMissingSection(t *testing.T) {
	updated, err := replaceRuleSection("[General]\nloglevel = notify\n", "Rule", []string{"FINAL,PROXY"})
	if err != nil {
		t.Fatalf("replaceRuleSection() error = %v", err)
	}
	if !strings.Contains(updated, "[Rule]\nFINAL,PROXY") {
		t.Fatalf("updated content = %q", updated)
	}
}
