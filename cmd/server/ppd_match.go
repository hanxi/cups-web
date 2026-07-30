package main

// PPD 候选匹配引擎（纯函数层）。
//
// 为什么需要它：历史实现 findBestPPDFromModels 是「两级 strings.Contains + 首个命中即返回」，
// 没有排序也没有来源偏好——厂商专有 PPD、foomatic 通用 PPD、gutenprint 一视同仁，
// 谁在 `lpinfo -m` 输出里排前面谁赢。镜像里装了 printer-driver-all +
// foomatic-db-compressed-ppds + openprinting-ppds + hpijs-ppds + hplip，
// `lpinfo -m` 有数千条，绝大多数机型其实都能被正确匹配，只是被这套算法浪费了。
//
// 本文件刻意只放纯函数：零 exec、零 os、零全局可变状态。
// 有副作用的一侧（跑 lpinfo、缓存、cups-driverd 委托、driverless 探测）在 ppd_query.go。
// 这样打分逻辑可以完全表驱动测试，而排序结果必须与 `lpinfo -m` 的输入行顺序无关
// （ppd_match_test.go 里有一条打乱输入的用例锁死这点）。

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// PPDSource 是 PPD 的来源分类，决定同等型号匹配强度下的优先级。
type PPDSource string

const (
	// PPDSourceCustom 是管理员通过 /api/admin/drivers/upload 上传的 PPD，
	// 排最前：既然人工上传了，就是最清楚自己要什么。
	PPDSourceCustom PPDSource = "custom"
	// PPDSourceVendor 是厂商原厂驱动的 PPD（CAPT / UFR II / ESC/P-R 等）。
	PPDSourceVendor PPDSource = "vendor"
	// PPDSourceHPLIP 是 HP 官方开源驱动（hpcups / hpijs）。
	PPDSourceHPLIP PPDSource = "hplip"
	// PPDSourceEverywhere 是 IPP Everywhere 伪驱动（lpadmin -m everywhere），
	// 属性由打印机自报，不存在型号错配风险。
	PPDSourceEverywhere PPDSource = "everywhere"
	// PPDSourceFoomatic 是 foomatic 社区驱动生成的 PPD。
	PPDSourceFoomatic PPDSource = "foomatic"
	// PPDSourceGutenprint 是 Gutenprint 驱动。
	PPDSourceGutenprint PPDSource = "gutenprint"
	// PPDSourceGeneric 是通用 PostScript / PCL PPD，能打但纸盒双面等功能多半缺失。
	PPDSourceGeneric PPDSource = "generic"
	// PPDSourceVirtual 是 CUPS-PDF / 盲文 / Text-Only 这类虚拟设备，直接剔除。
	PPDSourceVirtual PPDSource = "virtual"
	// PPDSourceUnknown 是分类不出来的，按 foomatic 稍低一档处理。
	PPDSourceUnknown PPDSource = "unknown"
)

// 置信度取值，直接回给前端渲染徽章。
const (
	ppdConfidenceHigh   = "high"
	ppdConfidenceMedium = "medium"
	ppdConfidenceLow    = "low"
)

// PPDEntry 是 `lpinfo -m` 一行的预解析结果。
// 归一化字段（normMake / normModel / compact / tokens）在解析时算好并随缓存一起存，
// 避免每次匹配请求都对几千行重复做一遍字符串归一化。
type PPDEntry struct {
	// Name 是第一列 ppd-name，可直接传给 lpadmin -m。
	Name string
	// MakeModel 是第二列原文，保留未加工的样子供人工核对与前端展示。
	MakeModel string
	// Source 由 ClassifyPPDSource 用「原始行」判定（在噪声 token 移除之前）。
	Source PPDSource
	// Language 是从描述尾缀（",zh"）或 ppd-name 路径段（"zh/xxx.ppd"）提取的 locale，未知为空。
	Language string
	// Recommended 表示描述里带 "(recommended)"，foomatic 用它标注首选驱动。
	Recommended bool

	normMake  string   // 归一化后的厂商（已过厂商别名表）
	normModel string   // 归一化后的型号，空格分隔
	compact   string   // normModel 去掉所有空格，让 "LBP-2900" ≡ "LBP2900"
	tokens    []string // normModel 的 token 切分
}

// PPDCandidate 是一条候选，直接 JSON 序列化给前端。
type PPDCandidate struct {
	// PPD 可直接作为 lpadmin -m 的实参。
	PPD string `json:"ppd"`
	// MakeAndModel 是 PPD 描述原文。
	MakeAndModel string    `json:"makeAndModel"`
	Source       PPDSource `json:"source"`
	Score        int       `json:"score"`
	// Confidence ∈ {high, medium, low}，前端据此渲染「完全匹配 / 可能匹配 / 通用」徽章。
	Confidence string `json:"confidence"`
	// Reason 是中文说明，前端直接显示，不必自己拼文案。
	Reason string `json:"reason"`
	// Recommended 只有 Top-1 且分数够、且不是 generic 时为 true。
	Recommended bool   `json:"recommended"`
	Language    string `json:"language,omitempty"`
	// DriverdRank 是 cups-driverd 按 IEEE-1284 device-id 给出的排名（1-based），0 表示未被命中。
	DriverdRank int `json:"driverdRank,omitempty"`
}

