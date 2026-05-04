# syntax=docker/dockerfile:1
#
# 顶部的 `# syntax=docker/dockerfile:1` 声明要求 BuildKit 前端——`BUILDPLATFORM`
# 这类自动变量由 BuildKit 注入，若缺失声明，部分旧 buildx 环境会把它当成
# 未设置，导致 `--platform=$BUILDPLATFORM` 退化成 `--platform=`（默认 target），
# 跨架构构建时 java-builder 会重新落回 QEMU 模拟，前功尽弃。

# ---- Frontend build ----
# 使用 node:20-slim 替代 oven/bun：Bun 官方不支持 32-bit ARM（#5060 Closed as not planned），
# 会导致 linux/arm/v7 构建直接找不到 manifest。node:20-slim 官方镜像覆盖 amd64/arm32v7/arm64v8，
# 而 frontend/package.json 里 scripts 全是标准 Vite/Node 命令，完全不依赖 bun 专有 API，
# 用 npm ci 替换 bun install 即可获得跨三架构的一致构建产物。
FROM node:20-slim AS frontend-build
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci --no-audit --no-fund --prefer-offline
COPY frontend ./
RUN npm run build

# ---- Java OFD converter build ----
#
# 关键点：`FROM --platform=$BUILDPLATFORM ...` 把本阶段锁在 **host 本地架构**（CI 上是 amd64），
# 不跟随 buildx 的 TARGETPLATFORM 走进 QEMU armhf 模拟。这么做的核心理由：
#
#   QEMU 用户态模拟 armhf 下，OpenJDK 17 不稳定。Maven 无论是用 Debian 的 `apt install maven`
#   还是 Apache 官方 Maven 3.9.x 二进制 tarball，启动时都会随机抛
#   `java.lang.ClassNotFoundException: org.apache.maven.cli.MavenCli`（堆栈完全一致，只差
#   classworlds 版本行号），这说明问题在 JVM 层——具体是 QEMU 模拟下的 ClassLoader /
#   JIT 稳定性问题，而非 Maven 安装方式。侧面佐证：Adoptium Temurin JDK 17/21/25 对 Linux
#   ARM 32-bit Hard-Float **官方不支持**（仅 JDK 8/11 有 armhf 二进制），说明 ARM 32-bit 上的
#   现代 JVM 本来就是薄弱环节，在 QEMU 下更是雪上加霜。
#
# 由于 ofd-converter 是纯 Java 项目（maven.compiler.source=1.8 → target JVM bytecode），
# 产物 `.jar` 跨架构通吃，让 java-builder 固定跑在 amd64 上构建一次、各架构的 runtime
# 统一 `COPY --from=java-builder` 这份纯字节码 jar，是 Docker 官方推荐的多架构 Java
# 最佳实践，也是 `BUILDPLATFORM` 这个自动变量最典型的用法。
#
# 其他保留说明：
# - 基础镜像使用 debian:trixie-slim（和 runtime 阶段统一，减少下载层）；
# - Maven 仍用 Apache 官方 tarball（`apt install maven` 依赖 dpkg triggers 更新
#   update-alternatives 软链，虽然 host amd64 不会触发 QEMU 坑，但 tarball 方式更自包含，
#   跨 base 镜像升级无副作用）；
# - 即便某天 GitHub Actions 的 runner 换成 arm64/linux，`BUILDPLATFORM` 也会自动跟随，
#   那时 amd64 的 runtime 反而要 QEMU 模拟 java-builder——但 amd64 上的 JVM 远比 armhf 稳，
#   在实践中是可接受的。真正要彻底脱离 QEMU，只能靠 GH Actions 的 multi-runner 矩阵拆分。
FROM --platform=$BUILDPLATFORM debian:trixie-slim AS java-builder
ENV DEBIAN_FRONTEND=noninteractive
ENV MAVEN_VERSION=3.9.9
ENV MAVEN_HOME=/opt/maven
ENV PATH=/opt/maven/bin:$PATH
# Maven tarball 来源策略：
# 1) 优先 dlcdn.apache.org（Apache CDN，快）—— 但它只保留 current release，
#    一旦官方发布 3.9.10+，3.9.9 会立即 404，CI 会挂（exit code 22）。
# 2) Fallback 到 archive.apache.org/dist/maven/...（永久归档，所有历史版本都在）。
# 这样日常走 CDN 快，被 dlcdn 抛弃后自动用归档也能跑通，升级 Maven 时只需改 MAVEN_VERSION。
RUN apt-get update && apt-get install -y --no-install-recommends \
      openjdk-21-jdk-headless ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && ( \
        curl -fsSL "https://dlcdn.apache.org/maven/maven-3/${MAVEN_VERSION}/binaries/apache-maven-${MAVEN_VERSION}-bin.tar.gz" -o /tmp/maven.tar.gz \
        || curl -fsSL "https://archive.apache.org/dist/maven/maven-3/${MAVEN_VERSION}/binaries/apache-maven-${MAVEN_VERSION}-bin.tar.gz" -o /tmp/maven.tar.gz \
       ) \
    && mkdir -p "${MAVEN_HOME}" \
    && tar -xzf /tmp/maven.tar.gz -C "${MAVEN_HOME}" --strip-components=1 \
    && rm -f /tmp/maven.tar.gz \
    && mvn -version
