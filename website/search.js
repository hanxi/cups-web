const dialog = document.querySelector('.search-dialog')
const input = dialog.querySelector('input')
const status = dialog.querySelector('.search-status')
const list = dialog.querySelector('.search-results')
const english = document.documentElement.lang === 'en'
const root = new URL('../', import.meta.url)
const indexUrl = new URL(english ? 'search-index.en.json' : 'search-index.json', import.meta.url)
let index
let queryNumber = 0

async function search() {
  const number = ++queryNumber
  const words = input.value.trim().toLowerCase().split(/\s+/).filter(Boolean)
  list.replaceChildren()
  if (!words.length) {
    status.textContent = english ? 'Search titles and document contents.' : '输入关键词可搜索标题和正文。'
    return
  }
  try {
    index ??= await fetch(indexUrl).then(response => {
      if (!response.ok) throw new Error('Failed to load search index')
      return response.json()
    })
  } catch {
    if (number === queryNumber) status.textContent = english ? 'Search is temporarily unavailable.' : '暂时无法加载搜索索引。'
    return
  }
  if (number !== queryNumber) return
  const matches = []
  for (const record of index) {
    const title = `${record.page} ${record.title}`.toLowerCase()
    const body = record.text.toLowerCase()
    if (!words.every(word => title.includes(word) || body.includes(word))) continue
    const score = words.reduce((sum, word) => sum + (title.includes(word) ? 3 : 1), 0)
    matches.push({ record, score })
  }
  status.textContent = matches.length
    ? (english ? `${matches.length} matches` : `共找到 ${matches.length} 个结果`)
    : (english ? 'No matching sections.' : '没有找到相关章节。')
  for (const { record } of matches.sort((a, b) => b.score - a.score).slice(0, 12)) {
    const item = document.createElement('li')
    const link = document.createElement('a')
    const label = document.createElement('strong')
    const summary = document.createElement('span')
    link.href = new URL(record.href, root)
    label.textContent = record.page === record.title ? record.title : `${record.page} · ${record.title}`
    const start = Math.max(0, record.text.toLowerCase().indexOf(words[0]) - 20)
    summary.textContent = `${start ? '…' : ''}${record.text.slice(start, start + 110)}${record.text.length > start + 110 ? '…' : ''}`
    link.append(label, summary)
    item.append(link)
    list.append(item)
  }
}

function openSearch() {
  if (!dialog.open) dialog.showModal()
  input.focus()
  input.select()
  search()
}

document.querySelector('[data-open-search]').addEventListener('click', openSearch)
input.addEventListener('input', search)
dialog.addEventListener('keydown', event => {
  if (event.key === 'Escape') {
    event.preventDefault()
    dialog.close()
  }
})
list.addEventListener('click', event => {
  if (event.target.closest('a')) dialog.close()
})
document.addEventListener('keydown', event => {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault()
    openSearch()
  }
})
