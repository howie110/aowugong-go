# VPN 公共分流规则 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add one file-backed, read-only public routing policy that is embedded in Clash/Surge/Shadowrocket subscriptions and exposed as a separate authenticated routing URL for v2rayN.

**Architecture:** SourceCatalog reads storage/private/vpn/common-routing.json on every build, validates the Xray routing object, and converts supported field rules into each client syntax. Existing node formats and device-token lifecycle remain unchanged; Build adds a routing payload and the service exposes its URL as routing_url. The admin summary returns the raw file only to administrators, while the UI only renders and copies it.

**Tech Stack:** Go 1.26, encoding/json, gopkg.in/yaml.v3, chi HTTP handlers, React/TypeScript, existing Go and Node test runners.

**Spec:** docs/superpowers/specs/2026-09-06-vpn-common-routing-design.md

## Global Constraints

- The only authoritative rules file is storage/private/vpn/common-routing.json.
- The page must not provide an editor, save endpoint, draft, publish, version, or rollback flow.
- Rules are read on each subscription build so the next client refresh sees the latest deployed file.
- Missing or empty rules preserve the existing four node subscription outputs; invalid or unsupported rules fail generation explicitly.
- routing is an additive v2rayN custom-routing response; existing client URL formats remain stable.
- Never log or persist node credentials or routing file contents outside the intended response/payload.
- Preserve the existing untracked LOG/ directory and do not add it to commits.

---

### Task 1: Add the common-routing parser and client rule converters

**Files:**
- Create: internal/vpn/routing.go
- Create: internal/vpn/routing_test.go

**Interfaces:**
- Produce type commonRouting, parseCommonRouting([]byte) (commonRouting, error), commonRouting.JSON() (string, error), commonRouting.RuleLines(client, proxyPolicy string) ([]string, error), and replaceRuleSection(content, section string, rules []string) (string, error).
- commonRouting contains DomainStrategy string and Rules []routingRule; routingRule contains Type, Domain, IP, Port, Network, Protocol, and OutboundTag with Xray JSON tags.

- [ ] Step 1: Write failing parser and conversion tests.

Create a JSON fixture with one domain:example.com proxy rule, one geoip:private direct rule, and one 1.1.1.1/32 block rule. Assert parsing preserves DomainStrategy and order, and RuleLines("clash", "PROXY") returns DOMAIN-SUFFIX,example.com,PROXY, GEOIP,PRIVATE,DIRECT, and IP-CIDR,1.1.1.1/32,REJECT. Add malformed JSON and unknown outboundTag failures. Add text-section tests proving [Rule] replacement preserves [General]/[MITM] and appends a missing section. Add a JSON serialization test.

- [ ] Step 2: Run the focused tests and verify the expected red failure.

Run: go test ./internal/vpn -run 'Test(ParseCommonRouting|CommonRouting|ReplaceRuleSection)' -count=1

Expected: compilation/test failure because the parser, converter, and section replacement functions do not exist.

- [ ] Step 3: Implement the minimal parser and converters.

Use json.Decoder.DisallowUnknownFields. Treat empty input as an empty routing object; reject unsupported rule types and unknown targets. Support domain:, full:, keyword:, geoip:, IPv4/IPv6 CIDR, numeric port, and tcp/udp network. Map proxy to the selected policy, direct to DIRECT, and block to REJECT. Use DST-PORT for Clash and DEST-PORT for Surge/Shadowrocket. Return explicit errors for protocol, compound matchers, malformed CIDRs, non-numeric ports, and unsupported rule types. Implement replaceRuleSection by replacing only the named bracket section and appending it when absent.

- [ ] Step 4: Run the focused tests and verify green.

Run the same go test command; all parser, converter, and section tests must pass.

- [ ] Step 5: Commit.

Run: git add internal/vpn/routing.go internal/vpn/routing_test.go && git commit -m "feat: add vpn common routing converters"

### Task 2: Integrate common routing into source discovery and generated configs