WORKDIR /src/ofd-converter
COPY ofd-converter/pom.xml ./
RUN mvn dependency:go-offline -q
COPY ofd-converter/src ./src
RUN mvn clean package -q -DskipTests

FROM golang:1.26 AS builder
WORKDIR /src

# 构建期注入版本号（Issue #26）：
#   - CI (docker-publish.yml) 会通过 `--build-arg VERSION=${{ github.ref_name }}` 传入
#     形如 `v1.2.3` 的 tag 名；push master 分支时会传入分支名 `master`。
#   - 本地 `make docker-build` 会把 `git describe --tags --always --dirty` 透传进来。
#   - 未指定时保持空字符串，让 main.Version 保持默认 "dev"，便于区分"未注入"与"注入失败"。
ARG VERSION=""

# copy go modules and source
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://goproxy.cn,direct
RUN go mod download
COPY . .
# Copy built frontend assets into expected location for go:embed
COPY --from=frontend-build /src/frontend/dist ./frontend/dist

# Build the Go binary (frontend must be built before this step in CI/local)
#
# ldflags 注入版本号的写法说明：
#
# 以前尝试过条件写法 `$([ -n "$VERSION" ] && echo "-X main.Version=$VERSION")`
# 在 Docker RUN 的 shell form 下会挂——因为 Dockerfile 会先把 $VERSION 展开，
# 导致 shell 实际看到的是四层嵌套双引号的 `-ldflags="-s -w $(... "master" ...
# "-X main.Version=master")"`，中间那对 `"master"` 把外层 -ldflags="..."
# 提前闭合，于是 `-X main.Version=master` 被切开，链接器只收到 `-X main.Version`，
# 报 `flag provided but not defined: -X main.Version`。
#
# 修复思路：Go 对 `-X main.Version=`（空串）是合法接受的（等价于 var Version = ""，
# 与默认值 "dev" 在显示上有区别但不会报错），所以无需条件判断，直接无条件
# 拼 `-X main.Version=$VERSION`，让 Docker ARG 展开去把 $VERSION 变成空串或
# 实际值，shell 里只剩一对引号，彻底避开嵌套转义。
RUN CGO_ENABLED=0 GOOS=linux \
    go build \
      -ldflags="-s -w -X main.Version=$VERSION" \
      -o /out/cups-web ./cmd/server

FROM debian:trixie-slim AS runtime