// MatchInput 是一次匹配请求的全部输入。
type MatchInput struct {
	Manufacturer string
	Model        string
	// DeviceID 是原始 IEEE-1284 串，本层只用它兜底解析型号；真正的指纹匹配在 ppd_query.go 里
	// 交给 cups-driverd，结果通过 DriverdRanks 传进来。
	DeviceID string
	// Scheme 是 device-uri 的 scheme（usb / ipp / dnssd …），决定 everywhere 是否可用。
	Scheme string
	// PreferLang 是偏好 locale，国内部署填 "zh"。
	PreferLang string
	// DriverdRanks 是 ppd-name → cups-driverd 排名。可为 nil（该能力不可用时）。
	DriverdRanks map[string]int
	// EverywhereEvidence：0 = 不可用；1 = 仅 scheme + CUPS 侧有 everywhere 条目；
	// 2 = 打印机 IPP 属性已确认支持。只有 2 才允许把 everywhere 置顶。
	EverywhereEvidence int
}

// 型号匹配强度分档。tier 决定语义，其余修正项只做同档内的 tie-break，
// 量级刻意拉开（相邻档 >= 150 分），避免「来源偏好 + driverd 加分」把弱匹配抬过强匹配。
const (
	ppdTierExactWithMake    = 1000 // 厂商一致 + compact 完全相等
	ppdTierBoundaryWithMake = 850  // 厂商一致 + compact 在字母/数字边界上互为前后缀
	ppdTierTokensWithMake   = 700  // 厂商一致 + 型号所有 token 命中
	ppdTierExactNoMake      = 500  // 厂商未知 + compact 完全相等
	ppdTierTokensNoMake     = 380  // 厂商未知 + 型号所有 token 命中
	ppdTierPartial          = 200  // token 命中率 >= 2/3 且含数字 token 全中
	ppdTierNone             = 0
)

// 修正项。
const (
	// ppdPenaltyMakeConflict：双方厂商都已知且不同。不淘汰而是打到低分区——
	// 万一型号解析把厂商认错了，管理员还能在候选列表末尾找到正确项手动选。
	ppdPenaltyMakeConflict = -600
	// ppdPenaltySeries：靠去掉 "series" 才命中的，精确度略低。
	ppdPenaltySeries = -40
	// ppdBonusRecommended：foomatic 自己标的 "(recommended)"。
	// 权重刻意小于来源偏好差（hplip +40 vs foomatic +20 = 20），
	// 否则一个 (recommended) 标记就能让 foomatic 翻过 hplip。
	ppdBonusRecommended = 10
	// ppdBonusLangMatch / ppdPenaltyLangOther：locale 偏好，权重刻意很小，
	// 语言不该盖过型号匹配强度。
	ppdBonusLangMatch   = 8
	ppdPenaltyLangOther = -6
	// ppdEverywhereBaseScore 让 everywhere 落在 medium 区间：
	// 有「厂商 + 型号」级命中（tier >= 700，加来源偏好后 >= 720）时排其后，
	// 但稳稳排在 foomatic 通用 PPD 的弱匹配（tier 200/380）之前。
	ppdEverywhereBaseScore = 420
)

// ppdSourceBonus 是来源偏好。顺序理由：
//   - custom 最高：管理员手动上传，意图最明确。
//   - vendor / hplip 次之：原厂 PPD 暴露装订、纸盒、分辨率等私有选项，
//     这是 everywhere 拿不到的（scripts/driver/install-canon-ufr2.sh 的注释已写明这点）。
//   - everywhere 排在 foomatic 之前：属性由打印机自报，型号错配风险为零，
//     且 media-source-supported / duplex 一定齐全——正是 /api/printer-info 依赖的那批属性。
//   - generic 负分：能打，但纸盒双面多半缺失，只作兜底。
func ppdSourceBonus(s PPDSource) int {
	switch s {
	case PPDSourceCustom:
		return 60
	case PPDSourceVendor:
		return 50
	case PPDSourceHPLIP:
		return 40
	case PPDSourceEverywhere:
		return 35
	case PPDSourceFoomatic:
		return 20
	case PPDSourceUnknown:
		return 15
	case PPDSourceGutenprint:
		return 10
	case PPDSourceGeneric:
		return -80
	}
	return 0
}

// ppdSourceRank 是同分时的排序权重（越小越前），与 ppdSourceBonus 保持同序。
func ppdSourceRank(s PPDSource) int {
	switch s {
	case PPDSourceCustom:
		return 0
	case PPDSourceVendor:
		return 1
	case PPDSourceHPLIP:
		return 2
	case PPDSourceEverywhere:
		return 3
	case PPDSourceFoomatic:
		return 4
	case PPDSourceUnknown:
		return 5
	case PPDSourceGutenprint:
		return 6
	case PPDSourceGeneric:
		return 7
	}
	return 8
}

// ppdSourceLabel 是给前端用的中文来源文案。放在后端是为了让 CLI / 日志 / 前端口径一致。
func ppdSourceLabel(s PPDSource) string {
	switch s {
	case PPDSourceCustom:
		return "PPD загружен администратором"
	case PPDSourceVendor:
		return "Оригинальный драйвер производителя"
	case PPDSourceHPLIP:
		return "Официальный драйвер HP (HPLIP)"
	case PPDSourceEverywhere:
		return "IPP Everywhere (без драйвера)"
	case PPDSourceFoomatic:
		return "Сообщество (Foomatic)"
	case PPDSourceGutenprint:
		return "Высококачественный драйвер Gutenprint"
	case PPDSourceGeneric:
		return "Универсальный PostScript / PCL (ограниченные функции)"
	}
	return "Неизвестный источник"
}

// ── 厂商别名归一 ───────────────────────────────────────────────────────────────

