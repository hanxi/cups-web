import { createHash } from 'node:crypto'
import { cp, lstat, mkdir, readFile, readdir, writeFile } from 'node:fs/promises'
import { dirname, posix, resolve } from 'node:path'
import MarkdownIt from 'markdown-it'
import GithubSlugger from 'github-slugger'

const root = resolve(import.meta.dirname, '..')
const output = resolve(process.argv[2] ?? resolve(import.meta.dirname, 'dist'))
const github = 'https://github.com/hanxi/cups-web'
const md = new MarkdownIt({ html: true, linkify: true })
const escape = md.utils.escapeHtml
const stylesheet = await readFile(resolve(import.meta.dirname, 'style.css'))
const stylesheetVersion = createHash('sha256').update(stylesheet).digest('hex').slice(0, 12)
const stylesheetPath = `assets/style.${stylesheetVersion}.css`

const groups = [
  ['开始使用', 'Getting started', ['quick-start', 'configuration', 'usage', 'windows-shared-printer', 'reverse-proxy', 'faq', 'screenshots']],
  ['打印与运维', 'Printing and operations', ['printer-options', 'driver-management', 'pdf-pipeline', 'container-startup']],
  ['开发与构建', 'Development and builds', ['reference', 'architecture', 'conventions', 'docker-build']]
]

const languages = {
  'zh-CN': {
    prefix: '', suffix: '', group: 0, reference: '开发者指南', siteTitle: 'CUPS Web · 网页打印管理',
    description: '基于 CUPS 的开源网页打印管理工具，支持文档预览、定时打印、扫描和多用户管理。',
    footer: '基于 CUPS 的开源打印管理工具', license: 'MIT 许可证', docs: '文档', docsHome: '文档首页',
    docsLead: '部署、打印配置和维护指南。', start: '开始使用', siteNav: '站点导航', docsNav: '文档导航',
    toc: '本页目录', skip: '跳到正文', search: '搜索', searchLabel: '搜索文档',
    searchHint: '搜索指南、命令或功能', close: '关闭', source: '在 GitHub 查看原文',
    sections: ['快速开始', '配置说明', '使用指南', '进阶配置', '常见问题', '贡献'],
    cards: [
      ['从部署到打印', '选择 Docker 或二进制部署，连接 CUPS 并添加打印机。'],
      ['Windows 共享打印机', '从 Linux 的 CUPS 连接 Windows 上的 SMB 打印共享。'],
      ['打印机选项', '了解纸张、介质、质量选项和无边距打印。'],
      ['开发者指南', '查阅架构、HTTP API、数据结构与项目约定。']
    ]
  },
  en: {
    prefix: 'en/', suffix: '.en', group: 1, reference: 'Developer Guide', siteTitle: 'CUPS Web · Browser-based printing',
    description: 'CUPS Web is an open-source print manager for CUPS with document previews, scheduling, scanning, and user management.',
    footer: 'Open-source print management for CUPS', license: 'MIT license', docs: 'Docs', docsHome: 'Documentation home',
    docsLead: 'Guides for deployment, printing, and maintenance.', start: 'Get started', siteNav: 'Site navigation', docsNav: 'Documentation',
    toc: 'On this page', skip: 'Skip to content', search: 'Search', searchLabel: 'Search the documentation',
    searchHint: 'Search guides, commands, and features', close: 'Close', source: 'View source on GitHub',
    sections: ['Quick Start', 'Configuration', 'Usage Guide', 'Advanced Configuration', 'FAQ', 'Contributing'],
    cards: [
      ['Get up and running', 'Deploy with Docker or a binary, connect CUPS, and add a printer.'],
      ['Windows printer sharing', 'Connect Linux CUPS to an SMB printer shared from Windows.'],
      ['Printer options', 'Choose paper, media, quality, and borderless printing options.'],
      ['Developer guide', 'See the architecture, HTTP API, data model, and project conventions.']
    ]
  }
}

const docsFiles = async directory => (await readdir(resolve(root, directory), { recursive: true }))
  .map(file => file.split('\\').join('/')).filter(file => file.endsWith('.md')).sort()
const chineseDocs = (await docsFiles('docs')).filter(file => !file.startsWith('en/'))
const englishDocs = await docsFiles('docs/en')
const englishSet = new Set(englishDocs)
const chineseSet = new Set(chineseDocs)
const unpaired = [
  ...chineseDocs.filter(file => !englishSet.has(file)).map(file => `docs/en/${file}`),
  ...englishDocs.filter(file => !chineseSet.has(file)).map(file => `docs/${file}`)
]
if (unpaired.length) throw new Error(`Missing document translations:\n${unpaired.join('\n')}`)

