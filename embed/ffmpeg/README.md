# Bundled ffmpeg binaries

Place static ffmpeg/ffprobe per platform for `bundle_ffmpeg` builds:

```
embed/ffmpeg/linux_amd64/ffmpeg
embed/ffmpeg/linux_amd64/ffprobe
embed/ffmpeg/linux_arm64/ffmpeg
embed/ffmpeg/linux_arm64/ffprobe
embed/ffmpeg/darwin_amd64/ffmpeg
embed/ffmpeg/darwin_amd64/ffprobe
embed/ffmpeg/darwin_arm64/ffmpeg
embed/ffmpeg/darwin_arm64/ffprobe
```

Extract from `mwader/static-ffmpeg` Docker image in CI:

```bash
docker create --name ff mwader/static-ffmpeg:8.1
docker cp ff:/ffmpeg embed/ffmpeg/linux_amd64/ffmpeg
docker cp ff:/ffprobe embed/ffmpeg/linux_amd64/ffprobe
docker rm ff
```
