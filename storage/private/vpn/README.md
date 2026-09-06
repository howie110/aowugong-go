# Local VPN configs

Put local client config files here, for example Clash, Shadowrocket, Surge,
v2rayN, or subscription export files.

Files in this folder are ignored by Git by default because they may contain
server addresses, tokens, UUIDs, or private subscription links.

Suggested files from the current set:

- `clash_dmit.yaml`
- `flclash_dmit.yaml`
- `powershell.txt`
- `shadowrocket_dmit.conf`
- `surge_dmit.conf`
- `surge_flowercloud.conf`
- `v2rayn_dmit.json`

The optional `common-routing.json` file is the single public routing policy for
all assigned users. It is read-only on the web page and should be updated by
AI/deployment when the shared rules change. Supported outbound targets are
`proxy`, `direct`, and `block`; the file uses the Xray routing shape:

```json
{
  "domainStrategy": "IPIfNonMatch",
  "rules": [
    {"type": "field", "domain": ["domain:example.com"], "outboundTag": "proxy"},
    {"type": "field", "ip": ["geoip:private"], "outboundTag": "direct"}
  ]
}
```

Clash, Surge, and Shadowrocket receive converted rules inside their configs.
v2rayN keeps its standard node subscription. Its normal subscription format
cannot carry the full routing configuration, so the user page does not expose
an additional standalone rules resource; v2rayN routing remains a local client
setting when needed.

The production Xray client reads `/usr/local/etc/xray/config.json` and exposes
HTTP/SOCKS listeners only on `127.0.0.1`. Use `scripts/install-xray-client.sh`
to install a locally prepared configuration. Never force-add live node files to
this public repository.