const pages = []
for (const [lang, labels] of Object.entries(languages)) {
  const readmeFile = `README${labels.suffix}.md`
  const readme = await readFile(resolve(root, readmeFile), 'utf8')
  const heading = index => `## ${['🚀', '⚙️', '📖', '🔧', '❓', '🤝'][index]} ${labels.sections[index]}`
  for (const [id, index] of [['quick-start', 0], ['configuration', 1], ['usage', 2], ['faq', 4]]) {
    const beginning = readme.indexOf(heading(index))
    const ending = readme.indexOf(heading(index + 1), beginning + 1)
    if (beginning < 0 || ending < 0) throw new Error(`${readmeFile} section missing: ${labels.sections[index]}`)
    pages.push({ lang, id, title: labels.sections[index], source: readmeFile, content: readme.slice(beginning, ending) })
  }
  pages.push({ lang, id: 'reference', source: `AGENTS${labels.suffix}.md`, title: labels.reference })
  for (const file of chineseDocs) {
    if (file === 'README.md') continue
    const id = file === 'screenshots/README.md' ? 'screenshots' : file.replace(/\.md$/, '')
    pages.push({ lang, id, source: `docs/${labels.prefix}${file}` })
  }
}

const bySource = new Map()
const search = { 'zh-CN': [], en: [] }
for (const page of pages) {
  page.content ??= await readFile(resolve(root, page.source), 'utf8')
  const tokens = md.parse(page.content, {})
  const slugger = new GithubSlugger()
  const sections = []
  for (let i = 0; i < tokens.length; i++) {
    const token = tokens[i]
    if (token.type === 'heading_open') {
      const title = tokens[i + 1].children.map(child => child.content).join('')
      const id = slugger.slug(title)
      token.attrSet('id', id)
      sections.push({ title, id, level: Number(token.tag.slice(1)), text: [] })
    } else if (token.type === 'inline' && tokens[i - 1]?.type !== 'heading_open' && sections.length) {
      sections.at(-1).text.push(token.children.map(child => child.content).join(''))
    }
  }
  page.title ??= sections[0]?.title || page.id
  page.tokens = tokens
  page.sections = sections
  page.file = `${languages[page.lang].prefix}docs/${page.id === 'index' ? 'index/index' : page.id}.html`
  bySource.set(page.source, [...(bySource.get(page.source) ?? []), page])
  for (const section of sections) {
    search[page.lang].push({ page: page.title, title: section.title, href: `${urlPath(page.file)}#${encodeURIComponent(section.id)}`, text: section.text.join(' ') })
  }
}

const listed = new Set(groups.flatMap(([, , ids]) => ids))
const additional = [...new Set(pages.filter(page => !listed.has(page.id)).map(page => page.id))]
if (additional.length) groups.push(['其他文档', 'More documentation', additional])
const key = (lang, id) => `${lang}/${id}`
const byId = new Map(pages.map(page => [key(page.lang, page.id), page]))
if (byId.size !== pages.length) throw new Error('Duplicate documentation page IDs')
if (new Set(pages.map(page => page.file)).size !== pages.length) throw new Error('Duplicate documentation output paths')

function urlPath(path) {
  return path.split('/').map(encodeURIComponent).join('/')
}

function relative(from, to) {
  return urlPath(posix.relative(posix.dirname(from), to) || posix.basename(to))
}

const assetExtension = /\.(?:png|jpe?g|gif|webp|svg|pdf|avif)$/i
const screenshotDir = resolve(root, 'docs/screenshots')
const screenshotDimensions = new Map()
for (const name of await readdir(screenshotDir)) {
  if (!name.endsWith('.png')) continue
  const png = await readFile(resolve(screenshotDir, name))
  if (png.toString('ascii', 12, 16) !== 'IHDR') throw new Error(`Invalid screenshot: ${name}`)
  screenshotDimensions.set(name, [png.readUInt32BE(16), png.readUInt32BE(20)])
}

function rewrite(value, { page }) {
  if (!value || /^(?:[a-z]+:|\/)/i.test(value)) return value
  const [target, anchor] = value.split('#', 2)
  const fragment = anchor ? `#${anchor}` : ''
  if (!target && (!anchor || page.sections.some(section => section.id === decodeURI(anchor)))) return value
  const source = target ? posix.normalize(posix.join(posix.dirname(page.source), decodeURI(target))) : page.source
  if (source === 'docs/README.md' || source === 'docs/en/README.md') {
    return `${relative(page.file, source.includes('/en/') ? 'en/docs/index.html' : 'docs/index.html')}${fragment}`
  }
  const options = bySource.get(source)
  const match = anchor ? options?.find(item => item.sections.some(section => section.id === decodeURI(anchor))) : options?.[0]
  if (match) return relative(page.file, match.file) + fragment
  if (source.startsWith('docs/screenshots/') && source.endsWith('.png')) {
    return relative(page.file, `assets/screenshots/${posix.basename(source)}`)
  }
  if (source.startsWith('docs/') && assetExtension.test(source)) {
    return relative(page.file, `assets/${source}`) + fragment
  }
  return `${github}/blob/master/${urlPath(source)}${fragment}`
}