// ppdMakeAliases 把厂商的各种写法收敛到统一名字。
// key 是出现在 PPD 描述 / device-id 里的写法（全小写），value 是 canonical 名。
// 匹配时按 key 长度降序尝试前缀匹配，所以多词写法（"hewlett-packard"）会先于单词命中。
var ppdMakeAliases = map[string]string{
	"hewlett-packard":            "hp",
	"hewlett packard":            "hp",
	"hp":                         "hp",
	"canon inc.":                 "canon",
	"canon inc":                  "canon",
	"canon":                      "canon",
	"seiko epson":                "epson",
	"epson":                      "epson",
	"brother industries":         "brother",
	"brother":                    "brother",
	"konica minolta":             "konica minolta",
	"konicaminolta":              "konica minolta",
	"minolta":                    "konica minolta",
	"fuji xerox":                 "xerox",
	"xerox":                      "xerox",
	"ricoh company":              "ricoh",
	"ricoh":                      "ricoh",
	"samsung electronics":        "samsung",
	"samsung":                    "samsung",
	"kyocera document solutions": "kyocera",
	"kyocera mita":               "kyocera",
	"kyocera":                    "kyocera",
	"lexmark international":      "lexmark",
	"lexmark":                    "lexmark",
	"oki data":                   "oki",
	"okidata":                    "oki",
	"oki":                        "oki",
	"sharp":                      "sharp",
	"dell":                       "dell",
	"toshiba":                    "toshiba",
	"panasonic":                  "panasonic",
	"fujitsu":                    "fujitsu",
	"apple":                      "apple",
	"zebra":                      "zebra",
	"pantum":                     "pantum",
	"deli":                       "deli",
	"lenovo":                     "lenovo",
}

// ppdMakeAliasKeys 是 ppdMakeAliases 的 key 按长度降序排列，供最长前缀匹配用。
// 用包级变量 + init 预排，避免每次调用都重新排序（匹配是热路径：几千行 × 每次请求）。
var ppdMakeAliasKeys []string

func init() {
	ppdMakeAliasKeys = make([]string, 0, len(ppdMakeAliases))
	for k := range ppdMakeAliases {
		ppdMakeAliasKeys = append(ppdMakeAliasKeys, k)
	}
	sort.Slice(ppdMakeAliasKeys, func(i, j int) bool {
		if len(ppdMakeAliasKeys[i]) != len(ppdMakeAliasKeys[j]) {
			return len(ppdMakeAliasKeys[i]) > len(ppdMakeAliasKeys[j])
		}
		return ppdMakeAliasKeys[i] < ppdMakeAliasKeys[j]
	})
}

// canonicalMake 把厂商名收敛成 canonical 形式，认不出来就原样返回（已小写去空格）。
func canonicalMake(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	if c, ok := ppdMakeAliases[s]; ok {
		return c
	}
	return s
}

// splitMakeFromDescription 从「厂商 + 型号」整串里剥出厂商，返回 canonical 厂商与剩余型号。
// 剥不出已知厂商时厂商返回空串、整串当型号——宁可当「厂商未知」走 500/380 档，
// 也不要把型号的第一个词硬当成厂商（"LBP2900" 会被误判成厂商 "lbp2900"）。
func splitMakeFromDescription(s string) (make, rest string) {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" {
		return "", ""
	}
	for _, alias := range ppdMakeAliasKeys {
		if !strings.HasPrefix(lower, alias) {
			continue
		}
		// 必须落在词边界上，否则 "hp" 会从 "hpcups" 里被剥出来。
		tail := lower[len(alias):]
		if tail != "" && !isPPDSeparator(tail[0]) {
			continue
		}
		return ppdMakeAliases[alias], strings.TrimSpace(tail)
	}
	return "", lower
}

// isPPDSeparator 判断字节是否是型号串里的分隔符。
func isPPDSeparator(b byte) bool {
	switch b {
	case ' ', '\t', '-', '_', '/', '+', '.', ',', '(', ')':
		return true
	}
	return false
}

// ── 型号归一化 ─────────────────────────────────────────────────────────────────

// ppdNoiseTokens 是对型号匹配毫无贡献、却会稀释 token 命中率的词。
// 它们大多是驱动名或修饰词，被 PPD 描述塞在型号后面。
var ppdNoiseTokens = map[string]bool{
	"series": true, "printer": true, "printers": true, "recommended": true,
	"cups": true, "foomatic": true, "gutenprint": true, "hpijs": true,
	"hpcups": true, "brlaser": true, "escpr": true, "escpr2": true,
	"capt": true, "ufr": true, "ii": true, "ver": true, "driver": true,
	"linux": true, "for": true, "inkjet": true, "laser": true, "postscript": true,
}

// ppdVersionToken 匹配 "v5.3.4" / "3.23.12" 这类版本号 token。
var ppdVersionToken = regexp.MustCompile(`^v?[0-9]+(\.[0-9]+)+$`)

// ppdLocaleSuffix 匹配描述尾部的 locale 标记，如 ",zh" / ",zh_cn" / ",en"。
var ppdLocaleSuffix = regexp.MustCompile(`,\s*([a-z]{2}(?:[_-][a-z]{2})?)\s*$`)

// ppdLocalePathSeg 匹配 ppd-name 路径里的 locale 目录段，如 "zh/HP-xxx.ppd"。
var ppdLocalePathSeg = regexp.MustCompile(`(?:^|/)([a-z]{2}(?:_[A-Z]{2})?)/`)

