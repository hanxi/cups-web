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
# 选型：debian:bookworm-slim + apt 装 openjdk-17-jdk-headless + curl/ca-certificates，
# Maven 用 Apache 官方 3.9.x 二进制 tarball，不用 Debian 的 maven 包。
#
# 为什么不用 maven:3.9-eclipse-temurin-17：Eclipse Temurin 17 在 Linux ARM 32-bit 上
# Not Supported（Adoptium 官方平台矩阵，JDK 17/21/25 均无 armhf），Maven 官方镜像同样
# 没有 armhf manifest。debian:bookworm-slim 在三架构都有 openjdk-17-jdk-headless，
# ofd-converter 的 maven.compiler.source=1.8，JDK 17 完全能驱动编译与运行。
#
# 为什么不用 Debian 的 apt install maven：QEMU 用户态模拟 armhf 下，Debian 的
# maven 包装完无法启动，会抛 java.lang.ClassNotFoundException: org.apache.maven.cli.MavenCli
# —— 这是上游已知坑（GitHub carlossg/docker-maven#213）。根因是 Debian maven 包依赖
# dpkg triggers + update-alternatives 把 maven-core.jar 等软链进 plexus-classworlds
# 启动器的 classpath，QEMU 模拟下这些 triggers 有时会静默跳过或执行不完整。而 Apache
# 官方 tarball 是纯 Java 且 classpath 完全自包含（lib/ 下所有 jar 都直接可用），
# 同一个 tar.gz 跨架构通吃，不依赖任何 OS 打包细节。
FROM debian:bookworm-slim AS java-builder
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
      openjdk-17-jdk-headless ca-certificates curl \
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
# 只在 VERSION 非空时追加 `-X main.Version=...`，避免传入空值导致 ldflags 拼接出
# `-X main.Version=` 这种含空值的不合法 flag（部分 Go 版本会把空值写成字面空串）。
RUN CGO_ENABLED=0 GOOS=linux \
    go build \
      -ldflags="-s -w $([ -n \"$VERSION\" ] && echo \"-X main.Version=$VERSION\")" \
      -o /out/cups-web ./cmd/server

FROM debian:bookworm-slim AS runtime

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
# 我们把文件写到 /etc/ghostscript/cidfmap.local，并依靠 `pdf_normalize.go` 里的 gs
# 调用显式传入 `-I/etc/ghostscript` + `-c "(cidfmap.local) .runlibfile"`，不依赖任何
# "Debian 自动合并"的约定（不同 gs 版本、不同 Debian patch 的自动加载行为差异很大，
# 最保险是调用侧显式指定）。`.runlibfile` 运行在 gs 的资源加载上下文里，`;` 终止符
# 合法，不会踩到新 PDF 解释器 `-dNEWPDF=true` 下 `/undefined in ;` 的坑。
#
# === 诊断命令 =====================================================================
#   gs -dPDFDEBUG -I/etc/ghostscript -c "(cidfmap.local) .runlibfile" \
#      -dNOPAUSE -dBATCH -sDEVICE=pdfwrite -sOutputFile=/tmp/out.pdf <in.pdf> 2>&1 \
#      | grep -E "Substituting|CIDFSubst|Loading CIDFont"
#   - 命中 cidfmap.local 时日志出现：Substituting font <宋体> from /usr/share/fonts/...
#   - 未命中 cidfmap.local、走第 2 层兜底时出现：substitute from .../DroidSansFallback.ttf
RUN apt-get update && apt-get install -y --no-install-recommends \
    libreoffice-core libreoffice-writer libreoffice-calc libreoffice-impress openjdk-17-jre \
    ghostscript fonts-droid-fallback \
    fonts-dejavu-core fonts-noto-cjk fonts-arphic-uming fonts-arphic-ukai fonts-wqy-zenhei \
    ca-certificates \
  && rm -rf /var/lib/apt/lists/*

# 写入 Ghostscript cidfmap.local：把"空壳 PDF"里的 GBK 字节 BaseFont 精准映射到真实 TTF。
#
# 语法参考：/usr/share/ghostscript/*/Resource/Init/cidfmap（gs 官方示例）
# 用 /#xx 十六进制 name 转义表示 GBK 字节：
#   宋体 = CB CE CC E5  →  /#cb#ce#cc#e5
#   黑体 = BA DA CC E5  →  /#ba#da#cc#e5
#   楷体 = BF AC CC E5  →  /#bf#ac#cc#e5
#   仿宋 = B7 C2 CB CE  →  /#b7#c2#cb#ce
# CSI 固定为 [(GB1) 2] = Adobe-GB1-2，覆盖 GB2312/GBK 常用汉字。
#
# 使用 BuildKit heredoc 格式 `RUN <<EOF` 写入文件（Dockerfile frontend 1.3+ 支持）。
# 注意：不能再用 `RUN cat > file <<'EOF'` 这种 shell heredoc 混合 Dockerfile 指令的
# 写法——后者会让 Docker parser 把 heredoc body 当成下一条 Dockerfile 指令（以 `%!` 开头
# 报 `unknown instruction`）。
RUN <<'CIDFMAP' tee /etc/ghostscript/cidfmap.local > /dev/null
%!
% ---- cidfmap.local: GBK-byte BaseFont 显式映射到 arphic / wqy TrueType 字体 ----
% 本文件由 cups-web Dockerfile 生成，不要手工编辑。
% 通过 `pdf_normalize.go` 里 gs 命令行的 `-I/etc/ghostscript` + `.runlibfile` 显式加载。
%
% 映射命中后 gs pdfwrite 生成的 PDF 里对应字体会走这里指定的字形，而不是
% 默认的 DroidSansFallback（唯一无衬线字重）。
%
% 策略说明：arphic / wqy 是单字重字库，没有 Bold 配套文件，gs 也不会做 synthetic bold。
% 因此 Bold 变体通过"映射到另一套更粗的字体"来制造视觉粗细差（宋体 Bold → wqy，
% 仿宋 Bold → wqy）；wqy-zenhei 本身就是本镜像里最粗的中文字体，没有更粗的可换，
% 所以黑体/楷体的 Bold 与 Regular 视觉相同，属于字库本身的限制。