for (const [type, attr] of [['link_open', 'href'], ['image', 'src']]) {
  const original = md.renderer.rules[type] ?? ((tokens, index, options, env, renderer) => renderer.renderToken(tokens, index, options))
  md.renderer.rules[type] = (tokens, index, options, env, renderer) => {
    const token = tokens[index]
    const location = rewrite(token.attrGet(attr), env)
    token.attrSet(attr, location)
    if (type === 'image') {
      token.attrSet('loading', 'lazy')
      if (location?.includes('assets/screenshots/')) {
        const dimensions = screenshotDimensions.get(decodeURIComponent(posix.basename(location)))
        if (dimensions) {
          token.attrSet('width', String(dimensions[0]))
          token.attrSet('height', String(dimensions[1]))
        }
      }
    }
    return original(tokens, index, options, env, renderer)
  }
}
md.renderer.rules.table_open = () => '<div class="table-scroll"><table>\n'
md.renderer.rules.table_close = () => '</table></div>\n'

const otherLanguage = lang => lang === 'en' ? 'zh-CN' : 'en'
const homeFile = lang => `${languages[lang].prefix}index.html`
const docsFile = lang => `${languages[lang].prefix}docs/index.html`

function shell(file, lang, title, body, isDocs = false, alternate = homeFile(otherLanguage(lang))) {
  const labels = languages[lang]
  const home = relative(file, homeFile(lang))
  const docs = relative(file, docsFile(lang))
  const logo = relative(file, 'assets/logo.png')
  const brand = `<a class="brand" href="${home}"><img class="mark" src="${logo}" alt="" width="35" height="35"><span>CUPS Web</span></a>`
  const language = otherLanguage(lang)
  const contextAction = isDocs
    ? `<button class="search-open" data-open-search type="button">${labels.search} <kbd>Ctrl K</kbd></button>`
    : `<a class="top-cta" href="${relative(file, byId.get(key(lang, 'quick-start')).file)}">${labels.start}</a>`
  const header = `<header class="site-header"><div class="wrap header-inner">${brand}<nav aria-label="${labels.siteNav}"><a href="${docs}"${isDocs ? ' aria-current="page"' : ''}>${labels.docs}</a><a href="${github}">GitHub</a></nav><a class="language" href="${relative(file, alternate)}" hreflang="${language}" lang="${language}">${lang === 'en' ? '中文' : 'English'}</a>${contextAction}</div></header>`
  const footer = `<footer class="footer"><div class="wrap footer-inner">${brand}<span>${labels.footer}</span><a href="${github}/blob/master/LICENSE">${labels.license}</a></div></footer>`
  const dialog = isDocs ? `<dialog class="search-dialog" aria-label="${labels.searchLabel}"><div class="search-bar"><input type="search" placeholder="${labels.searchHint}" aria-label="${labels.searchLabel}" autocomplete="off"><form method="dialog"><button type="submit">${labels.close}</button></form></div><p class="search-status" aria-live="polite"></p><ul class="search-results"></ul></dialog><script type="module" src="${relative(file, 'assets/search.js')}"></script>` : ''
  return `<!doctype html>
<html lang="${lang}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta name="description" content="${escape(labels.description)}"><title>${escape(title)}</title><link rel="icon" type="image/png" sizes="192x192" href="${logo}"><link rel="stylesheet" href="${relative(file, stylesheetPath)}"></head>
<body><a class="skip" href="#main">${labels.skip}</a>${header}${body}${footer}${dialog}</body></html>\n`
}

function sectionList(file, lang, index = false) {
  return groups.map(group => {
    const pagesInGroup = group[2].map(id => byId.get(key(lang, id))).filter(Boolean)
    const links = pagesInGroup.map(page => {
      const link = `<a href="${relative(file, page.file)}"${file === page.file ? ' aria-current="page"' : ''}>${escape(page.title)}</a>`
      return index ? `<li>${link}</li>` : link
    }).join('')
    return `<section><h2>${group[languages[lang].group]}</h2>${index ? `<ul class="topic-list">${links}</ul>` : links}</section>`
  }).join('')
}

