# CLI 参考

安装：`go install github.com/ioai-tech/lerobot-go/cmd/lerobot-go@latest`

```text
lerobot-go [全局参数] <命令> [命令参数] [位置参数]
```

## 全局参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-v`, `--verbose` | `false` | 详细日志输出到 stderr |
| `--ffmpeg-path` | — | `ffmpeg` 路径（覆盖 `PATH`） |
| `--json` | `false` | stdout 输出 JSON（`validate`、`create`、`merge` 等） |

## 退出码

| 码 | 含义 |
|----|------|
| `0` | 成功 |
| `1` | 命令失败（校验、I/O、合并错误） |
| `2` | 用法 / 参数错误 |

## `validate`

校验目录是否符合 LeRobot v2.1 或 v3.0 目录结构。

```bash
lerobot-go validate ./dataset
lerobot-go validate ./dataset --json
lerobot-go validate ./dataset --tree -v
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--strict` | `true` | 存在 `data/` 时做完整 formatcheck |
| `--tree` | `false` | 打印文件树（人类可读，stderr） |

## `convert`

在 v2.1 与 v3.0 之间转换，写入新的 `--output` 目录（不支持原地覆盖）。

```bash
lerobot-go convert -i ./dataset_v21 -o ./dataset_v30 --to v3.0
lerobot-go convert -i ./dataset_v30 -o ./dataset_v21 --to v2.1
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-i`, `--input` | 必填 | 源数据集根目录 |
| `-o`, `--output` | 必填 | 目标根目录 |
| `--to` | 必填 | 目标版本：`v2.1` 或 `v3.0` |
| `--from` | 自动 | 从 `meta/info.json` 读取源版本 |
| `--force` | `false` | 允许非空输出目录 |
| `--data-file-size-mb` | `100` | v3.0 数据分块阈值（MB） |
| `--video-file-size-mb` | `200` | v3.0 视频分块大小（MB） |
| `--chunks-size` | `1000` | 每 chunk 最大文件数 |

## `create`

将已完成的 staging 分集（`ep_*`）落盘为正式数据集布局，等价于 Go API 的 `lerobot.Merge`。

在并行写入完成后使用，默认 staging 为 `<output>/_staging`（可用 `--staging` 指定）。

```bash
lerobot-go create -o ./dataset --version v3.0 --fps 30 --features ./features.json
lerobot-go create -o ./dataset --version v2.1 --fps 10 --features ./features.json --stats-mode full
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-o`, `--output` | 必填 | 数据集根目录（最终布局） |
| `--version` | 必填 | `v2.1` 或 `v3.0` |
| `--features` | 必填 | JSON：特征名 → `{dtype, shape}` |
| `--fps` | 必填 | 数据集 FPS |
| `--staging` | `<output>/_staging` | 含 `ep_*` 的 staging 根目录 |
| `--robot-type` | — | 写入 `meta/info.json` |
| `--stats-mode` | `sampled` | 图像/视频统计：`sampled` 或 `full` |
| `--force` | `false` | 允许非空输出目录 |

### features JSON 示例

```json
{
  "observation.state": { "dtype": "float32", "shape": [7] },
  "action": { "dtype": "float32", "shape": [7] },
  "observation.images.cam": { "dtype": "video", "shape": [480, 640, 3] }
}
```

将 `"dtype": "video"` 改为 `"image"` 可写入 parquet 内嵌图像（v2.1-img）。

## `merge`

合并两个或多个**已完成**的数据集。

```bash
lerobot-go merge -o ./merged --to v3.0 -i ./a -i ./b
lerobot-go merge -o ./merged_v21 --to v2.1 ./ds1 ./ds2
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-o`, `--output` | 必填 | 输出数据集根目录 |
| `--to` | 必填 | 输出版本：`v2.1` 或 `v3.0` |
| `-i`, `--input` | — | 源路径（可重复）；也可用位置参数 |
| `--force` | `false` | 允许非空输出目录 |
| `--stats-mode` | `sampled` | 重算媒体统计时使用；尽可能保留源 stats |

## `version` / `completion`

```bash
lerobot-go version
lerobot-go completion bash
```

支持 shell：`bash`、`zsh`、`fish`、`powershell`。
