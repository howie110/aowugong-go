---
name: chat-archive-sync
description: Use when the user asks to upload, backfill, preview, or verify local Codex or Claude Code dialogue through their dedicated HTTPS chat archive API.
---

# 个人聊天归档同步

版本：2.0（逐条核对与自动补齐）。默认一次执行完成：扫描授权来源 → 核对云端 → 补齐缺失/修改版本 → 再次核对与哈希校验。只上传真实用户文字与可确认的 AI 最终回复，不上传文件、附件、思考、工具记录或系统注入内容。不摘要、不改写、不修改源日志、不清空云端。

## 直接执行

配置优先使用本 Skill 目录的 `config.json`，否则沿用用户此前明确提供/本任务已使用的私密配置路径。不搜索其他项目、全局凭据或 SSH 配置。已有的 api_url、api_key、device_id、account_scope、sources 必须保留；更换设备标识会影响无原始消息 ID 的去重。

用户要求运行/上传本 Skill 已授权本次配置范围的文字归档。简短告知文字将长期保存且可能含敏感信息，然后运行一次：

```bash
python scripts/sync.py --config <现有私密JSON>
```

路径相对本 Skill；Python 3.11+，仅标准库，不需要数据库驱动、SSH 或本地状态库。压缩日志 `.jsonl.zst` 需要 zstd。只预览用 `--dry-run`；只比较本地与云端、不上传用 `--verify`。不要先单独做一份审计报告再停下等用户转交，也不要重写一套补传脚本。

缺少配置/依赖先在当前授权范围内处理；Key 无效、远端不可达或接口缺失则报告固定错误码，不能扩权、自行改服务器或换成数据库直连。公司数据只采集用户有权外传的范围。

## 升级安装

收到新版包时，使用包内脚本安装，`--config` 指向这台电脑**上次实际使用的配置**，不是包里尚未初始化的备用配置：

```bash
python chat-archive-sync/scripts/install.py --config <现有私密JSON>
```

默认目标为当前用户 `~/.codex/skills/chat-archive-sync`；旧脚本备份到同级 Codex 数据目录的 `skill-backups`，现有配置复制到安装目录 `config.json` 并限制权限，保留设备身份/来源路径。安装不上传，安装后立即按本次用户授权运行上面的同步命令。

只有真正的新设备才用 `configure_device.py --config <提供的HTTPS配置> --codex-home <授权Codex数据目录>` 初始化；不得在老设备上生成新身份。Claude Code 路径在配置的 sources 中显式列出，不自动扩大采集范围。

## 已实现的采集与恢复

- Codex 优先取 `content_item_kinds=user.text`；AI 仅取 final_answer/final。跳过子 Agent、analysis/commentary、心跳自动输入。旧 user 缺标记时，通过本机 Codex `thread/turns/list` 的 full 分页记录取得原生 userMessage 证据，按消息/轮次身份与文本哈希匹配，或在完整原生会话中按精确出现次数匹配。不是按正文去重，也不是把所有 role=user 收录。
- Codex 优先连接正在运行的 app-server proxy，无代理时自动回退到 CLI stdio 只读历史服务。只调用 initialize 和 thread/turns/list，不启动 turn、不恢复/续写会话。可选 `codex_command` 字符串数组指定已授权可执行文件和参数；默认从 PATH 定位 codex，Windows 另检查已知 OpenAI/Codex 安装二进制目录。原生接口不可用、读取不完整、无可靠来源证据的部分单独计数，不妨碍确认过的消息上传。
- Claude Code 排除元消息、摘要、子任务、工具调用文字；助手仅确认 end_turn 文字，其余最终阶段不明的文字计入待核实。Claude 桌面版不支持。
- 扫描所有已配置 JSONL/JSONL.ZST，不依赖旧增量游标。两个格式保持同一来源身份。来源变化重读一次；损坏压缩文件、异常超大单行明确失败。文本按最多500条及编码大小分批请求，不把整个大文件上传。
- `/check` 仅发送身份与版本哈希；`/repair` 只发送缺失文字，服务端幂等追加版本，不覆盖旧记录、不改原游标。失败重跑只补缺失部分，429/临时网络错误有限重试。同内容不同消息仍分别保留。身份缺失时使用本设备文件位置，不保证跨设备合并。

## 结果解读

以脚本最后的 summary 为准：报告确认 user/assistant 数量、本次 restored、inserted、verified、unclassified、failed_sources、missing_sources 和云端完整性校验结果。不输出正文或密钥。restored 是本次发现缺失/不同、随后确认云端已存在的条数；inserted 仅为服务端已确认响应中的插入数，若响应丢失后重试，可能比 restored 小，不能把它说成绝对实际插入总数。某批失败仍保留前面批次的完成数量。

- 退出0：配置内可读取、可确认的记录核对通过；不是证明所有工具从未丢失过记录。
- 退出2：有待确认来源/缺失源文件，或只读模式发现云端缺口。**同步模式下确认过的消息已经自动补传**，不能误报“因有待核实内容所以本次未上传”。列出实际例外，不把未确认内容猜成用户输入。
- 退出1：实际失败；说明固定错误码及已完成部分，不把总数相同或游标到文件末尾当成完整。

已删除、被摘要替换或工具无法提供的原文不能恢复。不要承诺绝对零遗漏，也不要把保守跳过当成没有对话。不发送外部通知、不启用后台定时任务、不要求用户在电脑之间转发排查文案。
