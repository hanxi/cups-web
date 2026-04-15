#!/bin/bash

# 版本升级脚本
# 用法: ./bump-version.sh [major|minor|patch]
# 默认: patch

set -e

# 获取当前目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# 获取升级类型
BUMP_TYPE="${1:-patch}"

# 验证输入
if [[ ! "$BUMP_TYPE" =~ ^(major|minor|patch)$ ]]; then
    echo "错误: 无效的升级类型 '$BUMP_TYPE'"
    echo "用法: $0 [major|minor|patch]"
    exit 1
fi

# 获取最新的 tag
LATEST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
echo "当前最新 tag: $LATEST_TAG"

# 解析版本号
VERSION=${LATEST_TAG#v}
MAJOR=$(echo "$VERSION" | cut -d. -f1)
MINOR=$(echo "$VERSION" | cut -d. -f2)
PATCH=$(echo "$VERSION" | cut -d. -f3)

# 根据类型升级版本
case "$BUMP_TYPE" in
    major)
        MAJOR=$((MAJOR + 1))
        MINOR=0
        PATCH=0
        ;;
    minor)
        MINOR=$((MINOR + 1))
        PATCH=0
        ;;
    patch)
        PATCH=$((PATCH + 1))
        ;;
esac

NEW_VERSION="v${MAJOR}.${MINOR}.${PATCH}"
echo "新版本: $NEW_VERSION"

# 检查工作区是否干净
if ! git diff-index --quiet HEAD --; then
    echo "错误: 工作区有未提交的更改，请先提交或暂存"
    exit 1
fi

# 创建并推送 tag
echo "创建 tag: $NEW_VERSION"
git tag "$NEW_VERSION"

echo "推送 tag 到远程..."
git push origin "$NEW_VERSION"

echo "✅ 版本升级完成: $LATEST_TAG -> $NEW_VERSION"
echo "GitHub Actions 将自动构建并发布新版本"
