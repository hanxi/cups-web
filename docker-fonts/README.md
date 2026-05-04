# docker-fonts - 自定义字体目录

本目录存放 Docker 构建时使用的中文 TrueType 字体，用于 Ghostscript cidfmap 映射。

## 当前包含的字体

| 文件名 | 字体名称 | 格式 | 大小 |
|---------|----------|------|------|
| `simsun.ttc` | 宋体 (SimSun) | TrueType Collection | ~10 MB |
| `simhei.ttf` | 黑体 (SimHei) | TrueType | ~9.3 MB |
| `simkai.ttf` | 楷体 (SimKai) | TrueType | ~11 MB |
| `simfang.ttf` | 仿宋 (SimFang) | TrueType | ~10 MB |

这些是 Windows 系统中文字体，与 Ghostscript cidfmap `/FileType /TrueType` 映射完全兼容。

## 为什么必须是 TrueType 格式

Ghostscript 的 cidfmap 使用 `/FileType /TrueType` 声明字体类型。如果提供 OTF (CFF) 格式字体，
gs 无法正确解析字形数据，会导致输出 PDF 中文字变成乱码方块。因此本目录**只接受 `.ttf` 或 `.ttc` 格式**。

## 字体映射关系

构建时 Dockerfile 会自动将这些字体映射到 `/etc/ghostscript/cidfmap.local`：

| GBK 名称 | 映射字体 |
|-----------|----------|
| 宋体 (CB CE CC E5) Regular | simsun.ttc |
| 宋体 Bold | simhei.ttf |
| 黑体 (BA DA CC E5) Regular/Bold | simhei.ttf |
| 楷体 (BF AC CC E5) Regular/Bold | simkai.ttf |
| 仿宋 (B7 C2 CB CE) Regular | simfang.ttf |
| 仿宋 Bold | simhei.ttf |

## 使用

1. 正常执行 `docker build` 或 `make docker-build`
2. 构建过程会自动安装字体并更新 Ghostscript 的字体映射

## 注意事项

- **SimSun/SimHei 等 Windows 字体为微软版权所有**，仅限个人/内部使用，请勿将包含这些字体的 Docker 镜像公开分发
- 仅支持 TrueType 格式（`.ttf` / `.ttc`），不要放 `.otf` 文件
- 如果缺少某个字体文件，Dockerfile 会自动跳过对应映射，退回到系统 arphic/wqy 兜底字体