// NormalizeModelKey 把型号串归一化成可比较的形式。
//
// 返回三个视角，打分时按需取用：
//   - norm：空格分隔的归一化型号，供人阅读与调试
//   - compact：norm 去掉所有空格，让 "LBP-2900" ≡ "LBP 2900" ≡ "LBP2900"
//   - tokens：切好的 token，供「所有 token 是否都命中」判定（顺序无关）
//
// 注意：本函数会移除噪声 token（series / foomatic / hpcups …），
// 而 ClassifyPPDSource 恰恰要靠这些词判定来源，所以它必须用原始行判定，
// 两者互不干扰——改任一侧前先确认没有把另一侧的输入弄脏。
func NormalizeModelKey(s string) (norm, compact string, tokens []string) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "", "", nil
	}

	// 分隔符统一成空格。'#' 与 '%' 也算：GBK 转义的 PPD 名里会出现。
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteByte(c)
		default:
			b.WriteByte(' ')
		}
	}

	for _, tok := range strings.Fields(b.String()) {
		if ppdNoiseTokens[tok] || ppdVersionToken.MatchString(tok) {
			continue
		}
		tokens = append(tokens, tok)
	}
	norm = strings.Join(tokens, " ")
	compact = strings.ReplaceAll(norm, " ", "")
	return norm, compact, tokens
}