# Install LibreOffice (headless conversion), Ghostscript, and minimal fonts/certificates
#
# === 中文字体在容器里的三层兜底（从"精准"到"保底"） =============================
#
# 第 1 层（最精准）——cidfmap.local：
#   针对 Acrobat/WPS 导出的"空壳 Type0 + UniGB-UCS2-H + GBK 字节 BaseFont"这类 PDF
#   （/BaseFont /#ba#da#cc#e5 即"黑体"的 GBK 字节，准考证/国标表格最常见），由我们在
#   下面的 RUN 里手动写入 /etc/ghostscript/cidfmap.local，把 8 个 GBK 字节名显式映射到
#   本镜像自带的真实 TrueType 字体（宋/黑/楷/仿宋 × Regular/Bold 各 1 条共 8 条）。
#
#   因为 arphic-uming / arphic-ukai / wqy-zenhei 都是**单字重 TrueType**（没有配套的
#   Bold 文件），gs pdfwrite 在重建字体字典时也不会做 synthetic bold——它只会照抄
#   `,Bold` 后缀进新字体名，实际字形仍是 Regular。因此我们用"换字体制造视觉粗细差"
#   的策略：
#     宋体 Regular → AR PL UMing CN（衬线细）     宋体 Bold → WenQuanYi Zen Hei（无衬线粗）
#     黑体 Regular → WenQuanYi Zen Hei            黑体 Bold → WenQuanYi Zen Hei（同文件，视觉差小，已是最粗可用字体）
#     楷体 Regular → AR PL UKai CN（楷体手写）    楷体 Bold → AR PL UKai CN（同上）
#     仿宋 Regular → AR PL UMing CN（明朝兜底）   仿宋 Bold → WenQuanYi Zen Hei（制造粗细差）
#   这样纸面上至少能看出"标题比正文粗"的视觉层级，而不是全部糊成单一字重。楷体 Bold
#   受限于 arphic/wqy 字库没有对应字体，视觉上仍与 Regular 相同，这是字库本身的限制。
#
#   之所以只选纯 TrueType 字体（arphic/wqy）而非 Noto CJK，是因为 Ghostscript 10.x
#   对 CFF-based OpenType Collection（如 Noto CJK OTC）在 CIDFont 子字体索引上偶有坑，
#   TrueType TTC 最稳。
#
# 第 2 层——fonts-droid-fallback（兜底 CID 字体）：
#   cidfmap.local 没覆盖到的 GBK/GB1 字体名（例如个别厂商自造字体名），gs 会回落到
#   Resource/CIDFSubst/DroidSansFallback.ttf 这个按 Adobe-GB1 CID 编号组织的字体，
#   与 UniGB-UCS2-H 推导的 CID 对齐。Debian 把这个字体拆成独立的 fonts-droid-fallback
#   包（ghostscript 主包里的路径是指向它的软链接），缺包就会出现中文变"豆腐块"
#   （macOS brew 的 ghostscript 自带该字体，本地测试不会踩到；Docker 里才会）。
#
# 第 3 层——fonts-noto-cjk / fonts-arphic-* / fonts-wqy-zenhei（Unicode 字形库）：
#   给 LibreOffice headless 渲染 Office 文档时用；按 Unicode 组织、不按 CID，因此
#   不能替代第 1/2 层在 gs CIDFSubst 路径的角色。
#
# === cidfmap.local 的加载路径（重要） ============================================
# 我们把文件写到 /etc/ghostscript/cidfmap.local，并在构建阶段将其复制为
# gs Resource/Init/cidfmap，trixie 的 gs 10.05.1 默认不存在该文件，
# 直接创建后 gs 启动时自动加载，无需任何额外命令行参数。
#
# === 诊断命令 =====================================================================
#   gs -dPDFDEBUG \
#      -dNOPAUSE -dBATCH -sDEVICE=pdfwrite -sOutputFile=/tmp/out.pdf <in.pdf> 2>&1 \
#      | grep -E "Substituting|CIDFSubst|Loading CIDFont"
#   - 命中 cidfmap.local 时日志出现：Substituting font <宋体> from /usr/share/fonts/...
#   - 未命中 cidfmap.local、走第 2 层兜底时出现：substitute from .../DroidSansFallback.ttf
RUN apt-get update && apt-get install -y --no-install-recommends \
    libreoffice-core libreoffice-writer libreoffice-calc libreoffice-impress openjdk-21-jre \
    ghostscript fonts-droid-fallback \
    fonts-dejavu-core fonts-noto-cjk fonts-arphic-uming fonts-arphic-ukai fonts-wqy-zenhei \
    ca-certificates \
  && rm -rf /var/lib/apt/lists/*

# 自定义字体：用户可将 SimSun/SimHei 等 Windows 字体放入 docker-fonts/ 目录，
# 构建时自动安装到镜像中，以获得更接近原始 PDF 的字体渲染效果。
# 如果 docker-fonts/ 目录为空（只有 README.md），此步骤不会报错。
COPY docker-fonts/ /tmp/docker-fonts/
RUN mkdir -p /usr/share/fonts/truetype/custom && \
    find /tmp/docker-fonts -type f \( -iname '*.ttf' -o -iname '*.ttc' -o -iname '*.otf' \) \
      -exec cp {} /usr/share/fonts/truetype/custom/ \; && \
    fc-cache -f /usr/share/fonts/truetype/custom 2>/dev/null || true

# 安装 fontconfig 中文字体别名配置（从 docker-fonts/fontconfig-chinese.conf 复制）
# 原因：LibreOffice fallback 渲染路径依赖 fontconfig 查找"宋体""黑体"等中文字体名
# simsun.ttf 用于宋体映射（已从 TTC 替换为 TTF 单体格式，解决 gs 10.x 和 fontconfig 兼容性问题）
RUN cp /tmp/docker-fonts/fontconfig-chinese.conf /etc/fonts/conf.d/05-custom-chinese-fonts.conf 2>/dev/null || true
RUN fc-cache -f 2>/dev/null || true

# 安装 Ghostscript cidfmap.local（从 docker-fonts/cidfmap.local 复制）
#
# 语法参考：/usr/share/ghostscript/*/Resource/Init/cidfmap（gs 官方示例）
# 用 /#xx 十六进制 name 转义表示 GBK 字节：
#   宋体 = CB CE CC E5  →  /#cb#ce#cc#e5
#   黑体 = BA DA CC E5  →  /#ba#da#cc#e5
#   楷体 = BF AC CC E5  →  /#bf#ac#cc#e5
#   仿宋 = B7 C2 CB CE  →  /#b7#c2#cb#ce
# CSI 固定为 [(GB1) 2] = Adobe-GB1-2，覆盖 GB2312/GBK 常用汉字。
RUN mkdir -p /etc/ghostscript && \
    cp /tmp/docker-fonts/cidfmap.local /etc/ghostscript/cidfmap.local && \
    rm -rf /tmp/docker-fonts