function docLayout(file, lang, article, sections = []) {
  const labels = languages[lang]
  const nav = `<nav aria-label="${labels.docsNav}"><a class="nav-home" href="${relative(file, docsFile(lang))}"${file === docsFile(lang) ? ' aria-current="page"' : ''}>${labels.docsHome}</a>${sectionList(file, lang)}</nav>`
  const toc = sections.filter(section => section.level === 2 || section.level === 3)
  const tocNav = toc.length ? `<nav aria-label="${labels.toc}"><h2>${labels.toc}</h2>${toc.map(section => `<a href="#${encodeURIComponent(section.id)}"${section.level === 3 ? ' class="toc-sub"' : ''}>${escape(section.title)}</a>`).join('')}</nav>` : ''
  return `<div class="wrap doc-layout"><aside class="doc-nav">${nav}</aside><details class="mobile-nav"><summary>${labels.docsNav}</summary>${nav}</details><main id="main" class="doc-main">${tocNav ? `<details class="mobile-toc"><summary>${labels.toc}</summary>${tocNav}</details>` : ''}<article class="article">${article}</article></main><aside class="doc-toc">${tocNav}</aside></div>`
}

function galleryLayout(page, sourceLink) {
  const labels = languages[page.lang]
  const firstSection = page.tokens.findIndex(token => token.type === 'heading_open' && token.tag === 'h2')
  if (firstSection === -1) throw new Error(`Missing screenshot sections in ${page.source}`)
  const intro = md.renderer.render(page.tokens.slice(0, firstSection), md.options, { page })
  const content = md.renderer.render(page.tokens.slice(firstSection), md.options, { page })
  const jumps = page.sections.filter(section => section.level === 2).map(section => `<a href="#${encodeURIComponent(section.id)}">${escape(section.title)}</a>`).join('')
  return `<main id="main" class="wrap gallery-page"><header class="gallery-intro"><a class="gallery-back" href="${relative(page.file, docsFile(page.lang))}">${labels.docsHome}</a>${intro}</header><nav class="gallery-index" aria-label="${labels.toc}">${jumps}</nav><article class="article gallery-content">${content}${sourceLink}</article></main>`
}

async function save(file, html) {
  await mkdir(resolve(output, dirname(file)), { recursive: true })
  await writeFile(resolve(output, file), html)
}

for (const [lang, labels] of Object.entries(languages)) {
  const file = homeFile(lang)
  const landing = await readFile(resolve(import.meta.dirname, `landing${labels.suffix}.html`), 'utf8')
  await save(file, shell(file, lang, labels.siteTitle, landing))
  const index = docsFile(lang)
  const topicIds = ['quick-start', 'windows-shared-printer', 'printer-options', 'reference']
  const cards = topicIds.map((id, i) => `<a href="${relative(index, byId.get(key(lang, id)).file)}"><h2>${labels.cards[i][0]}</h2><p>${labels.cards[i][1]}</p></a>`).join('')
  const indexBody = `<div class="doc-intro"><h1>${labels.docs}</h1><p>${labels.docsLead}</p></div><div class="topic-grid">${cards}</div>${sectionList(index, lang, true)}`
  await save(index, shell(index, lang, `${labels.docs} · CUPS Web`, docLayout(index, lang, indexBody), true, docsFile(otherLanguage(lang))))
}

await mkdir(resolve(output, 'assets'), { recursive: true })
for (const page of pages) {
  const link = `${github}/blob/master/${urlPath(page.source)}`
  const sourceLink = `<p class="source-link"><a href="${link}">${languages[page.lang].source}</a></p>`
  const body = page.id === 'screenshots'
    ? galleryLayout(page, sourceLink)
    : docLayout(page.file, page.lang, md.renderer.render(page.tokens, md.options, { page }) + sourceLink, page.sections)
  const alternate = byId.get(key(otherLanguage(page.lang), page.id)).file
  await save(page.file, shell(page.file, page.lang, `${page.title} · CUPS Web`, body, true, alternate))
}
await writeFile(resolve(output, stylesheetPath), stylesheet)
// Older HTML in caches still requests this stylesheet path.
await writeFile(resolve(output, 'assets/style.css'), stylesheet)
for (const name of ['search.js', 'logo.png']) await cp(resolve(import.meta.dirname, name), resolve(output, 'assets', name))
await cp(resolve(root, 'frontend/public/sponsor.png'), resolve(output, 'assets/sponsor.png'))
await cp(screenshotDir, resolve(output, 'assets/screenshots'), {
  recursive: true, filter: source => !source.endsWith('.md')
})
await cp(resolve(root, 'docs'), resolve(output, 'assets/docs'), {
  recursive: true,
  async filter(source) {
    if (source === screenshotDir) return false
    const entry = await lstat(source)
    return entry.isDirectory() || (entry.isFile() && assetExtension.test(source))
  }
})
for (const [lang, labels] of Object.entries(languages)) {
  await writeFile(resolve(output, `assets/search-index${labels.suffix}.json`), JSON.stringify(search[lang]))
}
console.log(`Built ${pages.length} documentation pages in ${Object.keys(languages).length} languages to ${output}`)