// stripPPDDescriptionNoise 把 PPD 描述里的驱动尾缀剥掉，只留「厂商 + 型号」。
//
// 真实的 `lpinfo -m` 描述有好几种尾缀形态，逐一剥：
//
//	"HP LaserJet 1020, hpcups 3.23.12"                       → 逗号后是驱动名
//	"HP LaserJet 1020 Foomatic/foo2zjs (recommended)"         → Foomatic/xxx + 括号标记
//	"Epson L3250 - CUPS+Gutenprint v5.3.4"                    → 破折号后是驱动名
//	"HP LaserJet 1020 Foomatic/foo2zjs,zh"                    → 尾部 locale
func stripPPDDescriptionNoise(desc string) string {
	s := strings.TrimSpace(desc)
	// 先去掉尾部 locale（",zh"），否则会被后面的逗号切分吃掉、语言信息也就丢了。
	s = ppdLocaleSuffix.ReplaceAllString(s, "")
	// 括号标记：(recommended) / (en) / (A4 default) 等。
	if i := strings.Index(s, "("); i > 0 {
		s = strings.TrimSpace(s[:i])
	}
	// 逗号之后是驱动描述。
	if i := strings.Index(s, ","); i > 0 {
		s = strings.TrimSpace(s[:i])
	}
	// " - " 之后是驱动名（Gutenprint 用这种写法）。
	if i := strings.Index(s, " - "); i > 0 {
		s = strings.TrimSpace(s[:i])
	}
	// " Foomatic/xxx" 尾缀。
	if i := strings.Index(strings.ToLower(s), " foomatic/"); i > 0 {
		s = strings.TrimSpace(s[:i])
	}
	// " ver.X.Y" 尾缀（"Canon LBP2900 CAPT ver.2.71" → "Canon LBP2900 CAPT"）。
	// 不剥的话 "2.71" 会被分隔符归一拆成 "2" "71" 两个垃圾 token 污染匹配。
	if i := strings.Index(strings.ToLower(s), " ver"); i > 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

// extractPPDLanguage 从描述尾缀或 ppd-name 路径段里提取 locale，取不到返回空串。
func extractPPDLanguage(name, desc string) string {
	if m := ppdLocaleSuffix.FindStringSubmatch(strings.ToLower(strings.TrimSpace(desc))); m != nil {
		return strings.ReplaceAll(m[1], "-", "_")
	}
	if m := ppdLocalePathSeg.FindStringSubmatch(name); m != nil {
		return strings.ToLower(strings.ReplaceAll(m[1], "-", "_"))
	}
	return ""
}

// ── 来源分类 ───────────────────────────────────────────────────────────────────

// ClassifyPPDSource 判定一行 PPD 的来源。
//
// 判定顺序是有讲究的，两处容易踩坑：
//  1. generic 必须排在 gutenprint / foomatic 之前。
//     "gutenprint.5.3://pcl-4/expert  Generic PCL 4 Printer - CUPS+Gutenprint" 这行
//     虽然由 gutenprint 提供，但它是「通用 PCL」，对具体型号毫无价值，
//     必须归 generic 拿到负分，否则会挤掉真正的型号匹配项。
//  2. 必须用原始行判定（未经 NormalizeModelKey 的噪声移除），
//     因为 "hpcups" / "Foomatic/" 这些正是判定依据。
func ClassifyPPDSource(name, makeModel string) PPDSource {
	lowerName := strings.ToLower(strings.TrimSpace(name))
	lowerDesc := strings.ToLower(strings.TrimSpace(makeModel))

	// 虚拟设备：直接剔除，绝不该被推荐给物理打印机。
	if strings.Contains(lowerName, "cups-pdf") || strings.Contains(lowerDesc, "cups-pdf") ||
		strings.Contains(lowerName, "cups-brf") || strings.Contains(lowerDesc, "braille") ||
		strings.Contains(lowerName, "textonly") || strings.Contains(lowerDesc, "text-only") {
		return PPDSourceVirtual
	}

	// IPP Everywhere 伪驱动。
	if lowerName == "everywhere" || strings.HasPrefix(lowerName, "driverless:") {
		return PPDSourceEverywhere
	}

	// 管理员上传的 PPD 装在 /usr/share/cups/model/custom/（见 customPPDInstallDir）。
	if strings.Contains(lowerName, "custom/") {
		return PPDSourceCustom
	}

	// 通用 PPD：CUPS 自带的 sample.drv / cupsfilters.drv，或描述以 "Generic " 开头。
	// 必须排在 gutenprint / foomatic 之前（见函数注释）。
	if strings.Contains(lowerName, "drv:///sample.drv/") ||
		strings.Contains(lowerName, "drv:///cupsfilters.drv/") ||
		strings.Contains(lowerName, "lsb/usr/cupsfilters/") ||
		strings.HasPrefix(lowerDesc, "generic ") {
		return PPDSourceGeneric
	}

	if strings.HasPrefix(lowerName, "gutenprint") || strings.Contains(lowerDesc, "gutenprint") {
		return PPDSourceGutenprint
	}

	// foomatic：ppd-name 以 "foomatic:" 或 "foomatic-db-compressed-ppds:" 开头，
	// 或描述含 "Foomatic/"。
	if strings.HasPrefix(lowerName, "foomatic:") ||
		strings.HasPrefix(lowerName, "foomatic-db-compressed-ppds:") ||
		strings.Contains(lowerDesc, "foomatic/") {
		return PPDSourceFoomatic
	}

	// HPLIP：hpcups / hpijs 两条 filter 链。
	// 真实 ppd-name 形态：drv:///hpijs.drv/xxx、lsb/usr/hplip/HP/xxx、hpijs-ppds:xxx。
	if strings.Contains(lowerDesc, "hpcups") || strings.Contains(lowerDesc, "hpijs") ||
		strings.Contains(lowerName, "hpcups") || strings.Contains(lowerName, "hpijs") ||
		strings.Contains(lowerName, "lsb/usr/hplip/") {
		return PPDSourceHPLIP
	}

	// 厂商原厂驱动：
	//   - ppd-name 首段是厂商目录名（Canon/、Epson/、Brother/…）
	//   - 或 ppd-name 以已知厂商驱动前缀开头（escpr:、openprinting-ppds:）
	//   - 或描述里带厂商专有驱动标识（CAPT、UFR II、ESC/P-R…）
	if ppdNameHasVendorDir(lowerName) || hasVendorDriverMarker(lowerDesc) ||
		strings.HasPrefix(lowerName, "escpr:") || strings.HasPrefix(lowerName, "escpr2:") {
		return PPDSourceVendor
	}

	// openprinting-ppds: 前缀的 PPD 来源多样（有厂商的也有通用的），
	// 描述里通常含厂商名，交给 hasVendorDriverMarker 上面的分支处理；
	// 走到这里的按 unknown 处理（比 foomatic 稍低一档）。
	return PPDSourceUnknown
}

// ppdNameHasVendorDir 判断 ppd-name 的路径里是否出现已知厂商目录。
// openprinting-ppds / hplip 会把 PPD 装到 /usr/share/ppd/<Vendor>/ 下，
// lpinfo -m 的第一列就带上这个目录名（如 "Canon/CNRCUPSLBP2900CAPTK.ppd"）。
func ppdNameHasVendorDir(lowerName string) bool {
	for _, seg := range strings.Split(lowerName, "/") {
		if seg == "" {
			continue
		}
		if _, ok := ppdMakeAliases[seg]; ok {
			return true
		}
	}
	return false
}

// ppdVendorDriverMarkers 是厂商专有驱动在 PPD 描述里的特征串。
var ppdVendorDriverMarkers = []string{
	"capt", "ufr ii", "ufrii", "cnrcups", "cnpkbidi", "bizhub",
	"esc/p-r", "escpr", "epson inkjet printer driver", "pxlmono",
	"postscript-hp", "brother", "pcl6", "kpdl",
}

// hasVendorDriverMarker 判断描述里是否含厂商专有驱动标识。
func hasVendorDriverMarker(lowerDesc string) bool {
	for _, m := range ppdVendorDriverMarkers {
		if strings.Contains(lowerDesc, m) {
			return true
		}
	}
	return false
}

// ── 行解析 ─────────────────────────────────────────────────────────────────────

// ParsePPDLines 把 `lpinfo -m` 的输出行解析成 PPDEntry 列表。
// 每行形如 "<ppd-name> <make-and-model...>"，以第一个空格分列。
// 非法行（空行、只有一列、只有前导空格）直接跳过，绝不 panic——
// 这份输入来自外部命令，格式随 CUPS 版本有细微差别，宽容比严格更重要。
func ParsePPDLines(lines []string) []PPDEntry {
	entries := make([]PPDEntry, 0, len(lines))
	for _, raw := range lines {
		// 顺手吃掉 CRLF：容器里不会有，但管理员手工喂文件时会。
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line == "" {
			continue
		}
		name, desc, ok := strings.Cut(line, " ")
		if !ok || name == "" {
			continue
		}
		desc = strings.TrimSpace(desc)
		if desc == "" {
			continue
		}

		e := PPDEntry{
			Name:        name,
			MakeModel:   desc,
			Source:      ClassifyPPDSource(name, desc),
			Language:    extractPPDLanguage(name, desc),
			Recommended: strings.Contains(strings.ToLower(desc), "(recommended)"),
		}
		mk, rest := splitMakeFromDescription(stripPPDDescriptionNoise(desc))
		e.normMake = mk
		e.normModel, e.compact, e.tokens = NormalizeModelKey(rest)
		entries = append(entries, e)
	}
	return entries
}

// ── 打分 ───────────────────────────────────────────────────────────────────────

// charClassOf 把字节归成三类，供边界判定用。
func charClassOf(b byte) int {
	switch {
	case b >= '0' && b <= '9':
		return 1
	case b >= 'a' && b <= 'z':
		return 2
	}
	return 3
}

// compactBoundaryMatch 判断两个 compact 型号是否在「字母↔数字」边界上互为前后缀。
//
// 这条规则是防型号数字截断误配的关键：
//   - "l3250" vs "l3250series" → 边界处 '0'(数字) ↔ 's'(字母)，是真前缀，命中
//   - "l325"  vs "l3250"       → 边界处 '5'(数字) ↔ '0'(数字)，同类字符，
//     说明只是数字被截断了，L325 与 L3250 是两台不同的机器，不命中
func compactBoundaryMatch(a, b string) bool {
	if a == "" || b == "" || a == b {
		return false // 完全相等归 tier 1000 处理
	}
	long, short := a, b
	if len(long) < len(short) {
		long, short = short, long
	}
	if strings.HasPrefix(long, short) {
		return charClassOf(short[len(short)-1]) != charClassOf(long[len(short)])
	}
	if strings.HasSuffix(long, short) {
		return charClassOf(long[len(long)-len(short)-1]) != charClassOf(short[0])
	}
	return false
}

// tokensAllPresent 判断 want 里的每个 token 是否都出现在 have 中。
func tokensAllPresent(want, have []string) bool {
	if len(want) == 0 {
		return false
	}
	set := make(map[string]bool, len(have))
	for _, t := range have {
		set[t] = true
	}
	for _, t := range want {
		if !set[t] {
			return false
		}
	}
	return true
}

// tokenHitStats 统计 want 中有多少 token 命中 have，以及「含数字的 token」是否全部命中。
// 含数字的 token（"1020" / "l3250"）是型号的身份标识，缺一个就是另一台机器。
func tokenHitStats(want, have []string) (hits, total, digitTotal, digitHits int) {
	set := make(map[string]bool, len(have))
	for _, t := range have {
		set[t] = true
	}
	for _, t := range want {
		total++
		hasDigit := strings.ContainsAny(t, "0123456789")
		if hasDigit {
			digitTotal++
		}
		if set[t] {
			hits++
			if hasDigit {
				digitHits++
			}
		}
	}
	return hits, total, digitTotal, digitHits
}

// scoreTier 算出一条 PPD 相对目标设备的型号匹配强度档位。
func scoreTier(e PPDEntry, devMake, devCompact string, devTokens []string) int {
	if devCompact == "" || e.compact == "" {
		return ppdTierNone
	}
	makeKnown := devMake != "" && e.normMake != ""
	makeSame := makeKnown && devMake == e.normMake

	if makeSame {
		switch {
		case e.compact == devCompact:
			return ppdTierExactWithMake
		case compactBoundaryMatch(e.compact, devCompact):
			return ppdTierBoundaryWithMake
		case tokensAllPresent(devTokens, e.tokens):
			return ppdTierTokensWithMake
		}
	}

	// 厂商未知（任一侧解析不出来）时也要能匹配：很多网络打印机的 make-and-model
	// 只有裸型号（"LBP2900"），此时型号本身的精确度就是唯一依据。
	if !makeKnown {
		switch {
		case e.compact == devCompact:
			return ppdTierExactNoMake
		case tokensAllPresent(devTokens, e.tokens):
			return ppdTierTokensNoMake
		}
	}

	// 部分命中：至少 2/3 的 token 中，且所有含数字的 token 都命中。
	hits, total, digitTotal, digitHits := tokenHitStats(devTokens, e.tokens)
	if total > 0 && digitTotal > 0 && digitHits == digitTotal && hits*3 >= total*2 {
		return ppdTierPartial
	}
	return ppdTierNone
}

// ScorePPDCandidates 对全部 PPD 行打分并返回稳定排序的 Top-N 候选。
//
// 排序完全确定（与输入行顺序无关）：先按描述字典序预排，再按
// score → 来源 rank → 描述长度 → 描述字典序 做稳定排序。
// ppd_match_test.go 里有一条「打乱输入后结果必须完全等值」的用例锁死这点。
func ScorePPDCandidates(entries []PPDEntry, in MatchInput, topN int) []PPDCandidate {
	if topN <= 0 {
		topN = 5
	}

	devMake := canonicalMake(in.Manufacturer)
	devModelRaw := strings.TrimSpace(in.Model)
	// 型号里常带厂商前缀（device-id 的 MDL 字段尤其常见），去掉以免归一化后
	// 型号 token 里混进厂商名，把 token 命中率算低。
	if devMake != "" {
		if _, rest := splitMakeFromDescription(devModelRaw); rest != "" {
			devModelRaw = rest
		}
	}
	_, devCompact, devTokens := NormalizeModelKey(devModelRaw)

	// 型号解析不出东西时直接短路。这是历史实现最危险的缺陷：
	// 用空串或单字符去 strings.Contains 恒为 true，会把 lpinfo -m 的第一条 PPD
	// 随机套到打印机上。现在宁可什么都不推荐，让管理员手动选。
	modelUsable := len(devCompact) >= 2

	var everywhereEntry *PPDEntry
	cands := make([]PPDCandidate, 0, 16)
	maxTier := 0

	for i := range entries {
		e := &entries[i]
		if e.Source == PPDSourceVirtual {
			continue
		}
		if e.Source == PPDSourceEverywhere {
			// everywhere 的描述里没有型号，走不了 tier 判定，单独处理。
			if everywhereEntry == nil {
				everywhereEntry = e
			}
			continue
		}

		tier := ppdTierNone
		if modelUsable {
			tier = scoreTier(*e, devMake, devCompact, devTokens)
		}

		// generic 永远保留（作为兜底让管理员能选），其余没匹配上的直接丢弃，
		// 否则几千条 PPD 全进候选，列表就没意义了。
		if tier == ppdTierNone && e.Source != PPDSourceGeneric {
			continue
		}
		if tier > maxTier {
			maxTier = tier
		}

		score := tier + ppdSourceBonus(e.Source)

		// 厂商冲突：双方都已知且不同。不淘汰，只打到低分区。
		if devMake != "" && e.normMake != "" && devMake != e.normMake {
			score += ppdPenaltyMakeConflict
		}
		// 靠去掉 "series" 才命中的，精确度略低。
		// 但 compact 精确匹配（tier 1000/500）时不罚：此时 "series" 已被噪声移除
		// 处理掉了，再罚就是双重扣分。
		if tier < ppdTierExactNoMake &&
			strings.Contains(strings.ToLower(e.MakeModel), "series") &&
			!strings.Contains(strings.ToLower(devModelRaw), "series") {
			score += ppdPenaltySeries
		}
		if e.Recommended {
			score += ppdBonusRecommended
		}
		score += langAdjust(e.Language, in.PreferLang)

		rank := 0
		if in.DriverdRanks != nil {
			if r, ok := in.DriverdRanks[e.Name]; ok && r > 0 {
				rank = r
				// cups-driverd 命中是很强的信号（它比的是 PPD 里的 *1284DeviceID 指纹），
				// 但加分量级刻意小于一个 tier 差（150）：driverd 只该在同档内改变顺序，
				// 不该把弱匹配抬过强匹配。
				score += max(120-min(rank, 10)*10, 0)
			}
		}

		cands = append(cands, PPDCandidate{
			PPD:          e.Name,
			MakeAndModel: e.MakeModel,
			Source:       e.Source,
			Score:        score,
			Language:     e.Language,
			DriverdRank:  rank,
			Reason:       ppdSourceLabel(e.Source),
		})
	}

	// everywhere：只有 CUPS 侧确实有这个条目、且 scheme/IPP 证据支持时才作为候选出现。
	// 让管理员选一个 lpadmin 必然报错的值，比不给这个选项更糟。
	if everywhereEntry != nil && in.EverywhereEvidence >= 1 {
		score := ppdEverywhereBaseScore + ppdSourceBonus(PPDSourceEverywhere)
		// 覆盖规则：没有「厂商 + 型号」级命中（tier < 700），而打印机自己确认支持
		// IPP Everywhere（level 2）时把它顶到第一。理由：弱匹配的通用 PPD 套错机型
		// 的代价（乱码、几十页废纸）远高于 everywhere 的「属性略少但一定正确」。
		if in.EverywhereEvidence >= 2 && maxTier < ppdTierTokensWithMake {
			top := 0
			for _, c := range cands {
				if c.Score > top {
					top = c.Score
				}
			}
			if score <= top {
				score = top + 1
			}
		}
		cands = append(cands, PPDCandidate{
			PPD:          everywhereEntry.Name,
			MakeAndModel: everywhereEntry.MakeModel,
			Source:       PPDSourceEverywhere,
			Score:        score,
			Reason:       ppdSourceLabel(PPDSourceEverywhere),
		})
	}

	sortPPDCandidates(cands)

	// 截断，然后把 generic 兜底项补回列表尾部（最多 2 条）：
	// 一个 Top-N 里全是弱匹配却没有通用 PPD 可选的列表，管理员会无路可走。
	out := make([]PPDCandidate, 0, topN+2)
	for _, c := range cands {
		if len(out) >= topN {
			break
		}
		out = append(out, c)
	}
	appendGenericFallbacks(&out, cands, 2)

	for i := range out {
		out[i].Confidence = ppdConfidence(out[i].Score)
	}
	// Recommended 只给 Top-1，且必须分数够、且不是 generic——
	// 「推荐一条通用 PPD」在语义上就是错的。
	if len(out) > 0 && out[0].Score >= 400 && out[0].Source != PPDSourceGeneric {
		out[0].Recommended = true
	}
	return out
}

// langAdjust 算 locale 偏好的加减分。权重刻意很小：语言不该盖过型号匹配强度。
func langAdjust(entryLang, prefer string) int {
	if entryLang == "" {
		return 0
	}
	prefer = strings.ToLower(strings.TrimSpace(prefer))
	lang := strings.ToLower(entryLang)
	if prefer != "" && (lang == prefer || strings.HasPrefix(lang, prefer+"_")) {
		return ppdBonusLangMatch
	}
	// 英文是通用兜底，不加不减；其他语言的 PPD 对国内用户是负担。
	if lang == "en" || strings.HasPrefix(lang, "en_") {
		return 0
	}
	return ppdPenaltyLangOther
}

// sortPPDCandidates 做完全确定的排序：先按描述字典序预排消除输入顺序的影响，
// 再按 score → 来源 rank → 描述短者优先 → 描述字典序 稳定排序。
// 「描述短者优先」的直觉：更精确的 PPD 描述通常更短
// （"HP LaserJet 1020" vs "HP LaserJet 1020 Foomatic/foo2zjs (recommended)"）。
func sortPPDCandidates(cands []PPDCandidate) {
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].MakeAndModel != cands[j].MakeAndModel {
			return cands[i].MakeAndModel < cands[j].MakeAndModel
		}
		return cands[i].PPD < cands[j].PPD
	})
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		ra, rb := ppdSourceRank(a.Source), ppdSourceRank(b.Source)
		if ra != rb {
			return ra < rb
		}
		if len(a.MakeAndModel) != len(b.MakeAndModel) {
			return len(a.MakeAndModel) < len(b.MakeAndModel)
		}
		return a.MakeAndModel < b.MakeAndModel
	})
}