# 构建期自检：确保文件写入成功、条目数对得上。不在构建期用 gs 解析这个文件，因为
# `.runlibfile` 必须配合 `-I` 才能工作，而且 gs 加载资源要占用额外的子进程空间，
# 运行时首次 gs 调用会做真正的解析验证，构建期只做结构性检查。
RUN test -s /etc/ghostscript/cidfmap.local \
  && echo "[dockerfile] cidfmap.local size: $(wc -c < /etc/ghostscript/cidfmap.local) bytes" \
  && entries=$(grep -cE '^/#' /etc/ghostscript/cidfmap.local) \
  && echo "[dockerfile] cidfmap.local entries: $entries (expect 8)" \
  && test "$entries" = "8"

# 如果用户提供了 SimSun/SimHei/SimKai/SimFang 字体，更新 cidfmap.local 映射，
# 用真实 Windows 字体替换 arphic/wqy 的 fallback 映射，获得更精确的渲染效果。
# 注意：宋体(Regular+Bold)和仿宋(Regular+Bold)的基准路径都是 uming.ttc，
# simsun.ttf 先做全局替换，simfang.ttf 再通过行匹配 /^#b7#c2#cb#ce/ 精确覆盖仿宋行。
RUN if [ -f /usr/share/fonts/truetype/custom/simsun.ttf ]; then \
      sed -i 's|/usr/share/fonts/truetype/arphic/uming.ttc) /SubfontID 0|/usr/share/fonts/truetype/custom/simsun.ttf) /SubfontID 0|g' /etc/ghostscript/cidfmap.local; \
      echo "[dockerfile] cidfmap: simsun.ttf mapped (宋体 Regular+Bold, 仿宋 pending)"; \
    fi && \
    if [ -f /usr/share/fonts/truetype/custom/simhei.ttf ]; then \
      sed -i 's|/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc) /SubfontID 0|/usr/share/fonts/truetype/custom/simhei.ttf) /SubfontID 0|g' /etc/ghostscript/cidfmap.local; \
      echo "[dockerfile] cidfmap: simhei.ttf mapped (黑体 Regular+Bold)"; \
    fi && \
    if [ -f /usr/share/fonts/truetype/custom/simkai.ttf ]; then \
      sed -i 's|/usr/share/fonts/truetype/arphic/ukai.ttc) /SubfontID 0|/usr/share/fonts/truetype/custom/simkai.ttf) /SubfontID 0|g' /etc/ghostscript/cidfmap.local; \
      echo "[dockerfile] cidfmap: simkai.ttf mapped (楷体 Regular+Bold)"; \
    fi && \
    if [ -f /usr/share/fonts/truetype/custom/simfang.ttf ]; then \
      sed -i '/^\/#b7#c2#cb#ce[, ]/s|/usr/share/fonts/truetype/custom/simsun.ttf) /SubfontID 0|/usr/share/fonts/truetype/custom/simfang.ttf) /SubfontID 0|' /etc/ghostscript/cidfmap.local; \
      sed -i '/^\/#b7#c2#cb#ce[, ]/s|/usr/share/fonts/truetype/arphic/uming.ttc) /SubfontID 0|/usr/share/fonts/truetype/custom/simfang.ttf) /SubfontID 0|' /etc/ghostscript/cidfmap.local; \
      echo "[dockerfile] cidfmap: simfang.ttf mapped (仿宋 Regular+Bold)"; \
    fi

