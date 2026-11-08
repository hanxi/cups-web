# PDF 标准化管线与打印流水

> 本文档是 [AGENTS.md](../AGENTS.md)「打印流水」章节的深度补充，收录标准化管线原理、Ghostscript cidfmap 机制与已知副作用。

## 打印流水总览

`printHandler`（`cmd/server/print_handlers.go`）是核心入口，流程：

1. **接收**：解析 multipart 表单（上限 512MB），提取 `file` + 打印参数
2. **落盘**：`saveUploadedFile` 将上传文件按日期分目录保存到 `uploads/YYYYMMDD/` 下，文件名做安全化处理
3. **类型识别 & 转换**（`detectFileKind`）：
   - `pdf` → **PDF 标准化管线**（见下）
   - `office` → `convertOfficeToPDF`（调 `libreoffice --headless --convert-to pdf`）
   - `ofd` → `convertOFDToPDF`（调 `java -jar /ofd-converter.jar`）
   - `image` → `convertImageToPDF`（用 `gofpdf` 渲染；长边超过 3000px 的大图会先经 `downscaleImageIfNeeded` 下采样到 3000px 并以 JPEG Q85 重编码再嵌入 PDF，避免把手机端 10MB+ 原图整张塞进 PDF 导致移动端预览/下载超时，见 [Issue #22](https://github.com/hanxi/cups-web/issues/22)；PNG 透明像素会被合成到白底以符合打印预期）
   - `text` → `convertTextToPDF`（用 `gofpdf` + 内嵌中文字体渲染）
4. **页数统计**：`countPDFPages` / `countPDFPagesWithFallback` / `estimateTextPages`；PDF 页数读取失败时走 `normalizePDF` 再重试，仍失败则以 1 页兜底而非直接 400
5. **持久化**：在 `print_jobs` 插入一条 `queued` 记录
6. **提交打印**：`ipp.SendPrintJob` 构造 `Print-Job` IPP 请求并发出
7. **回写状态**：成功后更新为 `printed` 并回填 `job_id`

转换或标准化后的 PDF 以 `<原文件>.print.pdf` 副文件形式存到 `uploads/`，维护任务清理时会连同原文件一起删除。`/api/convert` 对 PDF 也会走同一条 `normalizePDF` 管线，让前端 `PdfCanvas` 预览与最终打印使用完全相同的字节流。

## 可选图片压缩（Issue #119）

标准打印选择图片后，可勾选「图片压缩」，默认关闭。启用后，上传前先在浏览器将每张图片压到 800KB（800,000 字节）以内，再转成 PDF 预览和打印。混合文件批量打印中的图片也使用该设置；切换开关会使已有的图片转换结果失效，下一次转换或打印按新设置重新生成。

`POST /api/convert`（单图 `file` / 多图 `files`）和 `POST /api/print`（图片 `file`）支持 multipart 字段 `compress_images=true`。后端在原有下采样、旋转和反色之后检查编码大小：先尝试 JPEG 质量 85、75、65、55、45，仍超过上限时等比缩小尺寸并重试，透明区域合成到白底。压缩失败返回错误，不静默提交超限图片。未启用时保持原有行为。

上限针对每张图片的编码数据，不限制整份 PDF，也不等同于打印机内存占用；CUPS 驱动仍会转换打印数据，是否能解决 HL1110 的失败需结合实际驱动、样图和日志复测。浏览器启用压缩后提交的是生成的 PDF，历史记录重打使用这份 PDF；直接 API 上传原图时，`compress_images` 是本次转换选项，不落库，原图重打不继承该选项。此功能不重新压缩已有 PDF、Office 文档或定时任务中的图片。

## PDF 标准化管线（`normalizePDF`）

`diagnosePDF` 诊断日志 → `normalizePDF`：

1. **Ghostscript `pdfwrite`**（优先）：`-dCompatibilityLevel=1.4 -dEmbedAllFonts=true`，两档重试：strict `/prepress` → lenient `-dNEWPDF=false -dPDFSTOPONERROR=false`
2. **LibreOffice `--convert-to pdf`**（兜底）
3. **passthrough**（最终降级）

**该管线只解决"CUPS 老驱动拒绝 PDF-1.7 新语法"这一类真正的兼容性故障**，对"预览显示"不会有帮助：gs 会把空壳 CJK 字体改写成带 subset 前缀的假嵌入字体，反而让浏览器 pdf.js 在预览时出现错位（详见前端 `PdfCanvas.vue` 的 `getDocument` 参数注释）。因此 `/api/convert` 预览入口应该**优先让 pdf.js 直接读原始 PDF**，只在真实打印前做最小化标准化。

## Ghostscript cidfmap：中文字形映射的两套加载机制

打印纸面中文字形的配套：镜像把 `docker-fonts/cidfmap.local` 里宋/黑/楷/仿宋（Regular + Bold，共 8 条 GBK 字节 BaseFont）到 `arphic-uming` / `arphic-ukai` / `wqy-zenhei` 三套 TrueType 字体的映射交给 gs，让 gs pdfwrite 在重建字体字典时能按字体名落到不同字形上，而不是全部坍缩成单一 `DroidSansFallback` 无衬线体。

**加载路径现在是两套并存**，两者都保留，各管一段场景（改动任一侧前请先读 `Dockerfile` 与 `cmd/server/pdf_normalize.go` 的注释）：

### 1. 主路径 —— gs 自动加载 `Resource/Init/cidfmap`（Docker 内）

`Dockerfile` 构建期先 `cp docker-fonts/cidfmap.local /etc/ghostscript/cidfmap.local`（并按用户自备的 `simsun/simhei/simkai/simfang.ttf` 用 `sed` 替换映射目标），再 `find /usr/share/ghostscript -path "*/Resource/Init"` 把同一份文件复制成 gs 的 `Resource/Init/cidfmap`。gs 启动时会自动加载 `Resource/Init/cidfmap`，**不需要任何命令行参数**。trixie 的 gs 10.05.1 默认不存在 `cidfmap`（只有 `FAPIcidfmap`），所以直接创建即可，不涉及和发行版文件的合并冲突。构建期还有一步自检：`test -s` + `grep -cE '^/#'` 必须等于 8 条，条目数不对就让构建失败。

### 2. 兼容路径 —— `pdf_normalize.go::cidfmapPreambleArgs()`（仍在代码里，仍被调用）

`tryGhostscriptRun` 每次拼 gs 命令行时都会 `append(args, cidfmapPreambleArgs()...)`。它现在**只**在 `/etc/ghostscript/cidfmap.local`（变量 `cidfmapSystemPath`）存在时返回**一个** `-I<dir>` 搜索路径参数，文件不存在（macOS 本地开发）时返回 `nil`，命令行退化为未打补丁前的形态。

**历史上曾经拼的 `-c "(cidfmap.local) .runlibfile" -f` 显式加载已经删掉**——`Resource/Init/cidfmap` 自动加载后它成了重复动作。`pdf_normalize_test.go::TestCidfmapPreambleArgs` 就在锁这两个分支（不存在 → `nil`；存在 → 恰好 1 个 `-I` 参数指向 cidfmap 所在目录），改这个函数会直接把测试打红。

### 字体映射与诊断

由于 arphic/wqy 都是**单字重字库**，gs 也不做 synthetic bold，Bold 变体只能通过"换字体制造视觉粗细差"——当前策略是宋体 Bold / 仿宋 Bold → `wqy-zenhei`（本镜像最粗的中文字体），黑体/楷体的 Bold 与 Regular 同源、视觉一致，属字库本身限制。

诊断方式：

```bash
gs -dPDFDEBUG -dNOPAUSE -dBATCH -sDEVICE=pdfwrite -sOutputFile=/tmp/out.pdf <in.pdf> 2>&1 | grep -E "Substituting|CIDFSubst"
```

命中 cidfmap 会看到 `Substituting font ... from /usr/share/fonts/truetype/...`；未命中才回落到 `DroidSansFallback`。

新增映射条目时，GBK 字节 → PostScript name 换算关系：宋体=`cb ce cc e5`、黑体=`ba da cc e5`、楷体=`bf ac cc e5`、仿宋=`b7 c2 cb ce`，CSI 固定用 `[(GB1) 2]`；改完记得同步 `Dockerfile` 里那句"expect 8"的自检数字。

## 已知副作用：空壳 CJK 字体与 pdf.js 预览错位

> ⚠️ Acrobat 导出的"空壳 Type0 + `UniGB-UCS2-H`"字体字典（`/BaseFont /#ba#da#cc#e5` 这种裸宋体名，准考证/国标表格常见）经 gs 改写为"subset 前缀 + FontFile2 假内嵌"后（`/BaseFont /CCGWER+#ba#da#cc#e5`），**pdf.js** 预览会出现"每 3-4 字错 1 字"的挤压错位（浏览器原生 PDF 引擎因有系统字体兜底不受影响）。之所以仍然共用 `normalizePDF`，是因为"预览与打印看到同一份字节流"的一致性比这类特殊 PDF 的预览准确性更重要——前端只使用 `pdfjs-dist` 在 canvas 里渲染预览（见 `frontend/src/components/print/PdfCanvas.vue`），遇到上述错位时用户可以忽略，不影响打印。

## 自定义中文字体挂载与排版保真

为解决公文打印（如仿宋_GB2312、方正小标宋简体）与商务文档转换时因缺少字体引起的断行错位、排版走样问题：

### 1. 外部卷挂载与启动同步
- **挂载路径**：宿主机字体目录挂载至容器内的 `/data/fonts`（在默认 Compose 配置下，直接放入 `./.data/fonts/` 即可）。
- **entrypoint 同步（步骤 1b）**：容器启动时自动扫描 `/data/fonts` 下所有 `.ttf` / `.ttc` / `.otf` 文件，自动建立软链接至 `/usr/share/fonts/truetype/custom` 并执行 `fc-cache -f`。
- **Ghostscript 动态修补**：若挂载了真实的 Windows 基础字库（`simsun.ttf`、`simhei.ttf`、`simkai.ttf`、`simfang.ttf`），脚本会自动修补 `/etc/ghostscript/cidfmap.local` 并同步覆盖至 `$GS_INIT_DIR/cidfmap`，使 Ghostscript `pdfwrite` 管线直接调用原生字体字形，避免回退到开源替代字体引起字重或度量不一致。

### 2. Web 界面可视化字体管理（`/fonts`）
管理员可直接在 Web 页面进行字体管理，无需频繁手动进入容器：
- **独立路由与直达入口**：提供 `/#/fonts` 独立页面，管理员在桌面端与移动端导航栏均可直接点击「字体」进入。
- **二进制元数据解析（`font_parser.go`）**：针对 TrueType、OpenType 与 TrueType Collection（TTC）实现标准 SFNT `name` 表解码器，基于 UTF-16BE 提取 Name ID 1（Family）、2（Subfamily）、4（Full Name）与 16（Preferred Family）。即使用户上传或挂载的是形如 `1.ttf` 的文件，系统也能解析出真实名称 `Arial`，中文字体展现为 `仿宋 (FangSong)`，TTC 展开为 `宋体 (SimSun) / 新宋体 (NSimSun)`。
- **动态生效**：Web 端上传或删除字体后，后端立即自动同步系统软链、执行 `fc-cache -f`、修补 `cidfmap` 并调用 `InvalidateSystemFontCache()`，无需重启容器即可热生效。

### 3. 文档字体预检（Font Preflight）
- **触发链路**：在 `/api/print` 和 `/api/convert` 接收 `.docx` 或未完全内嵌字体的 `.pdf` 时，服务端自动提取文档所需的字体族名称，并与系统已注册字体库（含常见别名与 GBK 十六进制 BaseFont）比对。
- **阻断机制**：检测到缺失字体且未启用替换时，接口返回 `422 Unprocessable Entity`（错误码 `MISSING_FONTS`）。前端弹出警告模态框，直观展示缺失字体列表，提供「取消」「本次允许替换」与「始终允许替换」选项，杜绝静默打出格式错误的废纸。
- **全局控制**：在「管理后台 → 系统设置」中提供「允许缺少字体时自动替换」开关（落库至 `settings` 表的 `allow_font_fallback` 键），可按需关闭预检严格拦截。

## HTTP 超时

`cmd/server/main.go` 的 `http.Server` 配置为 `ReadTimeout = WriteTimeout = IdleTimeout = 120s`。之所以放宽到 2 分钟，是因为 `/api/convert` 与 `/api/print` 在移动端场景需要：上传 10MB+ 原图 → 服务端下采样/标准化 → 回传 PDF，整条链路在 4G 网络下 15s 远远不够（[Issue #22](https://github.com/hanxi/cups-web/issues/22)）。如果未来要对个别接口设置更激进的独立超时，建议用 `http.TimeoutHandler` 包住具体子路由，而不是再调低全局值。