// appendGenericFallbacks 往 out 尾部补最多 limit 条尚未入选的 generic 候选。
func appendGenericFallbacks(out *[]PPDCandidate, all []PPDCandidate, limit int) {
	present := make(map[string]bool, len(*out))
	for _, c := range *out {
		present[c.PPD] = true
	}
	added := 0
	for _, c := range all {
		if added >= limit {
			break
		}
		if c.Source != PPDSourceGeneric || present[c.PPD] {
			continue
		}
		*out = append(*out, c)
		present[c.PPD] = true
		added++
	}
}

// ppdConfidence 把分数映射成三档置信度。
// 阈值对齐 tier：>= 850 意味着至少「厂商一致 + 型号边界匹配」，
// >= 400 意味着至少「型号 token 全中」，低于此只能算通用兜底。
func ppdConfidence(score int) string {
	switch {
	case score >= 850:
		return ppdConfidenceHigh
	case score >= 400:
		return ppdConfidenceMedium
	}
	return ppdConfidenceLow
}

// ── 队列名去重 ─────────────────────────────────────────────────────────────────

// uniquePrinterName 在已有队列名集合里找一个不冲突的名字。
// base 先过 sanitizePrinterName 收敛字符、截断到 127（CUPS 名字上限），
// 冲突时依次试 base-2 … base-50。返回最终名字与是否为新队列。
func uniquePrinterName(base string, existing map[string]string) (string, bool) {
	name, _ := uniquePrinterNameChecked(base, existing)
	if name == "" {
		name = "Printer"
	}
	_, taken := existing[name]
	return name, !taken
}

