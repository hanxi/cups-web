# CUPS Web website

[简体中文](README.md) · English

The repository's GitHub Pages site contains the product landing page and documentation. Changes to `docs/`, `README.md`, `README.en.md`, `AGENTS.md`, `AGENTS.en.md`, or the website source on `master` automatically trigger the [publishing workflow](../.github/workflows/pages.yml). You can also run **Publish website** from the Actions tab.

Build and preview from the repository root.

```bash
npm ci --prefix website
npm run build --prefix website
python3 -m http.server 8000 --bind 127.0.0.1 --directory website/dist
```

Open <http://127.0.0.1:8000>. Deploy `website/dist/` at the site root or a subpath.

Put Chinese documents in `docs/` and English translations at the matching path in `docs/en/`. Use a level-one heading as the page title. The builder checks that both languages are present and adds new documents to the navigation and search index. To set the group and order, edit `groups` in `build.mjs`. Use relative paths to link to images and other documents.
