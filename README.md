# CFData-WEB（精简版：官方优选 CLI）

基于 [PoemMisty/CFData-WEB](https://github.com/PoemMisty/CFData-WEB) 裁剪的轻量版，只保留适合 N1 / OpenWrt 等设备定时运行的功能：

**Cloudflare 官方 IP 段扫描（TCPing / HTTPing）→ 数据中心筛选 → 详细测试 → 下载测速 → 合格 IP 筛选 → 导出并上传 GitHub**

已移除：Web 前端与登录、WebSocket、非标优选（nsb）、edgetunnel 上传、Android APK、更新检测。

## 与原版的行为差异

- 始终是 CLI 模式，不需要 `-cli`（旧参数 `-cli` 仍可传入，会被忽略）。
- **只导出、只上传测速合格的 IP**（速度 ≥ `offspeedmin`）。
- 没有合格 IP 时**不写本地文件、不上传**，程序以退出码 1 结束，GitHub 上的旧文件保持不变。
- `offspeedlimit` 必须大于 0（精简版不再提供“仅延迟”输出）。
- 任何失败都以非 0 退出码结束，方便 cron / 脚本判断。

## 快速开始

从 Releases 下载对应平台二进制（Linux amd64 / arm64），首次运行会在二进制所在目录生成 `cfdata-config.json`：

```bash
./cfdata-linux-arm64
```

编辑配置后再次运行即可。仓库中的 `cfdata-config.example.json` 是定时任务场景的示例（已开启 `skipgeo`、关闭进度与颜色、开启 GitHub 上传）。

> `cfdata-config.json` 可能含 GitHub token，已被 `.gitignore` 忽略，请不要提交到仓库。推荐用 `ghtokenfile` 指向权限受限的 token 文件，并将 token 限制为仅能读写目标仓库。

## 定时运行（OpenWrt 示例）

必须开启 `skipgeo`（配置文件 `"skipgeo": true` 或命令行 `-skipgeo`），否则检测到非直连环境时程序会等待终端输入：

```bash
# crontab -e：每天 4:30 运行，日志写入文件
30 4 * * * cd /opt/cfdata && ./cfdata-linux-arm64 -skipgeo -nocolor -progress=false >> /var/log/cfdata.log 2>&1
```

## 配置与参数

优先级：命令行参数 > 配置文件 > 环境变量（`CFDATA_*`）> 默认值。

| 配置项 | 说明 | 默认 |
| --- | --- | --- |
| `skipgeo` | 跳过地区/代理环境验证（无人值守必须开启） | `false` |
| `scanmode` | `tcping` 或 `httping` | `tcping` |
| `offiptype` | IP 类型 `4` / `6` | `4` |
| `offthreads` | 扫描并发数（小内存设备建议 50 左右） | `100` |
| `offport` | 详细测试与测速端口 | `443` |
| `offdelay` | 延迟阈值（毫秒） | `500` |
| `offdc` | 指定数据中心，留空则自动选最低延迟 | 空 |
| `offurl` | 测速地址，`auto` 为自动选择 | `auto` |
| `offspeedlimit` | 测速达标数量上限（必须 > 0） | `5` |
| `offspeedmin` | 测速达标下限（MB/s） | `0.1` |
| `offout` | 本地输出文件名（仅合格 IP） | `ip.csv` |
| `format` / `fields` / `custom` / `v6bracket` | 导出格式与字段 | `txt` / `compact` |
| `github` / `ghrepo` / `ghbranch` / `ghpath` / `ghmessage` | GitHub 上传 | 关闭 |
| `ghtoken` / `ghtokenfile` | token 或 token 文件（也可用环境变量 `GITHUB_TOKEN`） | 空 |
| `ghupload` | 直接上传指定文件，不执行测试（需 `github=true`） | 空 |
| `compactipv4` | 精简本地 IPv4 地址库（覆盖 `ips-v4.txt`） | `false` |
| `dns` / `debug` / `nocolor` / `progress` | DNS、调试、输出控制 | 见 `-h` |

完整参数：`./cfdata-linux-arm64 -h`

## 本地缓存

首次运行会在当前目录下载并缓存 `ips-v4.txt`、`ips-v6.txt`、`locations.json`。需要重置时直接删除这些文件即可。

## 构建

Go 模块位于 `combined_refactor/`：

```bash
cd combined_refactor
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w -X main.appVersion=v1.0" -o ../cfdata-linux-arm64 .
```

GitHub Actions（`Build and Release`）会构建 Linux amd64 / arm64 并发布 Release。

## 免责声明

本程序仅用于学习与研究。使用本程序时，应自行遵守所在地区的法律法规，作者不对使用本程序所产生的任何后果承担责任。

## 致谢

- 上游项目：[PoemMisty/CFData-WEB](https://github.com/PoemMisty/CFData-WEB)
- 原始思路：TG 频道 CF中转IP、[Kwisma/iptest](https://github.com/Kwisma/iptest)

## License

GPL v3.0 or later，详见 [LICENSE](LICENSE)。
