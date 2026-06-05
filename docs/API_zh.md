# API 指南

导入路径：

```go
import "github.com/ioai-tech/lerobot-go/lerobot"
```

包文档：[pkg.go.dev/github.com/ioai-tech/lerobot-go/lerobot](https://pkg.go.dev/github.com/ioai-tech/lerobot-go/lerobot)

## 工作流

| 目标 | API | 对应 CLI |
|------|-----|----------|
| 并行分集写入 | `NewStagingWriter` → `Merge` | `create` |
| 单进程串行写入 | `Create` → `Finalize` | — |
| 校验布局 | `NewInspector` | `validate` |
| 有界并行任务 | `RunEpisodeJobs` | — |

## 核心类型

### Version

- `lerobot.V21` — codebase v2.1
- `lerobot.V30` — codebase v3.0（未设置时的默认值）

### FeatureSpec

与 `meta/info.json` 中特征定义一致：

```go
features := map[string]lerobot.FeatureSpec{
    "observation.state": {DType: "float32", Shape: []int{7}},
    "observation.images.cam": {DType: "video", Shape: []int{480, 640, 3}},
}
```

- `DType: "video"` — mp4 侧车文件（v2.1 默认路径），需要 ffmpeg
- `DType: "image"` — 图像内嵌 parquet（v2.1-img）
- 使用 video 时，在 StagingConfig 上设置 `UseVideos: true`

### StatsMode

- `lerobot.StatsSampled` — 官方抽样（默认）
- `lerobot.StatsFull` — 扫描每一帧

用于 `StagingConfig`、`MergeConfig`、`CreateConfig`。

### FFmpegConfig

```go
FFmpeg: lerobot.FFmpegConfig{FFmpegPath: "/usr/bin/ffmpeg"}
```

路径为空时从 `PATH` 查找。

### 临时帧目录（TempRoot）

`CreateConfig` 与 `StagingConfig` 也支持：

```go
TempRoot: "/dev/shm"
```

行为说明：

- `TempRoot` 为空时，写入器会自动探测内存文件系统；在 Linux 上优先使用 `/dev/shm`
- 若没有可写的内存文件系统，则自动回退到 episode 目录下的磁盘临时目录
- `dtype: "image"` 的内嵌图像不会再先写成临时图片文件，而是直接在内存中保留到 parquet 写入阶段
- `dtype: "video"` 必须配合 `UseVideos: true`；配置不一致会直接报错

## 并行 staging + merge

推荐用于多 episode 流水线（与 CLI `create` 等价）：

```go
w, _ := lerobot.NewStagingWriter(ctx, lerobot.StagingConfig{
    Version:  lerobot.V30,
    Dir:      filepath.Join(stagingRoot, "ep_000000"),
    Episode:  0,
    TempRoot: "/dev/shm", // 可选；留空时自动探测
    FPS:      30,
    Features: features,
    Stats:    lerobot.StatsSampled,
})
// 每帧 w.AddFrame(ctx, frame)
_, _ = w.SaveEpisode(ctx)

_ = lerobot.Merge(ctx, lerobot.MergeConfig{
    Version:     lerobot.V30,
    StagingRoot: stagingRoot,
    OutputRoot:  outputRoot,
    FPS:         30,
    Features:    features,
    Stats:       lerobot.StatsSampled,
})
```

staging 目录须命名为 `ep_NNNNNN`，且每集有完整的 `episode_meta.json`。

## 串行 Create + Finalize

单进程便捷 API，staging 在 `Root/_staging`，`Finalize` 时执行 Merge：

```go
ds, _ := lerobot.Create(ctx, lerobot.CreateConfig{
    Version:  lerobot.V30,
    Root:     "./dataset",
    TempRoot: "/dev/shm", // 可选；留空时自动探测
    FPS:      30,
    Features: features,
})
ds.AddFrame(ctx, frame)
ds.SaveEpisode(ctx)
_ = ds.Finalize(ctx)
```

## 校验

```go
insp := lerobot.NewInspector()
report, _ := insp.Validate(ctx, "./dataset")
strict, _ := insp.ValidateStrict(ctx, "./dataset")
diff, _ := insp.SchemaDiff(ctx, "./golden", "./candidate")
```

## 示例

- [examples/write_v30](../examples/write_v30/main.go)
- [examples/validate_dataset](../examples/validate_dataset/main.go)

## 协议说明

目录结构见 [protocol_v21.md](protocol_v21.md) 与 [protocol_v30.md](protocol_v30.md)。