**Files:**
- Modify: internal/vpn/source.go
- Modify: internal/vpn/source_test.go
- Modify: storage/private/vpn/README.md

**Interfaces:**
- Add SourceCatalog.CommonRouting() (ConfigContent, error) for the raw administrator view and internal loadCommonRouting() (commonRouting, error).
- Build continues returning the four existing keys and adds routing with application/json content type and filename <profile>-routing.json.

- [ ] Step 1: Write failing SourceCatalog tests.

Use a fixture with clash_demo.yaml, surge_demo.conf, shadowrocket_demo.conf, v2rayn_demo.json, and common-routing.json. Assert the same domain/IP rules appear in Clash, Surge, and Shadowrocket bodies, and configs["routing"].Body contains domainStrategy. Add missing-file compatibility and invalid-file error tests. Update old exact-length assertions to require the four existing keys while allowing routing.

- [ ] Step 2: Run the focused SourceCatalog tests and verify red.

Run: go test ./internal/vpn -run 'TestSourceCatalog' -count=1

Expected: failures because the catalog does not read or merge common-routing.json.

- [ ] Step 3: Add file reading and the routing payload.

Keep common-routing.json out of source filename discovery. CommonRouting reads it with the existing size limit; a missing file returns the fixed filename and empty body. loadCommonRouting parses it and uses an empty object for absent/blank input. Build loads once and always adds a valid routing ConfigContent while keeping the v2ray node links unchanged.

- [ ] Step 4: Apply rules to all generated client configs.

For Clash, parse YAML as a dynamic map, select an existing PROXY group or the first non-DIRECT/REJECT group, replace rules, and marshal YAML. Apply the same helper to generated Clash output. For Surge and Shadowrocket, select an existing PROXY group or the first [Proxy Group] policy and replace/append [Rule] with client-specific port syntax. Preserve original bodies when the common rule list is empty. When Shadowrocket source is missing, use the existing VMess-to-Surge text generator instead of returning only the Base64 node body, then merge rules.

- [ ] Step 5: Document the private file.

Update storage/private/vpn/README.md with the exact filename, a valid JSON example, target names proxy/direct/block, and the statement that AI/deployment updates the file while the page is read-only.

- [ ] Step 6: Run focused and package-wide VPN tests.

Run:
~~~bash
gofmt -w internal/vpn/routing.go internal/vpn/routing_test.go internal/vpn/source.go internal/vpn/source_test.go
go test ./internal/vpn -count=1
~~~

Expected: all VPN source and service tests pass.

- [ ] Step 7: Commit.

Run: git add internal/vpn/source.go internal/vpn/source_test.go storage/private/vpn/README.md && git commit -m "feat: merge common routing into vpn configs"

### Task 3: Expose read-only routing metadata and the v2rayN routing URL

**Files:**
- Modify: internal/vpn/model.go
- Modify: internal/vpn/service.go
- Modify: internal/vpn/service_test.go
- Modify: internal/httpserver/vpn_handlers_test.go

**Interfaces:**
- Add CommonRouting { Filename string; Body string } and Summary.CommonRouting *CommonRouting with omitempty.
- Add UserSubscription.RoutingURL string serialized as routing_url.
- publicSubscription creates routing_url at the same device/token path with format routing; the existing four subscriptions map entries remain unchanged.

- [ ] Step 1: Write failing service tests.

Add common-routing.json to a service fixture. After Create, assert routing_url uses the same device/token path as v2ray. Request service.Subscription with routing and assert the JSON body. Modify the file and request again to prove the newest disk contents are returned without republishing. Assert administrator Summary includes raw CommonRouting and ordinary-user Summary has nil CommonRouting.

- [ ] Step 2: Run focused service tests and verify red.

Run: go test ./internal/vpn -run 'TestService.*Routing|TestService.*Summary' -count=1

Expected: compilation/assertion failures because the model and service do not expose routing metadata.

- [ ] Step 3: Implement model and service changes.

