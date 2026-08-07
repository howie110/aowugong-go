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

The production Xray client reads `/usr/local/etc/xray/config.json` and exposes
HTTP/SOCKS listeners only on `127.0.0.1`. Use `scripts/install-xray-client.sh`
to install a locally prepared configuration. Never force-add live node files to
this public repository.