% 宋体 (CB CE CC E5)
/#cb#ce#cc#e5 << /FileType /TrueType /Path (/usr/share/fonts/truetype/arphic/uming.ttc) /SubfontID 0 /CSI [(GB1) 2] >> ;
/#cb#ce#cc#e5,Bold << /FileType /TrueType /Path (/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc) /SubfontID 0 /CSI [(GB1) 2] >> ;

% 黑体 (BA DA CC E5)
/#ba#da#cc#e5 << /FileType /TrueType /Path (/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc) /SubfontID 0 /CSI [(GB1) 2] >> ;
/#ba#da#cc#e5,Bold << /FileType /TrueType /Path (/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc) /SubfontID 0 /CSI [(GB1) 2] >> ;

% 楷体 (BF AC CC E5)
/#bf#ac#cc#e5 << /FileType /TrueType /Path (/usr/share/fonts/truetype/arphic/ukai.ttc) /SubfontID 0 /CSI [(GB1) 2] >> ;
/#bf#ac#cc#e5,Bold << /FileType /TrueType /Path (/usr/share/fonts/truetype/arphic/ukai.ttc) /SubfontID 0 /CSI [(GB1) 2] >> ;

% 仿宋 (B7 C2 CB CE)
/#b7#c2#cb#ce << /FileType /TrueType /Path (/usr/share/fonts/truetype/arphic/uming.ttc) /SubfontID 0 /CSI [(GB1) 2] >> ;
/#b7#c2#cb#ce,Bold << /FileType /TrueType /Path (/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc) /SubfontID 0 /CSI [(GB1) 2] >> ;
CIDFMAP
# 构建期自检：确保文件写入成功、条目数对得上。不在构建期用 gs 解析这个文件，因为
# `.runlibfile` 必须配合 `-I` 才能工作，而且 gs 加载资源要占用额外的子进程空间，
# 运行时首次 gs 调用会做真正的解析验证，构建期只做结构性检查。
RUN test -s /etc/ghostscript/cidfmap.local \
  && echo "[dockerfile] cidfmap.local size: $(wc -c < /etc/ghostscript/cidfmap.local) bytes" \
  && entries=$(grep -cE '^/#' /etc/ghostscript/cidfmap.local) \
  && echo "[dockerfile] cidfmap.local entries: $entries (expect 8)" \
  && test "$entries" = "8"

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