# 将 cidfmap.local 作为 gs 的默认 cidfmap 安装到 Resource/Init/
# trixie 的 gs 10.05.1 默认不存在 cidfmap（只有 FAPIcidfmap），直接创建即可。
# gs 启动时自动加载 Resource/Init/cidfmap，无需额外命令行参数。
RUN GS_INIT_DIR=$(find /usr/share/ghostscript -path "*/Resource/Init" -type d | head -1) && \
    if [ -n "$GS_INIT_DIR" ] && [ -f /etc/ghostscript/cidfmap.local ]; then \
        cp /etc/ghostscript/cidfmap.local "$GS_INIT_DIR/cidfmap" && \
        echo "[dockerfile] cidfmap.local -> $GS_INIT_DIR/cidfmap"; \
    fi

# Create a non-root user for running the service
RUN groupadd -r nonroot && useradd -r -g nonroot nonroot

RUN mkdir -p \
    /home/nonroot/.cache/dconf \
    /home/nonroot/.config/libreoffice \
    /home/nonroot/.local/share/libreoffice \
  && chown -R nonroot:nonroot /home/nonroot/ \
  && chmod -R 755 /home/nonroot/ \
  && chmod 700 /home/nonroot/.cache/dconf

ENV DCONF_USER_CONFIG_DIR=/home/nonroot/.config/dconf
ENV HOME=/home/nonroot
ENV XDG_CACHE_HOME=/home/nonroot/.cache

COPY --from=builder /out/cups-web /cups-web
COPY --from=java-builder /src/ofd-converter/target/ofd-converter.jar /ofd-converter.jar
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/cups-web"]
