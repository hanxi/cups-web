# CUPS Web 网站

简体中文 · [English](README.en.md)

仓库的 GitHub Pages 站点包含产品首页和技术文档。`master` 上的 `docs/`、两种语言的 README 和开发者指南，或网站源码变更会自动触发 [发布工作流](../.github/workflows/pages.yml)。也可在 GitHub Actions 手动运行 **Publish website**。

在仓库根目录运行以下命令即可构建并预览。

```bash
npm ci --prefix website
npm run build --prefix website
python3 -m http.server 8000 --bind 127.0.0.1 --directory website/dist
```

打开 <http://127.0.0.1:8000>。`website/dist/` 可以部署到网站根目录或任意子路径。

中文文档直接放入 `docs/`，英文版使用 `docs/en/` 下的同名路径。用一级标题作为页面名称。构建器会检查两种语言是否配对，自动将新文档加入目录和搜索。需要调整分组和顺序时，修改 `build.mjs` 中的 `groups`。文档内可使用相对路径引用图片和其他文档。