Populate Summary.CommonRouting only when canManage is true by calling SourceCatalog.CommonRouting. Keep ordinary-user summaries free of the raw rules. Add routing_url for active/published devices with a configured distributor. The existing Subscription method already delegates to Build, so routing uses the existing token validation.

- [ ] Step 4: Add HTTP assertions.

Request the routing URL in the public subscription test and assert HTTP 200, application/json content type, and rule body. Decode administrator and ordinary-user summaries and assert only the administrator response contains common_routing.

- [ ] Step 5: Run focused HTTP/service tests and verify green.

Run:
~~~bash
gofmt -w internal/vpn/model.go internal/vpn/service.go internal/vpn/service_test.go internal/httpserver/vpn_handlers_test.go
go test ./internal/vpn ./internal/httpserver -count=1
~~~

Expected: all service and HTTP tests pass.

- [ ] Step 6: Commit.

Run: git add internal/vpn/model.go internal/vpn/service.go internal/vpn/service_test.go internal/httpserver/vpn_handlers_test.go && git commit -m "feat: expose vpn routing subscription metadata"

### Task 4: Add read-only administrator and v2rayN user UI

**Files:**
- Modify: web/src/lib/vpn.ts
- Modify: web/src/pages/vpn.tsx
- Create: web/tests/vpn-common-routing.test.cjs

**Interfaces:**
- VPNSummary adds optional common_routing { filename: string; body: string }.
- VPNUserSubscription adds routing_url: string.
- Reuse copyTextToClipboard; no write API or editable form is added.

- [ ] Step 1: Write failing static UI tests.

Create Node tests that read web/src/pages/vpn.tsx and assert labels 公共分流规则 and 只读, a non-editable pre/read-only display, and copyTextToClipboard for raw rules. Assert v2rayN 分流规则 and subscription.routing_url. Assert no common-rule save/update request exists.

- [ ] Step 2: Run the UI tests and verify red.

Run: node --test web/tests/vpn-common-routing.test.cjs

Expected: failures because the labels, types, and rendering do not exist.

- [ ] Step 3: Add read-only admin rendering.

In VPNDistributionPage, render a card below the status cards when summary.can_manage is true. Show the fixed filename, summary.common_routing?.body || 暂无公共规则配置 in a pre block, a read-only description saying AI/deployment updates the file, and a copy button using the existing notifier. Do not add textarea, form state, save handler, or mutation API.

- [ ] Step 4: Add the v2rayN routing-link copy action.

In VPNResourcesPage, render a fifth small card only when subscription.routing_url is non-empty. Label it v2rayN 分流规则, explain that v2rayN imports custom routing separately, and copy it with the existing clipboard helper. Keep QR actions limited to the four client formats.

- [ ] Step 5: Run UI tests and the TypeScript build.

Run:
~~~bash
node --test web/tests/vpn-common-routing.test.cjs
npm --prefix web run build
~~~

Expected: all new assertions pass and the TypeScript/Vite build exits 0.

- [ ] Step 6: Commit.

Run: git add web/src/lib/vpn.ts web/src/pages/vpn.tsx web/tests/vpn-common-routing.test.cjs && git commit -m "feat: show vpn routing rules read-only"

### Task 5: Full verification and handoff

**Files:**
- Modify only files required by failing verification; never include LOG/.

- [ ] Step 1: Run the complete Go suite.

Run: go test ./... -count=1

Expected: exit code 0 with no failing packages.

- [ ] Step 2: Run the complete frontend suite and production build.

Run:
~~~bash
npm --prefix web test
npm --prefix web run build
~~~

Expected: both commands exit 0.

- [ ] Step 3: Inspect final diff and repository state.

Run:
~~~bash
git diff --check HEAD~5..HEAD
git status --short --branch
git log --oneline -6
~~~

Confirm the only untracked path remains the pre-existing LOG/ directory, no private VPN file was staged, and planned fields/routes/UI labels are present.

- [ ] Step 4: Report verified local status.

Report test/build results, the private file path to maintain, and whether production deployment was performed. Do not claim deployment unless separately authorized and its health check succeeds.