// uniquePrinterNameChecked 同 uniquePrinterName，但 50 个槽位全占时返回 error。
func uniquePrinterNameChecked(base string, existing map[string]string) (string, error) {
	name := sanitizePrinterName(base)
	if name == "" {
		name = "Printer"
	}
	if len(name) > 127 {
		name = name[:127]
	}
	if _, taken := existing[name]; !taken {
		return name, nil
	}
	for i := 2; i <= 50; i++ {
		candidate := fmt.Sprintf("%s-%d", name, i)
		if _, taken := existing[candidate]; !taken {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("Все 50 вариантов имени очереди %s заняты", name)
}

// ── driverless scheme 门槛（纯函数部分）──────────────────────────────────────

// driverlessInfo 描述一台打印机的 IPP Everywhere 可用性。
type driverlessInfo struct {
	Available bool   `json:"available"`
	Level     int    `json:"level"`  // 0=不可用 1=scheme+CUPS 2=IPP 属性确认
	Reason    string `json:"reason"`
	Evidence  string `json:"evidence,omitempty"`
}

// driverlessSchemes 是 IPP Everywhere 可用的 URI scheme 白名单。
// lpadmin -m everywhere 会对 device-uri 现场发 IPP Get-Printer-Attributes，
// 非 IPP scheme 必然失败。
var driverlessSchemes = map[string]bool{
	"ipp": true, "ipps": true, "dnssd": true, "http": true, "https": true,
}

// driverlessSchemeCheck 是 detectDriverless 的纯函数部分：
// 只看 URI scheme 与 CUPS 侧是否有 everywhere 条目，不做网络探测。
func driverlessSchemeCheck(uri string, entries []PPDEntry) driverlessInfo {
	scheme := ""
	if idx := strings.Index(uri, "://"); idx > 0 {
		scheme = strings.ToLower(uri[:idx])
	}
	if !driverlessSchemes[scheme] {
		return driverlessInfo{
			Available: false,
			Reason:    fmt.Sprintf("Подключение %s не поддерживает режим без драйверов (доступно только для IPP)", scheme),
		}
	}
	hasEverywhere := false
	for i := range entries {
		if entries[i].Source == PPDSourceEverywhere {
			hasEverywhere = true
			break
		}
	}
	if !hasEverywhere {
		return driverlessInfo{
			Available: false,
			Reason:    "Текущий CUPS не поддерживает IPP Everywhere (в lpinfo -m нет записи everywhere)",
		}
	}
	return driverlessInfo{
		Available: true,
		Level:     1,
		Reason:    "IPP соединение, CUPS поддерживает режим без драйверов",
	}
}

// ── PPD 名校验 ─────────────────────────────────────────────────────────────────

// ppdNameAllowed 是 ppd-name 的字符白名单。
// 真实的 ppd-name 形态：
//
//	everywhere
//	drv:///sample.drv/generic.ppd
//	gutenprint.5.3://escp2-l3250/expert
//	foomatic:HP-LaserJet_1020-foo2zjs.ppd
//	lsb/usr/HP/hp-laserjet_1020.ppd.gz
var ppdNameAllowed = regexp.MustCompile(`^[A-Za-z0-9._:%+/-]+$`)

// ValidatePPDNameSyntax 校验管理员指定的 ppd-name 语法是否合法。
//
// 为什么即使 lpadmin 是 argv 形式（无 shell 注入面）也必须校验：
//  1. 防日志注入——ppd-name 会被回显进 job log，前端用 <pre> 原样渲染，
//     含换行的值可以伪造出「命令执行成功」之类的假日志行。
//  2. 防路径穿越——虽然 -m 走的是 CUPS 的 model 目录，但拒绝 ".." 与绝对路径
//     是零成本的纵深防御。
//  3. 语义白名单（必须存在于 lpinfo -m 的输出里）在 ppd_query.go 里另做一道，
//     两道闸缺一不可：语法合法但不存在的名字会让 lpadmin 静默建出 raw 队列。
//
// 🔴 设计红线：永远不要为此暴露 lpadmin -P <file>。-P 接受任意本地文件路径，
// 等于给管理员一个「用 /etc/shadow 当 PPD」的入口。上传自定义 PPD 已有
// /api/admin/drivers/upload 这条路径，装完就会出现在 lpinfo -m 里，-m 足够。
func ValidatePPDNameSyntax(s string) error {
	if s == "" {
		return fmt.Errorf("PPD 名不能为空")
	}
	if len(s) > 512 {
		return fmt.Errorf("PPD 名过长（%d 字节，上限 512）", len(s))
	}
	if !ppdNameAllowed.MatchString(s) {
		return fmt.Errorf("PPD 名含非法字符（只允许字母、数字与 . _ : %% + / -）")
	}
	if strings.Contains(s, "..") {
		return fmt.Errorf("PPD 名不能包含 ..")
	}
	if strings.HasPrefix(s, "/") {
		return fmt.Errorf("PPD 名不能是绝对路径")
	}
	return nil
}
